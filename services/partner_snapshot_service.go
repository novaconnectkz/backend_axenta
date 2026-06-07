package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
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

	_ = waitAxentaToken(context.Background(), token) // zone#2: shared per-token rate-limit
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

	_ = waitAxentaToken(context.Background(), token) // zone#2: shared per-token rate-limit
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
		// Энфорс срока договора: не пишем снимок вне [start_date, end_date] (end_date NULL = open).
		if !contract.IsActiveOn(snapshotDate) {
			continue
		}
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
		AdminAccountID:       companyID,
		CompanyID:            companyID,
		ContractID:           0, // Нет договора
		SnapshotDate:         snapshotDate,
		PartnerCompanyID:     partnerAccountID,
		TariffPlanID:         defaultPlan.ID,
		MonthlyPrice:         monthlyPrice,
		DailyPrice:           dailyPrice,
		TotalObjectsCount:    len(objects),
		ActiveObjectsCount:   activeCount,
		DiscountType:         "none",
		DiscountPercent:      decimal.Zero,
		DiscountFixed:        decimal.Zero,
		CostBeforeDiscount:   costBeforeDiscount,
		DiscountAmount:       decimal.Zero,
		DailyCost:            dailyCost,
		Status:               "completed",
		VerifySecondaryCount: -1, // Ф1: cross-source для Axenta в Ф2; сверка через continuity-guard
		Notes:                fmt.Sprintf("Автоматический снимок партнера БЕЗ договора: %s", partnerName),
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
	return s.CreateSnapshotForContractWithTokenAndDBAndSource(contract, snapshotDate, token, db, "api") // По умолчанию API для обратной совместимости
}

// CreateSnapshotForContractWithTokenAndDBAndSource создает снимок с выбором источника данных
func (s *PartnerSnapshotService) CreateSnapshotForContractWithTokenAndDBAndSource(contract *models.Contract, snapshotDate time.Time, token string, db *gorm.DB, dataSource string) error {
	tokenPreview := token
	if len(token) > 10 {
		tokenPreview = token[:10] + "..."
	}
	log.Printf("📸 Создание снимка для договора ID=%d, number=%s на дату %s (токен: %s)",
		contract.ID, contract.Number, snapshotDate.Format("2006-01-02"), tokenPreview)

	// Проверяем, не создан ли уже снимок на эту дату (включая мягко удаленные)
	var existingSnapshot models.PartnerDailySnapshot
	if err := db.Unscoped().
		Where("contract_id = ? AND snapshot_date = ?", contract.ID, snapshotDate).
		First(&existingSnapshot).Error; err == nil {

		// Подтверждённый вручную снимок заморожен — регенерация не перезатирает
		// операторское решение (Codex critical, как в dup-key ветке ниже).
		if existingSnapshot.VerifyStatus == models.VerifyStatusApproved {
			log.Printf("🔒 Снимок договора %d на %s подтверждён вручную — пропуск регенерации",
				contract.ID, snapshotDate.Format("2006-01-02"))
			return nil
		}

		// Проверяем является ли это сегодняшним снимком
		today := time.Now().UTC()
		todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
		isToday := snapshotDate.Equal(todayStart)

		// Исторические снимки НЕ обновляются (данные за прошлые дни зафиксированы)
		// Обновляем только сегодняшний снимок (данные за сегодня могут меняться)
		// НО если пользователь явно запросил создание снимков через кнопку "Создать снимки",
		// то обновляем даже исторические снимки (принудительное обновление)
		if !isToday {
			log.Printf("ℹ️ Снимок для договора %d на дату %s уже существует (исторический), но выполняем принудительное обновление",
				contract.ID, snapshotDate.Format("2006-01-02"))
			// Продолжаем выполнение для обновления исторического снимка
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

		// Используем источник данных в зависимости от параметра dataSource
		var objectsCount, activeObjectsCount int
		var err error

		if dataSource == "db" {
			// Используем данные из БД
			objectsCount, activeObjectsCount, err = s.CountPartnerObjectsFromDB(*contract.PartnerCompanyID, snapshotDate, db)
			if err != nil {
				log.Printf("⚠️ Не удалось получить данные из БД: %v, используем API...", err)
				// Fallback: используем API если данные из БД недоступны
				dataSource = "api"
			} else {
				log.Printf("✅ Получено из БД: всего объектов=%d, активных=%d (без API запросов)", objectsCount, activeObjectsCount)
			}
		}

		if dataSource == "api" {
			// Используем API
			log.Printf("🌐 Запрос к Axenta Cloud API для партнера %d на дату %s", *contract.PartnerCompanyID, snapshotDate.Format("2006-01-02"))
			var objects []axentaObject
			objectsCount, activeObjectsCount, objects, err = s.getPartnerObjectsWithCountForDate(*contract.PartnerCompanyID, token, snapshotDate)
			if err != nil {
				log.Printf("❌ Ошибка запроса к Axenta Cloud API: %v", err)
				return fmt.Errorf("ошибка получения объектов партнера: %w", err)
			}
			log.Printf("✅ Получено из Axenta Cloud API: всего объектов=%d, активных=%d", objectsCount, activeObjectsCount)

			// Сохраняем объекты в БД
			adminAccountID := contract.AdminAccountID
			if adminAccountID == 0 {
				adminAccountID = 1
				log.Printf("⚠️ contract.AdminAccountID равен 0, используем значение по умолчанию 1")
			}
			if err := s.savePartnerObjectsToDB(adminAccountID, objects, snapshotDate, db); err != nil {
				log.Printf("⚠️ Ошибка сохранения объектов в БД (продолжаем обновление снимка): %v", err)
			}
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

		// Update-путь: BeforeCreate (где живёт ценовой расчёт + continuity-guard) на Save
		// НЕ срабатывает. Поэтому пересчитываем явно через Recompute — иначе verify-поля
		// (verify_status/amount_at_risk/delta_pct) застревают от первоначальной генерации,
		// а кнопка «Подтвердить»/предупреждение не уходят после регена. Recompute = ComputeCosts
		// (days_in_month, не хардкод /30) + applyVerifyGuard (baseline читается из tx-схемы).
		// VerifySecondaryCount=-1 до Recompute (симметрия с create/dup: cross-source Axenta
		// отложен, иначе старый secondary ложно вернёт needs_review — Codex).
		existingSnapshot.VerifySecondaryCount = -1
		existingSnapshot.Recompute(db)

		// Используем Unscoped() для сохранения, чтобы обновить даже мягко удаленную запись
		if err := db.Unscoped().Save(&existingSnapshot).Error; err != nil {
			return fmt.Errorf("ошибка обновления снимка: %w", err)
		}

		log.Printf("✅ Снимок обновлен: договор=%s, дата=%s, объектов=%d (активных=%d), стоимость=%s₽ (до скидки: %s₽)",
			contract.Number, snapshotDate.Format("2006-01-02"), objectsCount, activeObjectsCount,
			existingSnapshot.DailyCost.StringFixed(2), existingSnapshot.CostBeforeDiscount.StringFixed(2))

		return nil
	}

	// Загружаем тарифный план из глобальной БД (billing_plans в public схеме)
	var tariffPlan models.BillingPlan
	if err := s.db.
		Where("id = ?", *contract.TariffPlanID).
		First(&tariffPlan).Error; err != nil {
		return fmt.Errorf("тарифный план не найден: %w", err)
	}

	// Используем источник данных в зависимости от параметра dataSource
	var objectsCount, activeObjectsCount int
	var err error

	if dataSource == "db" {
		// Используем данные из БД
		objectsCount, activeObjectsCount, err = s.CountPartnerObjectsFromDB(*contract.PartnerCompanyID, snapshotDate, db)
		if err != nil {
			log.Printf("⚠️ Не удалось получить данные из БД: %v, используем API...", err)
			// Fallback: используем API если данные из БД недоступны
			dataSource = "api"
		} else {
			log.Printf("✅ Получено из БД: всего объектов=%d, активных=%d (без API запросов)", objectsCount, activeObjectsCount)
		}
	}

	if dataSource == "api" {
		// Используем API
		log.Printf("🌐 Запрос к Axenta Cloud API для партнера %d на дату %s (создание нового снимка)", *contract.PartnerCompanyID, snapshotDate.Format("2006-01-02"))
		var objects []axentaObject
		objectsCount, activeObjectsCount, objects, err = s.getPartnerObjectsWithCountForDate(*contract.PartnerCompanyID, token, snapshotDate)
		if err != nil {
			log.Printf("❌ Ошибка запроса к Axenta Cloud API: %v", err)
			return fmt.Errorf("ошибка получения объектов партнера: %w", err)
		}
		log.Printf("✅ Получено из Axenta Cloud API: всего объектов=%d, активных=%d", objectsCount, activeObjectsCount)

		// Сохраняем объекты в БД
		adminAccountID := contract.AdminAccountID
		if adminAccountID == 0 {
			adminAccountID = 1
			log.Printf("⚠️ contract.AdminAccountID равен 0, используем значение по умолчанию 1")
		}
		if err := s.savePartnerObjectsToDB(adminAccountID, objects, snapshotDate, db); err != nil {
			log.Printf("⚠️ Ошибка сохранения объектов в БД (продолжаем создание снимка): %v", err)
			// Не прерываем создание снимка из-за ошибки сохранения объектов
		}
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
		// Ф1: cross-source -1 (валидный второй счёт Axenta = COUNT axenta_object_snapshots
		// по дереву аккаунтов — отложен в Ф2; live-stats глобален, не per-партнёр). Сверка: continuity-guard.
		VerifySecondaryCount: -1,
	}
	// DailyPrice, CostBeforeDiscount, DiscountAmount, DailyCost + verify-поля рассчитает BeforeCreate

	if err := db.Create(&snapshot).Error; err != nil {
		// Проверяем, является ли ошибка нарушением уникального ограничения (снимок уже существует)
		if isDuplicateKeyError(err) {
			log.Printf("⚠️ Снимок для договора %d на дату %s уже существует, пытаемся обновить",
				contract.ID, snapshotDate.Format("2006-01-02"))
			// Пытаемся найти и обновить существующий снимок
			var existingSnapshot models.PartnerDailySnapshot
			if err := db.Unscoped().
				Where("contract_id = ? AND snapshot_date = ?", contract.ID, snapshotDate).
				First(&existingSnapshot).Error; err == nil {
				// Подтверждённый вручную снимок заморожен — не перезатираем (Codex critical).
				if existingSnapshot.VerifyStatus == models.VerifyStatusApproved {
					log.Printf("🔒 Axenta снимок договора %d на %s подтверждён вручную — пропуск обновления",
						contract.ID, snapshotDate.Format("2006-01-02"))
					return nil
				}
				// Обновляем существующий снимок
				existingSnapshot.TotalObjectsCount = objectsCount
				existingSnapshot.ActiveObjectsCount = activeObjectsCount
				existingSnapshot.MonthlyPrice = monthlyPrice
				existingSnapshot.DiscountType = contract.DiscountType
				existingSnapshot.DiscountPercent = discountPercent
				existingSnapshot.DiscountFixed = discountFixed
				existingSnapshot.Status = "completed"
				existingSnapshot.DeletedAt = gorm.DeletedAt{}

				// Ф1: единый пересчёт цен (days-in-month, фикс прежнего хардкода /30) +
				// сверки через модель. Save не триггерит BeforeCreate, поэтому Recompute явно.
				existingSnapshot.VerifySecondaryCount = -1
				existingSnapshot.Recompute(db)

				if err := db.Unscoped().Save(&existingSnapshot).Error; err != nil {
					return fmt.Errorf("ошибка обновления существующего снимка: %w", err)
				}
				log.Printf("✅ Существующий снимок обновлен: договор=%s, дата=%s, объектов=%d (активных=%d), стоимость=%.2f₽",
					contract.Number, snapshotDate.Format("2006-01-02"), objectsCount, activeObjectsCount, existingSnapshot.DailyCost.InexactFloat64())
				return nil
			}
		}
		return fmt.Errorf("ошибка создания снимка: %w", err)
	}

	log.Printf("✅ Снимок создан: договор=%s, дата=%s, объектов=%d (активных=%d), скидка=%s%%, стоимость=%s₽ (было %s₽)",
		contract.Number, snapshotDate.Format("2006-01-02"), objectsCount, activeObjectsCount,
		discountPercent.StringFixed(2), snapshot.DailyCost.StringFixed(2), snapshot.CostBeforeDiscount.StringFixed(2))

	return nil
}

// partnerSnapshotIsApproved — true если снимок за этот ключ уже подтверждён вручную
// (manual_approved). Cron-upsert НЕ должен его перезатирать: иначе ночной прогон снимает
// ручное подтверждение оператора и возвращает день в needs_review/verified (Codex critical).
// Подтверждённый снимок = заморожен (числа уже приняты оператором).
func partnerSnapshotIsApproved(db *gorm.DB, source string, connID uint, extID string, contractID uint, day time.Time) bool {
	var cnt int64
	db.Model(&models.PartnerDailySnapshot{}).
		Where("partner_source = ? AND connection_id = ? AND partner_external_id = ? AND contract_id = ? AND snapshot_date = ? AND verify_status = ?",
			source, connID, extID, contractID, day, models.VerifyStatusApproved).
		Count(&cnt)
	return cnt > 0
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

		_ = waitAxentaToken(context.Background(), token) // zone#2: shared per-token rate-limit
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

// getPartnerObjectsWithCountForDate получает объекты партнера на определенную дату и их количество
// Возвращает: количество, активное количество, список объектов, ошибка
func (s *PartnerSnapshotService) getPartnerObjectsWithCountForDate(partnerCompanyID uint, token string, snapshotDate time.Time) (total int, active int, objects []axentaObject, err error) {
	// Увеличенный таймаут для больших объемов данных (до 5000+ объектов)
	client := &http.Client{Timeout: 180 * time.Second}

	var allObjects []axentaObject

	page := 1
	perPage := 1000

	log.Printf("🌐 Начинаем загрузку объектов из Axenta Cloud API для партнера %d на дату %s", partnerCompanyID, snapshotDate.Format("2006-01-02"))

	for {
		// Запрос к Axenta Cloud API с фильтром accountId
		// API автоматически включает объекты всех дочерних аккаунтов партнера
		axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/objects/?accountId=%d&page=%d&per_page=%d",
			partnerCompanyID, page, perPage)

		log.Printf("📡 Запрос к Axenta Cloud API: %s (страница %d)", axentaURL, page)

		req, err := http.NewRequest("GET", axentaURL, nil)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("ошибка создания запроса: %w", err)
		}

		// Используем переданный токен
		if token == "" {
			return 0, 0, nil, fmt.Errorf("токен не предоставлен для запроса к Axenta Cloud API")
		}
		tokenPreview := token
		if len(token) > 10 {
			tokenPreview = token[:10] + "..."
		}
		req.Header.Set("Authorization", "Token "+token)
		log.Printf("🔑 Используется токен: %s", tokenPreview)

		_ = waitAxentaToken(context.Background(), token) // zone#2: shared per-token rate-limit
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

		log.Printf("♻️ Обновлен снимок 'Прочие': всего=%d, активных=%d, стоимость=%s₽",
			totalObjects, activeObjects, costBeforeDiscount.StringFixed(2))
		return nil
	}

	// Создаем новый снимок
	dailyRate := defaultPlan.Price.Div(decimal.NewFromInt(30))
	costBeforeDiscount := dailyRate.Mul(decimal.NewFromInt(int64(activeObjects)))

	snapshot := models.PartnerDailySnapshot{
		AdminAccountID:       companyID,
		CompanyID:            companyID,
		ContractID:           0, // Нет договора
		SnapshotDate:         snapshotDate,
		PartnerCompanyID:     virtualPartnerID, // 0 = виртуальный партнер "Прочие"
		TariffPlanID:         defaultPlan.ID,
		MonthlyPrice:         defaultPlan.Price,
		DailyPrice:           dailyRate,
		TotalObjectsCount:    totalObjects,
		ActiveObjectsCount:   activeObjects,
		DiscountType:         "none",
		DiscountPercent:      decimal.Zero,
		DiscountFixed:        decimal.Zero,
		CostBeforeDiscount:   costBeforeDiscount,
		DiscountAmount:       decimal.Zero,
		DailyCost:            costBeforeDiscount,
		Status:               "completed",
		VerifySecondaryCount: -1, // Ф1: cross-source для Axenta в Ф2; сверка через continuity-guard
		Notes:                "Учётная запись GLOMOS (186) - прямые клиенты без партнёров в иерархии",
	}

	if err := tenantDB.Create(&snapshot).Error; err != nil {
		return fmt.Errorf("ошибка создания снимка 'Прочие': %w", err)
	}

	log.Printf("✨ Создан снимок 'Прочие': всего=%d, активных=%d, стоимость=%s₽",
		totalObjects, activeObjects, costBeforeDiscount.StringFixed(2))

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
		AdminAccountID:       companyID,
		CompanyID:            companyID,
		ContractID:           contractID,
		SnapshotDate:         snapshotDate,
		PartnerCompanyID:     partnerID,
		TariffPlanID:         tariffPlan.ID,
		MonthlyPrice:         monthlyPrice,
		DailyPrice:           dailyRate,
		TotalObjectsCount:    totalObjects,
		ActiveObjectsCount:   activeObjects,
		DiscountType:         discountType,
		DiscountPercent:      discountPercent,
		DiscountFixed:        discountFixed,
		CostBeforeDiscount:   costBeforeDiscount,
		DiscountAmount:       discountAmount,
		DailyCost:            dailyCost,
		Status:               "completed",
		VerifySecondaryCount: -1, // Ф1: cross-source для Axenta в Ф2; сверка через continuity-guard
		Notes:                notes,
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

	// Нормализуем даты: начало дня для startDate, конец дня для endDate (в UTC)
	startDateUTC := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
	endDateUTC := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, time.UTC)

	// Используем сравнение по дате (без учета часового пояса) для надежности
	// Это гарантирует, что снимки будут найдены независимо от часового пояса в БД
	// Используем DATE() для извлечения даты из timestamp, что работает корректно с любым часовым поясом
	if err := s.db.
		Where("contract_id = ? AND DATE(snapshot_date) >= DATE(?) AND DATE(snapshot_date) <= DATE(?)",
			contractID, startDateUTC, endDateUTC).
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
// snapshotDBStmtTimeout — верхний deadline на ОДНУ DB-операцию snapshot-сохранения
// (zone#1 hardening). Защита от вечного зависания per-row UPSERT на PG-локе: HTTP-таймауты
// покрывают только Axenta-вызовы, не БД. Щедрый дефолт 120s → на нормальных прогонах не
// срабатывает; застрявший lock/запрос аборт через context → строка errorCount++/continue
// (не фатально, ретрай след. прогоном). Override env SNAPSHOT_DB_STMT_TIMEOUT_SEC.
func snapshotDBStmtTimeout() time.Duration {
	if v := os.Getenv("SNAPSHOT_DB_STMT_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 120 * time.Second
}

func (s *PartnerSnapshotService) savePartnerObjectsToDB(
	adminAccountID uint,
	objects []axentaObject,
	snapshotDate time.Time,
	db *gorm.DB,
) error {
	savedCount := 0
	errorCount := 0
	timeoutCount := 0 // zone#1/Codex#2: timeout-аборты считаем отдельно — кэш неполон, биллинг-DB-count недостоверен
	stmtTimeout := snapshotDBStmtTimeout()

	// Защита от нулевого adminAccountID (используем 1 по умолчанию)
	if adminAccountID == 0 {
		log.Printf("⚠️ adminAccountID равен 0, используем значение по умолчанию 1")
		adminAccountID = 1
	}

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
			// AdminAccountID обязателен для NOT NULL поля в БД (хотя объекты хранятся глобально)
			AdminAccountID:    &adminAccountID,
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
		// Привязка к партнерам через account_external_id и иерархию.
		// Per-row deadline (zone#1): застрявший на PG-локе UPSERT аборт через context,
		// а не вечный hang всего прогона (источник #5). cancel() сразу — без defer в цикле.
		ctx, cancel := context.WithTimeout(context.Background(), stmtTimeout)
		err := db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "external_object_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"admin_account_id", "account_external_id", "object_name", "unique_id", "device_type_name", "account_name",
				"status", "is_active", "last_synced_at", "raw_payload", "last_communication_at",
				"creator_name", "creator_id", "creator_is_active", "account_is_active",
				"phone_numbers", "axenta_created_at", "axenta_deleted_at",
			}),
		}).Create(&snapshot).Error
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				timeoutCount++
				log.Printf("⛔ Timeout (PG-lock?) сохранения объекта %d: %v", obj.ID, err)
			} else {
				log.Printf("⚠️ Ошибка сохранения объекта %d: %v", obj.ID, err)
			}
			errorCount++
			continue
		}

		savedCount++
	}

	if errorCount > 0 {
		log.Printf("⚠️ При сохранении объектов возникло ошибок: %d (обработано: %d)", errorCount, savedCount)
	}
	// Codex#2: timeout-аборт = неполный DB-кэш. Возвращаем ошибку, чтобы вызывающий НЕ
	// доверял DB-count (CountPartnerObjectsFromDB) как полному. Обычные per-row ошибки
	// остаются толерантными (исторически nil). API-путь логирует+продолжает (count из API,
	// безвреден); accumulation-путь пробрасывает; scheduler пока глотает (см. TODO).
	if timeoutCount > 0 {
		return fmt.Errorf("snapshot DB-кэш неполон: %d объектов аборчены по timeout (PG-lock, zone#1) — DB-count недостоверен", timeoutCount)
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
	effectiveAdminAccountID := adminAccountID
	if effectiveAdminAccountID == 0 {
		effectiveAdminAccountID = 1 // Значение по умолчанию
		log.Printf("⚠️ adminAccountID равен 0 в SavePartnerObjectsToDBForSnapshot, используем значение по умолчанию 1")
	}
	if err := s.savePartnerObjectsToDB(effectiveAdminAccountID, objects, snapshotDate, db); err != nil {
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

		_ = waitAxentaToken(context.Background(), token) // zone#2: shared per-token rate-limit
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
	effectiveAdminAccountID := adminAccountID
	if effectiveAdminAccountID == 0 {
		effectiveAdminAccountID = 1 // Значение по умолчанию
		log.Printf("⚠️ adminAccountID равен 0 в SaveAllObjectsToDBForSnapshot, используем значение по умолчанию 1")
	}
	if err := s.savePartnerObjectsToDB(effectiveAdminAccountID, allObjects, snapshotDate, db); err != nil {
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

// CountPartnerObjectsFromDB считает объекты партнера из БД на определенную дату (без API запросов)
// Использует данные из axenta_object_snapshots и axenta_account_snapshots
func (s *PartnerSnapshotService) CountPartnerObjectsFromDB(
	partnerCompanyID uint,
	snapshotDate time.Time,
	tenantDB *gorm.DB,
) (totalObjects int, activeObjects int, err error) {
	// Нормализуем дату снимка (начало дня UTC)
	snapshotStart := time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 0, 0, 0, 0, time.UTC)
	snapshotEnd := snapshotStart.AddDate(0, 0, 1)

	// Находим аккаунт партнера
	var partnerAccount models.AxentaAccountSnapshot
	if err := tenantDB.
		Where("external_account_id = ? AND is_active = ?", int64(partnerCompanyID), true).
		First(&partnerAccount).Error; err != nil {
		// Если аккаунт не найден, возвращаем 0 (партнер может не существовать на эту дату)
		if err == gorm.ErrRecordNotFound {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("ошибка поиска аккаунта партнера %d: %w", partnerCompanyID, err)
	}

	// Получаем все дочерние аккаунты партнера через иерархию
	// Иерархия хранится в формате: "GLOMOS > PartnerName > Child1 > Child2"
	// Ищем все аккаунты, у которых партнер является родителем
	partnerAccountName := partnerAccount.AccountName
	var childAccounts []models.AxentaAccountSnapshot

	// Ищем аккаунты, где партнер является родителем (ParentAccountName = имя партнера)
	// Или где в иерархии содержится путь через партнера (более точный поиск)
	// Используем паттерн " > PartnerName > " чтобы избежать ложных срабатываний
	hierarchyPattern := "% > " + partnerAccountName + " > %"
	// БЕЗ фильтра is_active на суб-аккаунтах: деактивированный аккаунт партнёра
	// может держать живые активные объекты (Axenta их считает и биллит). Фильтруем
	// активность на уровне ОБЪЕКТА ниже, не на уровне аккаунта.
	if err := tenantDB.
		Where("(parent_account_name = ? OR hierarchy LIKE ? OR hierarchy = ?)",
			partnerAccountName, hierarchyPattern, partnerAccountName).
		Find(&childAccounts).Error; err != nil {
		return 0, 0, fmt.Errorf("ошибка поиска дочерних аккаунтов: %w", err)
	}

	// Собираем все account_external_id для поиска объектов (рекурсивно включая всех потомков)
	accountIDs := make(map[int64]bool)
	accountIDs[int64(partnerCompanyID)] = true // Сам партнер

	// Добавляем прямые дочерние аккаунты
	for _, acc := range childAccounts {
		accountIDs[acc.ExternalAccountID] = true
	}

	// Рекурсивно ищем всех потомков дочерних аккаунтов
	// Используем итеративный подход для поиска всех уровней вложенности
	processedAccounts := make(map[int64]bool)
	toProcess := make([]int64, 0)
	for _, acc := range childAccounts {
		toProcess = append(toProcess, acc.ExternalAccountID)
	}

	for len(toProcess) > 0 {
		currentID := toProcess[0]
		toProcess = toProcess[1:]

		if processedAccounts[currentID] {
			continue
		}
		processedAccounts[currentID] = true

		// Ищем дочерние аккаунты текущего аккаунта
		var currentAccount models.AxentaAccountSnapshot
		if err := tenantDB.Where("external_account_id = ?", currentID).First(&currentAccount).Error; err == nil {
			var nestedChildren []models.AxentaAccountSnapshot
			nestedPattern := "% > " + currentAccount.AccountName + " > %"
			if err := tenantDB.
				Where("(parent_account_name = ? OR hierarchy LIKE ?)",
					currentAccount.AccountName, nestedPattern).
				Find(&nestedChildren).Error; err == nil {
				for _, nested := range nestedChildren {
					if !accountIDs[nested.ExternalAccountID] {
						accountIDs[nested.ExternalAccountID] = true
						toProcess = append(toProcess, nested.ExternalAccountID)
					}
				}
			}
		}
	}

	// Преобразуем map в slice для SQL запроса
	accountIDList := make([]int64, 0, len(accountIDs))
	for id := range accountIDs {
		accountIDList = append(accountIDList, id)
	}

	if len(accountIDList) == 0 {
		return 0, 0, nil
	}

	// Считаем объекты из axenta_object_snapshots, которые:
	// 1. Принадлежат аккаунтам партнера (account_external_id IN (...))
	// 2. Были активны на дату снимка:
	//    - axenta_created_at <= snapshotEnd (созданы до конца дня снимка)
	//    - (axenta_deleted_at IS NULL OR axenta_deleted_at >= snapshotEnd) (не удалены до конца дня снимка)
	//    - ИЛИ если axenta_created_at NULL, используем created_at
	var totalCount, activeCount int64

	// Общее количество объектов (все, которые существовали на дату снимка)
	if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("account_external_id IN ?", accountIDList).
		Where("(axenta_created_at IS NOT NULL AND axenta_created_at < ?) OR (axenta_created_at IS NULL AND created_at < ?)", snapshotEnd, snapshotEnd).
		Where("(axenta_deleted_at IS NULL OR axenta_deleted_at >= ?)", snapshotEnd).
		Count(&totalCount).Error; err != nil {
		return 0, 0, fmt.Errorf("ошибка подсчета всех объектов: %w", err)
	}

	// Активные объекты (is_active = true на дату снимка)
	// Для определения активности на дату снимка используем поле is_active из последнего синхронизированного состояния
	// Но более точно - объекты, которые были активны на момент снимка
	if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("account_external_id IN ?", accountIDList).
		Where("is_active = ?", true).
		Where("(axenta_created_at IS NOT NULL AND axenta_created_at < ?) OR (axenta_created_at IS NULL AND created_at < ?)", snapshotEnd, snapshotEnd).
		Where("(axenta_deleted_at IS NULL OR axenta_deleted_at >= ?)", snapshotEnd).
		Count(&activeCount).Error; err != nil {
		return 0, 0, fmt.Errorf("ошибка подсчета активных объектов: %w", err)
	}

	return int(totalCount), int(activeCount), nil
}

// RelinkOrphanPartnerSnapshots привязывает существующие снимки партнёра «без договора»
// (contract_id=0) к только что созданному партнёрскому договору: для каждой даты
// генерирует contract-linked снимок (data_source=db, без Axenta-токена — Recompute +
// continuity-guard + freeze approved внутри) и мягко удаляет orphan-ряд cid=0.
// Вызывается в горутине после создания договора. schema — имя tenant-схемы из
// контекста запроса (contract.CompanyID может быть 0, поэтому схему передаём явно).
//
// ВАЖНО (деньги): релинкается ВСЯ история «без договора» партнёра. Безопасно, потому
// что CalculateBillingForPartnerContract КЛЕМПИТ period_start снизу к contract.StartDate
// (даже если к договору привязаны более ранние снимки — они не попадут в счёт).
// Цель — чтобы «без договора» в справочнике сменилось номером договора по всей истории.
func RelinkOrphanPartnerSnapshots(contract models.Contract, schema string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ ПАНИКА в RelinkOrphanPartnerSnapshots (договор %d): %v", contract.ID, r)
		}
	}()

	if contract.ContractType != "partner" || contract.PartnerCompanyID == nil ||
		*contract.PartnerCompanyID == 0 || contract.TariffPlanID == nil {
		return
	}
	// Пустая/public схема → ConnectToTenant отдаёт глобальный DB (fail-open в public). Не работаем.
	if schema == "" || schema == "public" {
		log.Printf("⚠️ Релинк orphan: некорректная схема %q (договор %s) — пропуск", schema, contract.Number)
		return
	}
	tenantDB, err := database.ConnectToTenant(schema)
	if err != nil {
		log.Printf("⚠️ Релинк orphan: нет подключения к схеме %s: %v", schema, err)
		return
	}

	const relinkCap = 400
	var orphans []models.PartnerDailySnapshot
	if err := tenantDB.
		Where("partner_company_id = ? AND contract_id = 0 AND deleted_at IS NULL", *contract.PartnerCompanyID).
		Order("snapshot_date ASC").
		Limit(relinkCap).
		Find(&orphans).Error; err != nil {
		log.Printf("⚠️ Релинк orphan: ошибка выборки снимков партнёра %d: %v", *contract.PartnerCompanyID, err)
		return
	}
	if len(orphans) == 0 {
		return
	}
	if len(orphans) == relinkCap {
		log.Printf("⚠️ Релинк orphan партнёра %d: достигнут лимит %d — часть старых снимков не релинкнута",
			*contract.PartnerCompanyID, relinkCap)
	}

	svc := NewPartnerSnapshotService()
	relinked := 0
	for i := range orphans {
		// Подтверждённый вручную orphan не трогаем — операторское решение важнее авто-релинка.
		if orphans[i].VerifyStatus == models.VerifyStatusApproved {
			continue
		}
		date := orphans[i].SnapshotDate
		orphanID := orphans[i].ID
		// Per-date транзакция: создание contract-ряда + удаление orphan атомарны.
		// При гонке (второй договор того же партнёра) delete вернёт RowsAffected=0 →
		// откатываем создание этого ряда (orphan уже забрал первый релинк). Падение
		// процесса между шагами не оставит дубль — оба или ни одного.
		txErr := tenantDB.Transaction(func(tx *gorm.DB) error {
			if e := svc.CreateSnapshotForContractWithTokenAndDBAndSource(&contract, date, "", tx, "db"); e != nil {
				return e
			}
			// verify_status <> approved прямо в DELETE: если оператор подтвердил orphan
			// между пред-проверкой и этим моментом — RowsAffected=0 → откат (TOCTOU-guard).
			res := tx.Where("id = ? AND contract_id = 0 AND verify_status <> ?", orphanID, models.VerifyStatusApproved).
				Delete(&models.PartnerDailySnapshot{})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("orphan %d уже обработан/подтверждён (гонка) — откат", orphanID)
			}
			return nil
		})
		if txErr != nil {
			log.Printf("⚠️ Релинк orphan: договор %s на %s — %v", contract.Number, date.Format("2006-01-02"), txErr)
			continue // orphan оставляем — данные не теряем
		}
		relinked++
	}
	log.Printf("✅ Релинк orphan-снимков партнёра %d → договор %s: %d/%d привязано",
		*contract.PartnerCompanyID, contract.Number, relinked, len(orphans))
}

// AutoCalculateBillingForAllPartners автоматически запускает расчет биллинга за весь период
// для всех партнеров всех компаний, если еще нет снимков
// Вызывается после первой успешной синхронизации данных
func AutoCalculateBillingForAllPartners() {
	// Запускаем в отдельной горутине, чтобы не блокировать синхронизацию
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в AutoCalculateBillingForAllPartners: %v", r)
			}
		}()

		log.Println("🤖 Автоматический расчет биллинга: проверка необходимости расчета...")

		// Проверяем, есть ли уже снимки в БД
		mainDB := database.DB.Session(&gorm.Session{})
		if err := mainDB.Exec("SET search_path TO public").Error; err != nil {
			log.Printf("⚠️ Автоматический расчет биллинга: ошибка переключения схемы: %v", err)
			return
		}

		// Проверяем наличие снимков во всех компаниях.
		// Ф3-F-followup/Codex Q2: ORDER BY id ASC для детерминизма
		// companies[0]=firstCompany (синхронно с axenta_sync_service.go:63 и
		// api/partner_snapshots.go snapshotAdminKey=MIN(id)). Без ORDER —
		// heap-order, потенциальный mismatch tenant-схемы snapshot_settings.
		var companies []models.Company
		if err := mainDB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").Find(&companies).Error; err != nil {
			log.Printf("⚠️ Автоматический расчет биллинга: ошибка получения компаний: %v", err)
			return
		}

		hasAnySnapshots := false
		for _, company := range companies {
			tenantDB := database.GetTenantDBByID(company.ID)
			if tenantDB == nil {
				continue
			}

			var count int64
			if err := tenantDB.Model(&models.PartnerDailySnapshot{}).Count(&count).Error; err == nil && count > 0 {
				hasAnySnapshots = true
				break
			}
		}

		if hasAnySnapshots {
			log.Println("ℹ️ Автоматический расчет биллинга: снимки уже существуют, пропускаем расчет")
			return
		}

		log.Println("🚀 Автоматический расчет биллинга: снимков нет, запускаем расчет за весь период...")

		// Определяем период: от 2024-03-14 (начало данных) до текущей даты
		dateFrom := time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)
		dateTo := time.Now().UTC().AddDate(0, 0, -1) // Вчерашний день

		log.Printf("📅 Период расчета: %s - %s", dateFrom.Format("2006-01-02"), dateTo.Format("2006-01-02"))

		// Создаем запись о задаче
		job := &models.SnapshotJob{
			JobType:     models.SnapshotJobTypeManual,
			StartedAt:   time.Now(),
			DateFrom:    dateFrom,
			DateTo:      dateTo,
			Status:      models.SnapshotJobStatusRunning,
			TriggeredBy: "auto_after_sync",
		}
		if err := mainDB.Create(job).Error; err != nil {
			log.Printf("⚠️ Автоматический расчет биллинга: ошибка создания записи: %v", err)
			return
		}

		// Получаем токен для всех компаний
		systemToken := os.Getenv("AXENTA_ADMIN_TOKEN")
		if systemToken == "" {
			// Пытаемся получить токен из настроек
			if len(companies) > 0 {
				firstCompany := companies[0]
				tenantDB := database.GetTenantDBByID(firstCompany.ID)
				if tenantDB != nil {
					var settings models.SnapshotSettings
					if err := tenantDB.Where("company_id = ? AND is_active = ?", 1, true).First(&settings).Error; err == nil {
						systemToken = settings.AxentaToken
					}
				}
			}
		}

		if systemToken == "" {
			log.Printf("⚠️ Автоматический расчет биллинга: токен не найден, пропускаем расчет")
			job.FinishJob(models.SnapshotJobStatusFailed, "токен не найден")
			_ = mainDB.Save(job).Error
			return
		}

		snapshotService := NewPartnerSnapshotService()
		totalSuccess := 0
		totalErrors := 0
		totalContracts := 0

		// Для каждой компании
		for _, company := range companies {
			tenantDB := database.GetTenantDBByID(company.ID)
			if tenantDB == nil {
				continue
			}

			// Получаем все активные партнерские договоры
			var contracts []models.Contract
			if err := tenantDB.
				Where("contract_type = ? AND status = ?", "partner", "active").
				Where("partner_company_id IS NOT NULL").
				Find(&contracts).Error; err != nil {
				log.Printf("⚠️ Автоматический расчет биллинга: ошибка получения договоров для компании %d: %v", company.ID, err)
				continue
			}

			if len(contracts) == 0 {
				log.Printf("ℹ️ Автоматический расчет биллинга: нет активных договоров для компании %d", company.ID)
				continue
			}

			log.Printf("📊 Компания %d: найдено %d активных партнерских договоров", company.ID, len(contracts))

			// Для каждого договора и каждой даты создаем снимок
			// Используем данные из БД для быстрой работы
			for _, contract := range contracts {
				if contract.PartnerCompanyID == nil {
					continue
				}

				totalContracts++
				for d := dateFrom; !d.After(dateTo); d = d.AddDate(0, 0, 1) {
					// Используем БД по умолчанию для автоматического расчета (быстро)
					if err := snapshotService.CreateSnapshotForContractWithTokenAndDBAndSource(&contract, d, systemToken, tenantDB, "db"); err != nil {
						if err.Error() != "snapshot already exists" {
							totalErrors++
							log.Printf("⚠️ Ошибка создания снимка для договора %d на дату %s: %v", contract.ID, d.Format("2006-01-02"), err)
						}
					} else {
						totalSuccess++
					}
				}
			}
		}

		// Обновляем задачу
		job.TotalContracts = totalContracts
		job.SuccessCount = totalSuccess
		job.ErrorCount = totalErrors
		job.TotalDaysProcessed = totalSuccess + totalErrors

		status := models.SnapshotJobStatusCompleted
		errMsg := ""
		if totalErrors > 0 {
			status = models.SnapshotJobStatusPartial
			errMsg = fmt.Sprintf("Создано %d снимков, ошибок: %d", totalSuccess, totalErrors)
		}

		job.FinishJob(status, errMsg)
		if err := mainDB.Save(job).Error; err != nil {
			log.Printf("⚠️ Автоматический расчет биллинга: ошибка сохранения задачи: %v", err)
		} else {
			log.Printf("✅ Автоматический расчет биллинга завершен: создано %d снимков, ошибок: %d", totalSuccess, totalErrors)
		}
	}()
}
