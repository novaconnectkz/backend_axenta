package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// PartnerSnapshotService управляет ежедневными снимками партнерских объектов
type PartnerSnapshotService struct {
	db *gorm.DB
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
	log.Printf("📸 Создание снимка для договора ID=%d, number=%s на дату %s", 
		contract.ID, contract.Number, snapshotDate.Format("2006-01-02"))

	// Проверяем, не создан ли уже снимок на эту дату
	var existingSnapshot models.PartnerDailySnapshot
	if err := s.db.
		Where("contract_id = ? AND snapshot_date = ?", contract.ID, snapshotDate).
		First(&existingSnapshot).Error; err == nil {
		log.Printf("ℹ️ Снимок для договора %d на дату %s уже существует, обновляем...", 
			contract.ID, snapshotDate.Format("2006-01-02"))
		
		// Обновляем существующий снимок вместо создания нового
		// Загружаем тарифный план
		var tariffPlan models.BillingPlan
		if err := s.db.
			Where("id = ?", *contract.TariffPlanID).
			First(&tariffPlan).Error; err != nil {
			return fmt.Errorf("тарифный план не найден: %w", err)
		}

		// Получаем объекты партнера из Axenta Cloud API на дату снимка
		objectsCount, activeObjectsCount, err := s.getPartnerObjectsCountForDate(*contract.PartnerCompanyID, token, snapshotDate)
		if err != nil {
			return fmt.Errorf("ошибка получения объектов партнера: %w", err)
		}

		// Расчет цен
		monthlyPrice := tariffPlan.Price
		dailyPrice := monthlyPrice.Div(decimal.NewFromInt(30)).Round(4)
		dailyCost := dailyPrice.Mul(decimal.NewFromInt(int64(activeObjectsCount))).Round(2)

		// Обновляем снимок
		existingSnapshot.TotalObjectsCount = objectsCount
		existingSnapshot.ActiveObjectsCount = activeObjectsCount
		existingSnapshot.MonthlyPrice = monthlyPrice
		existingSnapshot.DailyPrice = dailyPrice
		existingSnapshot.DailyCost = dailyCost
		existingSnapshot.Status = "completed"

		if err := s.db.Save(&existingSnapshot).Error; err != nil {
			return fmt.Errorf("ошибка обновления снимка: %w", err)
		}

		log.Printf("✅ Снимок обновлен: договор=%s, дата=%s, объектов=%d (активных=%d), цена/день=%.4f₽, стоимость=%.2f₽",
			contract.Number, snapshotDate.Format("2006-01-02"), objectsCount, activeObjectsCount, dailyPrice, dailyCost)

		return nil
	}

	// Загружаем тарифный план
	var tariffPlan models.BillingPlan
	if err := s.db.
		Where("id = ?", *contract.TariffPlanID).
		First(&tariffPlan).Error; err != nil {
		return fmt.Errorf("тарифный план не найден: %w", err)
	}

	// Получаем объекты партнера из Axenta Cloud API на дату снимка
	objectsCount, activeObjectsCount, err := s.getPartnerObjectsCountForDate(*contract.PartnerCompanyID, token, snapshotDate)
	if err != nil {
		return fmt.Errorf("ошибка получения объектов партнера: %w", err)
	}

	// Расчет цен
	monthlyPrice := tariffPlan.Price
	dailyPrice := monthlyPrice.Div(decimal.NewFromInt(30))
	dailyCost := dailyPrice.Mul(decimal.NewFromInt(int64(activeObjectsCount)))

	// Создаем снимок
	snapshot := models.PartnerDailySnapshot{
		AdminAccountID:     contract.AdminAccountID,
		CompanyID:          contract.CompanyID,
		ContractID:         contract.ID,
		SnapshotDate:       snapshotDate,
		PartnerCompanyID:   *contract.PartnerCompanyID,
		TariffPlanID:       *contract.TariffPlanID,
		MonthlyPrice:       monthlyPrice,
		DailyPrice:         dailyPrice,
		TotalObjectsCount:  objectsCount,
		ActiveObjectsCount: activeObjectsCount,
		DailyCost:          dailyCost,
		Status:             "completed",
	}

	if err := s.db.Create(&snapshot).Error; err != nil {
		return fmt.Errorf("ошибка создания снимка: %w", err)
	}

	log.Printf("✅ Снимок создан: договор=%s, дата=%s, объектов=%d (активных=%d), цена/день=%.4f₽, стоимость=%.2f₽",
		contract.Number, snapshotDate.Format("2006-01-02"), objectsCount, activeObjectsCount, dailyPrice, dailyCost)

	return nil
}

// getPartnerObjectsCountWithToken получает количество объектов партнера из Axenta Cloud с указанным токеном
// Учитывает дату снимка для фильтрации объектов по дате создания
func (s *PartnerSnapshotService) getPartnerObjectsCountWithToken(partnerCompanyID uint, token string) (total int, active int, err error) {
	// Для обратной совместимости - используем текущую дату
	return s.getPartnerObjectsCountForDate(partnerCompanyID, token, time.Now())
}

// getPartnerObjectsCountForDate получает количество объектов партнера на определенную дату
// Фильтрует объекты: учитываются только те, что были созданы до или в день снимка
func (s *PartnerSnapshotService) getPartnerObjectsCountForDate(partnerCompanyID uint, token string, snapshotDate time.Time) (total int, active int, err error) {
	client := &http.Client{Timeout: 30 * time.Second}
	
	var allObjects []struct {
		ID        uint   `json:"id"`
		Name      string `json:"name"`
		IsActive  bool   `json:"isActive"`
		CreatedAt string `json:"createdAt"`
		DeletedAt string `json:"deletedAt"`
	}

	page := 1
	perPage := 1000

	for {
		// Запрос к Axenta Cloud API с пагинацией
		axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/objects/?accountId=%d&page=%d&per_page=%d", 
			partnerCompanyID, page, perPage)
		
		req, err := http.NewRequest("GET", axentaURL, nil)
		if err != nil {
			return 0, 0, fmt.Errorf("ошибка создания запроса: %w", err)
		}

		// Используем переданный токен
		req.Header.Set("Authorization", "Token "+token)

		resp, err := client.Do(req)
		if err != nil {
			return 0, 0, fmt.Errorf("ошибка запроса к Axenta Cloud: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return 0, 0, fmt.Errorf("Axenta Cloud вернул статус %d", resp.StatusCode)
		}

		var axentaResponse struct {
			Results []struct {
				ID        uint   `json:"id"`
				Name      string `json:"name"`
				IsActive  bool   `json:"isActive"`
				CreatedAt string `json:"createdAt"`
				DeletedAt string `json:"deletedAt"`
			} `json:"results"`
			Count int     `json:"count"`
			Next  *string `json:"next"` // URL следующей страницы (если есть)
		}

		if err := json.NewDecoder(resp.Body).Decode(&axentaResponse); err != nil {
			resp.Body.Close()
			return 0, 0, fmt.Errorf("ошибка парсинга ответа: %w", err)
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

	// Подсчитываем объекты с учетом даты снимка
	total = 0
	active = 0
	
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
			total++
			if obj.IsActive {
				active++
			}
		}
	}

	log.Printf("✅ Объектов партнера %d на дату %s: %d (активных: %d, неактивных: %d)", 
		partnerCompanyID, snapshotDate.Format("2006-01-02"), total, active, total-active)

	return total, active, nil
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

