package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PartnerSnapshotService управляет ежедневными снимками партнерских объектов
type PartnerSnapshotService struct {
	db *gorm.DB
}

// getTotalObjectsFromStats получает общее количество объектов (как на странице /objects)
// Использует тот же метод что и frontend: /cms/objects/?page=1&per_page=1 и берет count
func (s *PartnerSnapshotService) getTotalObjectsFromStats(token string) (total int, active int, err error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Запрашиваем первую страницу с 1 объектом чтобы получить общий count
	req, err := http.NewRequest("GET", "https://axenta.cloud/api/cms/objects/?page=1&per_page=1", nil)
	if err != nil {
		return 0, 0, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Token "+token)

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("ошибка запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("API вернул статус %d", resp.StatusCode)
	}

	var response struct {
		Count   int            `json:"count"`
		Results []axentaObject `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return 0, 0, fmt.Errorf("ошибка парсинга: %w", err)
	}

	// Подсчитываем активные объекты (нужно запросить все или использовать фильтр)
	// Для простоты делаем еще один запрос с фильтром is_active=true
	reqActive, _ := http.NewRequest("GET", "https://axenta.cloud/api/cms/objects/?page=1&per_page=1&is_active=true", nil)
	reqActive.Header.Set("Authorization", "Token "+token)

	respActive, err := client.Do(reqActive)
	if err == nil && respActive.StatusCode == http.StatusOK {
		var activeResponse struct {
			Count int `json:"count"`
		}
		json.NewDecoder(respActive.Body).Decode(&activeResponse)
		respActive.Body.Close()
		active = activeResponse.Count
	} else {
		// Если не удалось получить активные, оставляем 0
		active = 0
		if respActive != nil {
			respActive.Body.Close()
		}
	}

	total = response.Count

	log.Printf("📊 Общая статистика (как на /objects): всего=%d, активных=%d", total, active)

	return total, active, nil
}

// NewPartnerSnapshotService создает новый сервис снимков партнеров
func NewPartnerSnapshotService() *PartnerSnapshotService {
	return &PartnerSnapshotService{
		db: database.DB,
	}
}

// CreateDailySnapshots создает ежедневные снимки для всех партнерских договоров
// Вызывается каждый день в 00:00 UTC
func (s *PartnerSnapshotService) CreateDailySnapshots() error {
	log.Printf("📸 Начинаем создание ежедневных снимков партнерских договоров...")

	// Дата снимка - начало текущего дня в UTC
	now := time.Now().UTC()
	snapshotDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	log.Printf("📅 Дата снимка: %s", snapshotDate.Format("2006-01-02"))

	// Получаем все партнерские договоры
	var contracts []models.Contract
	if err := s.db.
		Where("contract_type = ? AND partner_company_id IS NOT NULL AND tariff_plan_id IS NOT NULL", "partner").
		Find(&contracts).Error; err != nil {
		return fmt.Errorf("ошибка получения партнерских договоров: %w", err)
	}

	log.Printf("📋 Найдено партнерских договоров: %d", len(contracts))

	if len(contracts) == 0 {
		log.Printf("ℹ️ Нет партнерских договоров для снимков")
		return nil
	}

	// Создаем снимки для каждого договора
	successCount := 0
	errorCount := 0

	for _, contract := range contracts {
		if err := s.CreateSnapshotForContract(&contract, snapshotDate); err != nil {
			log.Printf("❌ Ошибка создания снимка для договора %d: %v", contract.ID, err)
			errorCount++
		} else {
			successCount++
		}
	}

	log.Printf("✅ Создано снимков: %d, ошибок: %d", successCount, errorCount)
	return nil
}

// CreateSnapshotForContract создает снимок для конкретного договора
func (s *PartnerSnapshotService) CreateSnapshotForContract(contract *models.Contract, snapshotDate time.Time) error {
	// Получаем токен для запросов к Axenta Cloud
	// Для автоматических снимков используем системный токен
	token := os.Getenv("AXENTA_ADMIN_TOKEN")
	if token == "" {
		log.Printf("⚠️ AXENTA_ADMIN_TOKEN не установлен, пропускаем снимок для договора %d", contract.ID)
		return nil
	}

	return s.CreateSnapshotForContractWithToken(contract, snapshotDate, token)
}

// CreateSnapshotForContractWithToken создает снимок для конкретного договора с указанным токеном
func (s *PartnerSnapshotService) CreateSnapshotForContractWithToken(contract *models.Contract, snapshotDate time.Time, token string) error {
	return s.CreateSnapshotForContractWithTokenAndDB(contract, snapshotDate, token, s.db)
}

// CreateSnapshotForPartnerAccountWithoutContract создает снимок для партнерского аккаунта БЕЗ договора
func (s *PartnerSnapshotService) CreateSnapshotForPartnerAccountWithoutContract(
	companyID uint,
	partnerAccountID uint,
	partnerName string,
	snapshotDate time.Time,
	token string,
	defaultPlan *models.BillingPlan,
	db *gorm.DB,
) error {
	log.Printf("📸 Создание снимка для партнера БЕЗ договора: ID=%d, name=%s на дату %s",
		partnerAccountID, partnerName, snapshotDate.Format("2006-01-02"))

	// Проверяем есть ли уже снимок для этого партнера на эту дату (без привязки к договору)
	var existingSnapshot models.PartnerDailySnapshot
	if err := db.Unscoped().
		Where("partner_company_id = ? AND snapshot_date = ? AND contract_id = 0", partnerAccountID, snapshotDate).
		First(&existingSnapshot).Error; err == nil {
		log.Printf("ℹ️ Снимок для партнера %d (без договора) на дату %s уже существует", partnerAccountID, snapshotDate.Format("2006-01-02"))
		return fmt.Errorf("snapshot already exists")
	}

	// Получаем объекты партнера из Axenta Cloud
	objects, err := fetchPartnerObjects(token, int(partnerAccountID))
	if err != nil {
		return fmt.Errorf("ошибка получения объектов партнера: %w", err)
	}

	// Подсчитываем активные объекты
	activeCount := 0
	for _, obj := range objects {
		if obj.IsActive {
			activeCount++
		}
	}

	log.Printf("📊 Партнер %d (%s): всего=%d, активных=%d",
		partnerAccountID, partnerName, len(objects), activeCount)

	// Рассчитываем стоимость
	// Рассчитываем дневную стоимость из месячной
	monthlyPrice := defaultPlan.Price
	dailyPrice := monthlyPrice.Div(decimal.NewFromInt(30))

	costBeforeDiscount := dailyPrice.Mul(decimal.NewFromInt(int64(activeCount)))
	dailyCost := costBeforeDiscount // Без скидок для партнеров без договора

	// Создаем снимок
	snapshot := models.PartnerDailySnapshot{
		AdminAccountID:     companyID,
		CompanyID:          companyID,
		ContractID:         0, // Нет договора
		SnapshotDate:       snapshotDate,
		PartnerCompanyID:   partnerAccountID,
		TariffPlanID:       defaultPlan.ID,
		MonthlyPrice:       monthlyPrice,
		DailyPrice:         dailyPrice,
		TotalObjectsCount:  len(objects),
		ActiveObjectsCount: activeCount,
		DiscountType:       "none",
		DiscountPercent:    decimal.Zero,
		DiscountFixed:      decimal.Zero,
		CostBeforeDiscount: costBeforeDiscount,
		DiscountAmount:     decimal.Zero,
		DailyCost:          dailyCost,
		Status:             "completed",
		Notes:              fmt.Sprintf("Автоматический снимок партнера БЕЗ договора: %s", partnerName),
	}

	if err := db.Create(&snapshot).Error; err != nil {
		return fmt.Errorf("ошибка сохранения снимка: %w", err)
	}

	log.Printf("✅ Снимок создан для партнера %d (без договора): ID=%d, активных объектов=%d, стоимость=%.2f",
		partnerAccountID, snapshot.ID, activeCount, dailyCost.InexactFloat64())

	return nil
}

// CreateSnapshotForContractWithTokenAndDB создает снимок для конкретного договора с указанным токеном и базой данных
func (s *PartnerSnapshotService) CreateSnapshotForContractWithTokenAndDB(contract *models.Contract, snapshotDate time.Time, token string, db *gorm.DB) error {
	log.Printf("📸 Создание снимка для договора ID=%d, number=%s на дату %s",
		contract.ID, contract.Number, snapshotDate.Format("2006-01-02"))

	// Проверяем, не создан ли уже снимок на эту дату (включая мягко удаленные)
	var existingSnapshot models.PartnerDailySnapshot
	if err := db.Unscoped().
		Where("contract_id = ? AND snapshot_date = ?", contract.ID, snapshotDate).
		First(&existingSnapshot).Error; err == nil {

		// Проверяем является ли это сегодняшним снимком
		today := time.Now().UTC()
		todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
		isToday := snapshotDate.Equal(todayStart)

		// Исторические снимки НЕ обновляются (данные за прошлые дни зафиксированы)
		// Обновляем только сегодняшний снимок (данные за сегодня могут меняться)
		if !isToday {
			log.Printf("✅ Снимок для договора %d на дату %s уже существует (исторический), пропускаем обновление",
				contract.ID, snapshotDate.Format("2006-01-02"))
			return nil
		}

		log.Printf("ℹ️ Снимок для договора %d на дату %s уже существует (сегодняшний), обновляем...",
			contract.ID, snapshotDate.Format("2006-01-02"))

		// Обновляем существующий снимок вместо создания нового
		// Загружаем тарифный план из глобальной БД (billing_plans в public схеме)
		var tariffPlan models.BillingPlan
		if err := s.db.
			Where("id = ?", *contract.TariffPlanID).
			First(&tariffPlan).Error; err != nil {
			return fmt.Errorf("тарифный план не найден: %w", err)
		}

		// Получаем объекты партнера из Axenta Cloud API на дату снимка
		objectsCount, activeObjectsCount, objects, err := s.getPartnerObjectsWithCountForDate(*contract.PartnerCompanyID, token, snapshotDate)
		if err != nil {
			return fmt.Errorf("ошибка получения объектов партнера: %w", err)
		}

		// Сохраняем объекты в БД
		if err := s.savePartnerObjectsToDB(contract.AdminAccountID, objects, snapshotDate, db); err != nil {
			log.Printf("⚠️ Ошибка сохранения объектов в БД (продолжаем обновление снимка): %v", err)
			// Не прерываем обновление снимка из-за ошибки сохранения объектов
		}

		// Расчет цен
		monthlyPrice := tariffPlan.Price

		// Рассчитываем скидку (приоритет: фиксированная > процентная)
		discountPercent := contract.GetDiscountPercent(activeObjectsCount)
		discountFixed := contract.GetDiscountFixed()

		// Обновляем снимок
		existingSnapshot.TotalObjectsCount = objectsCount
		existingSnapshot.ActiveObjectsCount = activeObjectsCount
		existingSnapshot.MonthlyPrice = monthlyPrice
		existingSnapshot.DiscountType = contract.DiscountType
		existingSnapshot.DiscountPercent = discountPercent
		existingSnapshot.DiscountFixed = discountFixed
		existingSnapshot.Status = "completed"
		existingSnapshot.DeletedAt = gorm.DeletedAt{} // Восстанавливаем если был удален

		// DailyPrice, CostBeforeDiscount, DiscountAmount и DailyCost будут рассчитаны в BeforeCreate/BeforeSave
		// Но так как это update, пересчитаем вручную

		// Для фиксированной скидки: применяем к месячному тарифу
		// Для процентной скидки: применяем к дневной стоимости
		if discountFixed.GreaterThan(decimal.Zero) {
			// Фиксированная скидка применяется к МЕСЯЧНОМУ тарифу
			effectiveMonthlyPrice := monthlyPrice.Sub(discountFixed)
			if effectiveMonthlyPrice.IsNegative() {
				effectiveMonthlyPrice = decimal.Zero
			}
			effectiveDailyPrice := effectiveMonthlyPrice.Div(decimal.NewFromInt(30)).Round(4)
			existingSnapshot.DailyPrice = effectiveDailyPrice

			baseDailyPrice := monthlyPrice.Div(decimal.NewFromInt(30)).Round(4)
			existingSnapshot.CostBeforeDiscount = baseDailyPrice.Mul(decimal.NewFromInt(int64(activeObjectsCount))).Round(2)
			existingSnapshot.DiscountAmount = discountFixed.Div(decimal.NewFromInt(30)).Mul(decimal.NewFromInt(int64(activeObjectsCount))).Round(2)
			existingSnapshot.DailyCost = effectiveDailyPrice.Mul(decimal.NewFromInt(int64(activeObjectsCount))).Round(2)
		} else {
			// Базовая дневная цена
			baseDailyPrice := monthlyPrice.Div(decimal.NewFromInt(30)).Round(4)
			existingSnapshot.DailyPrice = baseDailyPrice

			costBeforeDiscount := baseDailyPrice.Mul(decimal.NewFromInt(int64(activeObjectsCount))).Round(2)
			existingSnapshot.CostBeforeDiscount = costBeforeDiscount

			// Процентная скидка применяется к стоимости
			if discountPercent.GreaterThan(decimal.Zero) {
				discountMultiplier := discountPercent.Div(decimal.NewFromInt(100))
				existingSnapshot.DiscountAmount = costBeforeDiscount.Mul(discountMultiplier).Round(2)
			} else {
				existingSnapshot.DiscountAmount = decimal.Zero
			}

			existingSnapshot.DailyCost = costBeforeDiscount.Sub(existingSnapshot.DiscountAmount).Round(2)
		}

		// Используем Unscoped() для сохранения, чтобы обновить даже мягко удаленную запись
		if err := db.Unscoped().Save(&existingSnapshot).Error; err != nil {
			return fmt.Errorf("ошибка обновления снимка: %w", err)
		}

		log.Printf("✅ Снимок обновлен: договор=%s, дата=%s, объектов=%d (активных=%d), стоимость=%.2f₽ (до скидки: %.2f₽)",
			contract.Number, snapshotDate.Format("2006-01-02"), objectsCount, activeObjectsCount,
			existingSnapshot.DailyCost, existingSnapshot.CostBeforeDiscount)

		return nil
	}

	// Загружаем тарифный план из глобальной БД (billing_plans в public схеме)
	var tariffPlan models.BillingPlan
	if err := s.db.
		Where("id = ?", *contract.TariffPlanID).
		First(&tariffPlan).Error; err != nil {
		return fmt.Errorf("тарифный план не найден: %w", err)
	}

	// Получаем объекты партнера из Axenta Cloud API на дату снимка
	objectsCount, activeObjectsCount, objects, err := s.getPartnerObjectsWithCountForDate(*contract.PartnerCompanyID, token, snapshotDate)
	if err != nil {
		return fmt.Errorf("ошибка получения объектов партнера: %w", err)
	}

	// Сохраняем объекты в БД
	if err := s.savePartnerObjectsToDB(contract.AdminAccountID, objects, snapshotDate, db); err != nil {
		log.Printf("⚠️ Ошибка сохранения объектов в БД (продолжаем создание снимка): %v", err)
		// Не прерываем создание снимка из-за ошибки сохранения объектов
	}

	// Расчет цен и скидок
	monthlyPrice := tariffPlan.Price
	discountPercent := contract.GetDiscountPercent(activeObjectsCount)
	discountFixed := contract.GetDiscountFixed()

	// Создаем снимок
	snapshot := models.PartnerDailySnapshot{
		AdminAccountID:     contract.AdminAccountID,
		CompanyID:          contract.CompanyID,
		ContractID:         contract.ID,
		SnapshotDate:       snapshotDate,
		PartnerCompanyID:   *contract.PartnerCompanyID,
		TariffPlanID:       *contract.TariffPlanID,
		MonthlyPrice:       monthlyPrice,
		TotalObjectsCount:  objectsCount,
		ActiveObjectsCount: activeObjectsCount,
		DiscountType:       contract.DiscountType,
		DiscountPercent:    discountPercent,
		DiscountFixed:      discountFixed,
		Status:             "completed",
	}
	// DailyPrice, CostBeforeDiscount, DiscountAmount и DailyCost будут рассчитаны в BeforeCreate

	if err := db.Create(&snapshot).Error; err != nil {
		return fmt.Errorf("ошибка создания снимка: %w", err)
	}

	log.Printf("✅ Снимок создан: договор=%s, дата=%s, объектов=%d (активных=%d), скидка=%.2f%%, стоимость=%.2f₽ (было %.2f₽)",
		contract.Number, snapshotDate.Format("2006-01-02"), objectsCount, activeObjectsCount,
		discountPercent, snapshot.DailyCost, snapshot.CostBeforeDiscount)

	return nil
}

// fetchPartnerObjects получает все объекты партнера из Axenta Cloud API
// Загружает ВСЕ объекты и фильтрует на сервере по accountId
func fetchPartnerObjects(token string, partnerCompanyID int) ([]axentaObject, error) {
	client := &http.Client{Timeout: 180 * time.Second}

	var allObjects []axentaObject

	page := 1
	perPage := 1000

	for {
		// Загружаем объекты для конкретного партнера через параметр accountId
		// API Axenta автоматически включает объекты всех дочерних аккаунтов
		axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/objects/?accountId=%d&page=%d&per_page=%d",
			partnerCompanyID, page, perPage)

		req, err := http.NewRequest("GET", axentaURL, nil)
		if err != nil {
			return nil, fmt.Errorf("ошибка создания запроса: %w", err)
		}

		req.Header.Set("Authorization", "Token "+token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ошибка запроса к Axenta Cloud: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("axenta Cloud вернул статус %d", resp.StatusCode)
		}

		var axentaResponse struct {
			Results []axentaObject `json:"results"`
			Count   int            `json:"count"`
			Next    *string        `json:"next"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&axentaResponse); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
		}
		resp.Body.Close()

		allObjects = append(allObjects, axentaResponse.Results...)

		if len(axentaResponse.Results) < perPage || axentaResponse.Next == nil {
			break
		}

		page++
	}

	// API Axenta Cloud уже отфильтровал объекты по партнеру (включая дочерние аккаунты)
	log.Printf("✅ Загружено %d объектов партнера %d (включая дочерние аккаунты)",
		len(allObjects), partnerCompanyID)

	return allObjects, nil
}

// getPartnerObjectsCountWithToken получает количество объектов партнера из Axenta Cloud с указанным токеном
// Учитывает дату снимка для фильтрации объектов по дате создания
func (s *PartnerSnapshotService) getPartnerObjectsCountWithToken(partnerCompanyID uint, token string) (total int, active int, err error) {
	// Для обратной совместимости - используем текущую дату
	return s.getPartnerObjectsCountForDate(partnerCompanyID, token, time.Now())
}

// getPartnerObjectsWithCountForDate получает объекты партнера на определенную дату и их количество
// Возвращает: количество, активное количество, список объектов, ошибка
func (s *PartnerSnapshotService) getPartnerObjectsWithCountForDate(partnerCompanyID uint, token string, snapshotDate time.Time) (total int, active int, objects []axentaObject, err error) {
	// Увеличенный таймаут для больших объемов данных (до 5000+ объектов)
	client := &http.Client{Timeout: 180 * time.Second}

	var allObjects []axentaObject

	page := 1
	perPage := 1000

	for {
		// Запрос к Axenta Cloud API с фильтром accountId
		// API автоматически включает объекты всех дочерних аккаунтов партнера
		axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/objects/?accountId=%d&page=%d&per_page=%d",
			partnerCompanyID, page, perPage)

		req, err := http.NewRequest("GET", axentaURL, nil)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("ошибка создания запроса: %w", err)
		}

		// Используем переданный токен
		req.Header.Set("Authorization", "Token "+token)

		resp, err := client.Do(req)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("ошибка запроса к Axenta Cloud: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return 0, 0, nil, fmt.Errorf("axenta Cloud вернул статус %d", resp.StatusCode)
		}

		var axentaResponse struct {
			Results []axentaObject `json:"results"`
			Count   int            `json:"count"`
			Next    *string        `json:"next"` // URL следующей страницы (если есть)
		}

		if err := json.NewDecoder(resp.Body).Decode(&axentaResponse); err != nil {
			resp.Body.Close()
			return 0, 0, nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
		}
		resp.Body.Close()

		// Добавляем объекты с текущей страницы
		allObjects = append(allObjects, axentaResponse.Results...)

		log.Printf("📄 Страница %d: получено %d объектов, всего загружено %d из %d",
			page, len(axentaResponse.Results), len(allObjects), axentaResponse.Count)

		// Если получили меньше объектов, чем per_page, или нет следующей страницы - это последняя страница
		if len(axentaResponse.Results) < perPage || axentaResponse.Next == nil {
			break
		}

		page++
	}

	// Фильтруем объекты с учетом даты снимка
	var filteredObjects []axentaObject
	total = 0
	active = 0

	log.Printf("📦 Загружено %d объектов партнера %d, фильтруем с учетом даты %s",
		len(allObjects), partnerCompanyID, snapshotDate.Format("2006-01-02"))

	// Устанавливаем конец дня для снимка (23:59:59)
	snapshotEndOfDay := time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 23, 59, 59, 0, time.UTC)

	for _, obj := range allObjects {
		// Парсим дату создания объекта
		var createdAt time.Time
		if obj.CreatedAt != "" {
			// Пробуем разные форматы даты
			formats := []string{
				time.RFC3339,
				"2006-01-02T15:04:05.000Z",
				"2006-01-02T15:04:05Z",
				"2006-01-02 15:04:05",
				"2006-01-02",
			}

			parsed := false
			for _, format := range formats {
				if t, err := time.Parse(format, obj.CreatedAt); err == nil {
					createdAt = t
					parsed = true
					break
				}
			}

			if !parsed {
				log.Printf("⚠️ Не удалось распарсить дату создания объекта %d: %s, пропускаем", obj.ID, obj.CreatedAt)
				continue
			}
		} else {
			// Если нет даты создания, считаем что объект существовал всегда
			createdAt = time.Time{}
		}

		// Проверяем, был ли объект удален до даты снимка
		var deletedAt time.Time
		if obj.DeletedAt != "" {
			// Пробуем разные форматы даты
			formats := []string{
				time.RFC3339,
				"2006-01-02T15:04:05.000Z",
				"2006-01-02T15:04:05Z",
				"2006-01-02 15:04:05",
				"2006-01-02",
			}

			for _, format := range formats {
				if t, err := time.Parse(format, obj.DeletedAt); err == nil {
					deletedAt = t
					break
				}
			}
		}

		// Учитываем объект только если:
		// 1. Он был создан до или в день снимка
		// 2. Он не был удален до даты снимка (или не удален вообще)
		if (createdAt.IsZero() || createdAt.Before(snapshotEndOfDay) || createdAt.Equal(snapshotEndOfDay)) &&
			(deletedAt.IsZero() || deletedAt.After(snapshotEndOfDay)) {
			filteredObjects = append(filteredObjects, obj)
			total++
			if obj.IsActive {
				active++
			}
		}
	}

	log.Printf("✅ Объектов партнера %d на дату %s: %d (активных: %d, неактивных: %d)",
		partnerCompanyID, snapshotDate.Format("2006-01-02"), total, active, total-active)

	return total, active, filteredObjects, nil
}

// getPartnerObjectsCountForDate получает только количество объектов партнера (для обратной совместимости)
func (s *PartnerSnapshotService) getPartnerObjectsCountForDate(partnerCompanyID uint, token string, snapshotDate time.Time) (total int, active int, err error) {
	total, active, _, err = s.getPartnerObjectsWithCountForDate(partnerCompanyID, token, snapshotDate)
	return total, active, err
}

// CreateVirtualOthersSnapshot создает снимок для виртуального партнера "Прочие объекты"
// Используется для учета прямых клиентов GLOMOS или разницы
func (s *PartnerSnapshotService) CreateVirtualOthersSnapshot(
	companyID uint,
	totalObjects int,
	activeObjects int,
	snapshotDate time.Time,
	defaultPlan *models.BillingPlan,
	tenantDB *gorm.DB,
) error {
	// ID = 186 для прямых клиентов GLOMOS
	const virtualPartnerID = 186

	// Проверяем существует ли уже снимок
	var existingSnapshot models.PartnerDailySnapshot
	err := tenantDB.
		Where("partner_company_id = ? AND DATE(snapshot_date AT TIME ZONE 'UTC') = ?",
			virtualPartnerID, snapshotDate.Format("2006-01-02")).
		First(&existingSnapshot).Error

	if err == nil {
		// Снимок уже существует, обновляем
		existingSnapshot.TotalObjectsCount = totalObjects
		existingSnapshot.ActiveObjectsCount = activeObjects

		// Расчет стоимости
		dailyRate := defaultPlan.Price.Div(decimal.NewFromInt(30))
		costBeforeDiscount := dailyRate.Mul(decimal.NewFromInt(int64(activeObjects)))
		existingSnapshot.DailyPrice = dailyRate
		existingSnapshot.CostBeforeDiscount = costBeforeDiscount
		existingSnapshot.DiscountAmount = decimal.Zero
		existingSnapshot.DailyCost = costBeforeDiscount

		if err := tenantDB.Save(&existingSnapshot).Error; err != nil {
			return fmt.Errorf("ошибка обновления снимка 'Прочие': %w", err)
		}

		log.Printf("♻️ Обновлен снимок 'Прочие': всего=%d, активных=%d, стоимость=%.2f₽",
			totalObjects, activeObjects, costBeforeDiscount)
		return nil
	}

	// Создаем новый снимок
	dailyRate := defaultPlan.Price.Div(decimal.NewFromInt(30))
	costBeforeDiscount := dailyRate.Mul(decimal.NewFromInt(int64(activeObjects)))

	snapshot := models.PartnerDailySnapshot{
		AdminAccountID:     companyID,
		CompanyID:          companyID,
		ContractID:         0, // Нет договора
		SnapshotDate:       snapshotDate,
		PartnerCompanyID:   virtualPartnerID, // 0 = виртуальный партнер "Прочие"
		TariffPlanID:       defaultPlan.ID,
		MonthlyPrice:       defaultPlan.Price,
		DailyPrice:         dailyRate,
		TotalObjectsCount:  totalObjects,
		ActiveObjectsCount: activeObjects,
		DiscountType:       "none",
		DiscountPercent:    decimal.Zero,
		DiscountFixed:      decimal.Zero,
		CostBeforeDiscount: costBeforeDiscount,
		DiscountAmount:     decimal.Zero,
		DailyCost:          costBeforeDiscount,
		Status:             "completed",
		Notes:              "Учётная запись GLOMOS (186) - прямые клиенты без партнёров в иерархии",
	}

	if err := tenantDB.Create(&snapshot).Error; err != nil {
		return fmt.Errorf("ошибка создания снимка 'Прочие': %w", err)
	}

	log.Printf("✨ Создан снимок 'Прочие': всего=%d, активных=%d, стоимость=%.2f₽",
		totalObjects, activeObjects, costBeforeDiscount)

	return nil
}

// CreateSnapshotWithObjectCounts создаёт снимок с уже подсчитанными объектами (без API запросов)
func (s *PartnerSnapshotService) CreateSnapshotWithObjectCounts(
	companyID uint,
	partnerID uint,
	totalObjects int,
	activeObjects int,
	snapshotDate time.Time,
	defaultPlan *models.BillingPlan,
	contract *models.Contract,
	partnerName string,
	tenantDB *gorm.DB,
) error {
	// Проверяем существует ли уже снимок
	// Учитываем partner_company_id, snapshot_date и contract_id (для правильной проверки уникальности)
	// Используем Unscoped для проверки всех записей (включая мягко удаленные)
	var existingSnapshot models.PartnerDailySnapshot
	query := tenantDB.Unscoped().
		Where("partner_company_id = ? AND DATE(snapshot_date AT TIME ZONE 'UTC') = ?",
			partnerID, snapshotDate.Format("2006-01-02"))

	// Если есть договор, проверяем с учетом contract_id, иначе проверяем снимки без договора (contract_id = 0)
	var contractID uint = 0
	if contract != nil {
		contractID = contract.ID
	}
	query = query.Where("contract_id = ?", contractID)

	err := query.First(&existingSnapshot).Error

	if err == nil {
		// Снимок уже существует
		log.Printf("⚠️ Снимок для партнера %d (договор %d) на дату %s уже существует (ID: %d), пропускаем создание",
			partnerID, contractID, snapshotDate.Format("2006-01-02"), existingSnapshot.ID)
		return fmt.Errorf("snapshot already exists")
	}

	// Определяем тарифный план
	var tariffPlan models.BillingPlan
	if contract != nil && contract.TariffPlanID != nil {
		if err := tenantDB.Where("id = ?", *contract.TariffPlanID).First(&tariffPlan).Error; err != nil {
			tariffPlan = *defaultPlan
		}
	} else {
		tariffPlan = *defaultPlan
	}

	// Расчёт стоимости
	monthlyPrice := tariffPlan.Price
	dailyRate := monthlyPrice.Div(decimal.NewFromInt(30))
	costBeforeDiscount := dailyRate.Mul(decimal.NewFromInt(int64(activeObjects)))

	// Расчёт скидки
	var discountAmount decimal.Decimal
	var discountType string
	var discountPercent decimal.Decimal
	var discountFixed decimal.Decimal

	if contract != nil {
		discountPercent = contract.GetDiscountPercent(activeObjects)
		discountFixed = contract.GetDiscountFixed()
		discountType = contract.DiscountType

		if discountType == "manual_fixed" && discountFixed.GreaterThan(decimal.Zero) {
			// Фиксированная скидка применяется к месячному тарифу, пересчитываем на дневную
			discountAmount = discountFixed.Div(decimal.NewFromInt(30))
		} else if (discountType == "manual_percent" || discountType == "manual" || discountType == "auto") && discountPercent.GreaterThan(decimal.Zero) {
			// Процентная скидка (включая автоматическую) применяется к стоимости
			discountAmount = costBeforeDiscount.Mul(discountPercent).Div(decimal.NewFromInt(100))
		}
	} else {
		discountType = "none"
		discountPercent = decimal.Zero
		discountFixed = decimal.Zero
		discountAmount = decimal.Zero
	}

	dailyCost := costBeforeDiscount.Sub(discountAmount)
	if dailyCost.LessThan(decimal.Zero) {
		dailyCost = decimal.Zero
	}

	// Создаём снимок
	// contractID уже определен выше при проверке существования снимка

	// Формируем примечание
	notes := "Снимок создан через точное распределение всех объектов (без дублей)"
	if partnerName != "" {
		notes = fmt.Sprintf("%s - %s", partnerName, notes)
	}

	snapshot := models.PartnerDailySnapshot{
		AdminAccountID:     companyID,
		CompanyID:          companyID,
		ContractID:         contractID,
		SnapshotDate:       snapshotDate,
		PartnerCompanyID:   partnerID,
		TariffPlanID:       tariffPlan.ID,
		MonthlyPrice:       monthlyPrice,
		DailyPrice:         dailyRate,
		TotalObjectsCount:  totalObjects,
		ActiveObjectsCount: activeObjects,
		DiscountType:       discountType,
		DiscountPercent:    discountPercent,
		DiscountFixed:      discountFixed,
		CostBeforeDiscount: costBeforeDiscount,
		DiscountAmount:     discountAmount,
		DailyCost:          dailyCost,
		Status:             "completed",
		Notes:              notes,
	}

	if err := tenantDB.Create(&snapshot).Error; err != nil {
		// Проверяем, является ли ошибка нарушением уникального ограничения
		if isDuplicateKeyError(err) {
			// Это нормальная ситуация - снимок уже существует (возможно, создан параллельно)
			log.Printf("⚠️ Снимок для партнера %d (договор %d) на дату %s уже существует (возможно, создан параллельно), пропускаем",
				partnerID, contractID, snapshotDate.Format("2006-01-02"))
			return fmt.Errorf("snapshot already exists")
		}
		return fmt.Errorf("ошибка создания снимка: %w", err)
	}

	return nil
}

// isDuplicateKeyError проверяет, является ли ошибка нарушением уникального ограничения
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "duplicate key") ||
		strings.Contains(errStr, "unique constraint") ||
		strings.Contains(errStr, "violates unique constraint") ||
		strings.Contains(errStr, "23505") || // PostgreSQL error code for unique violation
		strings.Contains(errStr, "idx_partner_snapshot_unique")
}

// GetSnapshotsForContract получает снимки для договора за период
func (s *PartnerSnapshotService) GetSnapshotsForContract(contractID uint, startDate, endDate time.Time) ([]models.PartnerDailySnapshot, error) {
	var snapshots []models.PartnerDailySnapshot

	if err := s.db.
		Where("contract_id = ? AND snapshot_date >= ? AND snapshot_date <= ?", contractID, startDate, endDate).
		Order("snapshot_date ASC").
		Find(&snapshots).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения снимков: %w", err)
	}

	return snapshots, nil
}

// CalculateCostForPeriod рассчитывает стоимость для договора за период на основе снимков
func (s *PartnerSnapshotService) CalculateCostForPeriod(contractID uint, startDate, endDate time.Time) (decimal.Decimal, error) {
	snapshots, err := s.GetSnapshotsForContract(contractID, startDate, endDate)
	if err != nil {
		return decimal.Zero, err
	}

	totalCost := decimal.Zero
	for _, snapshot := range snapshots {
		totalCost = totalCost.Add(snapshot.DailyCost)
	}

	return totalCost, nil
}

// StartDailySnapshotScheduler запускает планировщик для ежедневных снимков
func (s *PartnerSnapshotService) StartDailySnapshotScheduler() {
	log.Printf("🕐 Запуск планировщика ежедневных снимков партнерских договоров (00:00 UTC)")

	go func() {
		for {
			now := time.Now().UTC()

			// Вычисляем время до следующего полуночи UTC
			nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
			durationUntilMidnight := nextMidnight.Sub(now)

			log.Printf("⏰ Следующий снимок через %s (в %s UTC)", durationUntilMidnight.Round(time.Minute), nextMidnight.Format("2006-01-02 15:04:05"))

			// Ждем до полуночи
			time.Sleep(durationUntilMidnight)

			// Создаем снимки
			if err := s.CreateDailySnapshots(); err != nil {
				log.Printf("❌ Ошибка создания ежедневных снимков: %v", err)
			}
		}
	}()
}

// savePartnerObjectsToDB сохраняет объекты партнера в таблицу axenta_object_snapshots
func (s *PartnerSnapshotService) savePartnerObjectsToDB(
	adminAccountID uint,
	objects []axentaObject,
	snapshotDate time.Time,
	db *gorm.DB,
) error {
	savedCount := 0
	errorCount := 0

	// Устанавливаем время синхронизации на конец дня снимка
	snapshotEndOfDay := time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 23, 59, 59, 0, time.UTC)

	for _, obj := range objects {
		rawPayload, _ := json.Marshal(obj)

		// Парсим дату создания
		var createdAt time.Time
		var parsedCreated bool
		if obj.CreatedAt != "" {
			formats := []string{
				time.RFC3339,
				"2006-01-02T15:04:05.000Z",
				"2006-01-02T15:04:05Z",
				"2006-01-02 15:04:05",
				"2006-01-02",
			}

			for _, format := range formats {
				if t, err := time.Parse(format, obj.CreatedAt); err == nil {
					createdAt = t
					parsedCreated = true
					break
				}
			}
		}

		// Парсим дату удаления
		var deletedAt time.Time
		var parsedDeleted bool
		if obj.DeletedAt != "" {
			formats := []string{
				time.RFC3339,
				"2006-01-02T15:04:05.000Z",
				"2006-01-02T15:04:05Z",
				"2006-01-02 15:04:05",
				"2006-01-02",
			}

			for _, format := range formats {
				if t, err := time.Parse(format, obj.DeletedAt); err == nil {
					deletedAt = t
					parsedDeleted = true
					break
				}
			}
		}

		// Учитываем объект только если он был актуален на дату снимка
		shouldInclude := false
		if (createdAt.IsZero() || parsedCreated && createdAt.Before(snapshotEndOfDay) || parsedCreated && createdAt.Equal(snapshotEndOfDay)) &&
			(!parsedDeleted || deletedAt.IsZero() || deletedAt.After(snapshotEndOfDay)) {
			shouldInclude = true
		}

		if !shouldInclude && (parsedCreated || parsedDeleted) {
			// Пропускаем объекты, которые не существовали на дату снимка
			continue
		}

		snapshot := models.AxentaObjectSnapshot{
			// AdminAccountID больше не обязателен - объекты хранятся глобально
			// AdminAccountID:    &adminAccountID, // Можно оставить для обратной совместимости, но не используется в уникальном ключе
			AccountExternalID: int64(obj.AccountID),
			ExternalObjectID:  int64(obj.ID),
			ObjectName:        obj.Name,
			UniqueID:          obj.UniqueID,
			DeviceTypeName:    obj.DeviceTypeName,
			AccountName:       obj.AccountName,
			Status:            obj.Status,
			IsActive:          obj.IsActive,
			LastSyncedAt:      snapshotEndOfDay, // Используем конец дня снимка
			RawPayload:        string(rawPayload),
		}

		// Парсим последнее сообщение
		if obj.LastMessageDatetime != "" {
			if parsed := parseAxentaTime(obj.LastMessageDatetime); parsed != nil {
				snapshot.LastCommunicationAt = parsed
			}
		}

		// Новые поля из API
		if obj.CreatorName != "" {
			snapshot.CreatorName = &obj.CreatorName
		}
		if obj.CreatorID != 0 {
			snapshot.CreatorID = &obj.CreatorID
		}
		snapshot.CreatorIsActive = &obj.CreatorIsActive
		snapshot.AccountIsActive = &obj.AccountIsActive

		// Конвертируем массив телефонов в JSON
		if len(obj.PhoneNumbers) > 0 {
			phonesJSON, _ := json.Marshal(obj.PhoneNumbers)
			phonesStr := string(phonesJSON)
			snapshot.PhoneNumbers = &phonesStr
		}

		// Парсим дату создания в Axenta
		if parsedCreated {
			snapshot.AxentaCreatedAt = &createdAt
		}

		// Парсим дату удаления в Axenta
		if parsedDeleted && !deletedAt.IsZero() {
			snapshot.AxentaDeletedAt = &deletedAt
		}

		// Сохраняем с обработкой конфликтов (обновляем если существует)
		// Объекты хранятся глобально один раз по external_object_id
		// Привязка к партнерам через account_external_id и иерархию
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "external_object_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"account_external_id", "object_name", "unique_id", "device_type_name", "account_name",
				"status", "is_active", "last_synced_at", "raw_payload", "last_communication_at",
				"creator_name", "creator_id", "creator_is_active", "account_is_active",
				"phone_numbers", "axenta_created_at", "axenta_deleted_at",
			}),
		}).Create(&snapshot).Error; err != nil {
			log.Printf("⚠️ Ошибка сохранения объекта %d: %v", obj.ID, err)
			errorCount++
			continue
		}

		savedCount++
	}

	if errorCount > 0 {
		log.Printf("⚠️ При сохранении объектов возникло ошибок: %d (обработано: %d)", errorCount, savedCount)
	}
	// Финальная статистика по уникальным объектам выводится в SavePartnerObjectsToDBForSnapshot
	return nil
}

// SavePartnerObjectsToDBForSnapshot загружает и сохраняет объекты партнера в БД для снимка
func (s *PartnerSnapshotService) SavePartnerObjectsToDBForSnapshot(
	adminAccountID uint,
	partnerCompanyID uint,
	token string,
	snapshotDate time.Time,
	db *gorm.DB,
) error {
	// Получаем объекты для партнера (используем существующую функцию)
	_, _, objects, err := s.getPartnerObjectsWithCountForDate(partnerCompanyID, token, snapshotDate)
	if err != nil {
		return fmt.Errorf("ошибка получения объектов: %w", err)
	}

	// Подсчитываем количество уникальных объектов в БД до сохранения (глобально, без привязки к компании)
	var uniqueObjectsBefore int64
	db.Model(&models.AxentaObjectSnapshot{}).
		Where("DATE(last_synced_at AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
		Count(&uniqueObjectsBefore)

	// Сохраняем объекты в БД
	if err := s.savePartnerObjectsToDB(adminAccountID, objects, snapshotDate, db); err != nil {
		return fmt.Errorf("ошибка сохранения объектов: %w", err)
	}

	// Подсчитываем количество уникальных объектов в БД после сохранения (глобально)
	var uniqueObjectsAfter int64
	db.Model(&models.AxentaObjectSnapshot{}).
		Where("DATE(last_synced_at AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
		Count(&uniqueObjectsAfter)

	// Выводим информацию только о новых объектах, чтобы не перегружать логи
	newObjectsCount := uniqueObjectsAfter - uniqueObjectsBefore
	if newObjectsCount > 0 {
		log.Printf("✅ Партнер %d: добавлено %d новых уникальных объектов", partnerCompanyID, newObjectsCount)
	}
	// Финальная статистика будет выведена после всех сохранений

	return nil
}

// SaveAllObjectsToDBForSnapshot загружает все объекты и сохраняет их в БД один раз (для использования в scheduler)
func (s *PartnerSnapshotService) SaveAllObjectsToDBForSnapshot(
	adminAccountID uint,
	token string,
	snapshotDate time.Time,
	db *gorm.DB,
) error {
	// Увеличенный таймаут для больших объемов данных
	client := &http.Client{Timeout: 300 * time.Second}

	var allObjects []axentaObject
	page := 1
	perPage := 1000

	log.Printf("📥 Начинаем загрузку всех объектов с полными данными для сохранения в БД...")

	for {
		// Запрос к Axenta Cloud API без фильтра accountId (загружаем все объекты)
		axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/objects/?page=%d&per_page=%d",
			page, perPage)

		req, err := http.NewRequest("GET", axentaURL, nil)
		if err != nil {
			return fmt.Errorf("ошибка создания запроса: %w", err)
		}

		req.Header.Set("Authorization", "Token "+token)

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("ошибка запроса к Axenta Cloud: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if resp.StatusCode == 500 {
				log.Printf("⚠️ Ошибка сохранения объектов для компании: axenta Cloud вернул статус 500 (продолжаем)")
				break
			}
			return fmt.Errorf("axenta Cloud вернул статус %d", resp.StatusCode)
		}

		var axentaResponse struct {
			Results []axentaObject `json:"results"`
			Count   int            `json:"count"`
			Next    *string        `json:"next"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&axentaResponse); err != nil {
			resp.Body.Close()
			return fmt.Errorf("ошибка парсинга ответа: %w", err)
		}
		resp.Body.Close()

		allObjects = append(allObjects, axentaResponse.Results...)

		log.Printf("📄 Страница %d: получено %d объектов, всего загружено %d из %d",
			page, len(axentaResponse.Results), len(allObjects), axentaResponse.Count)

		if len(axentaResponse.Results) < perPage || axentaResponse.Next == nil {
			break
		}

		page++
	}

	log.Printf("✅ Загружено %d объектов с полными данными. Начинаем сохранение в БД...", len(allObjects))

	// Подсчитываем количество уникальных объектов в БД до сохранения
	var uniqueObjectsBefore int64
	db.Model(&models.AxentaObjectSnapshot{}).
		Where("DATE(last_synced_at AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
		Count(&uniqueObjectsBefore)

	// Сохраняем все объекты в БД
	if err := s.savePartnerObjectsToDB(adminAccountID, allObjects, snapshotDate, db); err != nil {
		return fmt.Errorf("ошибка сохранения объектов: %w", err)
	}

	// Подсчитываем количество уникальных объектов в БД после сохранения
	var uniqueObjectsAfter int64
	db.Model(&models.AxentaObjectSnapshot{}).
		Where("DATE(last_synced_at AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
		Count(&uniqueObjectsAfter)

	newObjectsCount := uniqueObjectsAfter - uniqueObjectsBefore
	if newObjectsCount > 0 {
		log.Printf("✅ Добавлено %d новых уникальных объектов в БД (всего загружено: %d)", newObjectsCount, len(allObjects))
	} else {
		log.Printf("ℹ️ Все объекты уже были в БД или были обновлены")
	}

	return nil
}
