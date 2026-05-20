package api

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// LoadProgressStatus хранит статус загрузки объектов
type LoadProgressStatus struct {
	IsLoading    bool       `json:"is_loading"`
	Progress     float64    `json:"progress"`     // Процент выполнения (0-100)
	Loaded       int        `json:"loaded"`       // Количество загруженных объектов
	Total        int        `json:"total"`        // Общее количество объектов
	CurrentPage  int        `json:"current_page"` // Текущая страница
	TotalPages   int        `json:"total_pages"`  // Всего страниц
	Status       string     `json:"status"`       // Статус: "loading", "saving", "completed", "error"
	ErrorMessage string     `json:"error_message,omitempty"`
	StartTime    *time.Time `json:"start_time,omitempty"`
	LastUpdate   time.Time  `json:"last_update"`
}

var (
	loadProgressMutex sync.RWMutex
	loadProgress      *LoadProgressStatus
)

// updateLoadProgress обновляет статус загрузки
func updateLoadProgress(status LoadProgressStatus) {
	loadProgressMutex.Lock()
	defer loadProgressMutex.Unlock()
	status.LastUpdate = time.Now()
	loadProgress = &status
}

// getLoadProgress возвращает текущий статус загрузки
func getLoadProgress() *LoadProgressStatus {
	loadProgressMutex.RLock()
	defer loadProgressMutex.RUnlock()
	if loadProgress == nil {
		return &LoadProgressStatus{
			IsLoading: false,
			Progress:  0,
			Status:    "idle",
		}
	}
	// Создаем копию для безопасного возврата
	status := *loadProgress
	return &status
}

// GetPartnerContractSnapshots получает ежедневные снимки для партнерского договора
func GetPartnerContractSnapshots(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	contractID := c.Param("contract_id")
	log.Printf("📊 Запрос снимков для партнерского договора ID=%s", contractID)

	// Получаем параметры периода
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	log.Printf("📅 Получены параметры периода: start_date=%s, end_date=%s", startDateStr, endDateStr)

	// Парсим даты
	// Поддерживаем два формата: RFC3339 (с временем) и YYYY-MM-DD (только дата)
	var startDate, endDate time.Time
	if startDateStr != "" {
		log.Printf("📅 Парсинг start_date: %s", startDateStr)
		// Сначала пробуем парсить как YYYY-MM-DD (более простой формат, который использует фронтенд)
		parsedDate, parseErr := time.Parse("2006-01-02", startDateStr)
		if parseErr == nil {
			// Успешно распарсили как YYYY-MM-DD, устанавливаем начало дня в UTC
			startDate = time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, time.UTC)
			log.Printf("✅ start_date распарсен как YYYY-MM-DD: %s", startDate.Format(time.RFC3339))
		} else {
			// Если не получилось, пробуем парсить как RFC3339 (с временем и часовым поясом)
			startDate, err = time.Parse(time.RFC3339, startDateStr)
			if err != nil {
				log.Printf("❌ Ошибка парсинга start_date: %v (значение: %s)", err, startDateStr)
				c.JSON(http.StatusBadRequest, gin.H{
					"status": "error",
					"error":  fmt.Sprintf("Неверный формат start_date: %s (ожидается YYYY-MM-DD или RFC3339)", startDateStr),
				})
				return
			}
			log.Printf("✅ start_date распарсен как RFC3339: %s", startDate.Format(time.RFC3339))
		}
	} else {
		// По умолчанию - последние 30 дней
		startDate = time.Now().AddDate(0, 0, -30)
	}

	if endDateStr != "" {
		log.Printf("📅 Парсинг end_date: %s", endDateStr)
		// Сначала пробуем парсить как YYYY-MM-DD (более простой формат, который использует фронтенд)
		parsedDate, parseErr := time.Parse("2006-01-02", endDateStr)
		if parseErr == nil {
			// Успешно распарсили как YYYY-MM-DD, устанавливаем конец дня в UTC
			endDate = time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 23, 59, 59, 999999999, time.UTC)
			log.Printf("✅ end_date распарсен как YYYY-MM-DD: %s (конец дня)", endDate.Format(time.RFC3339))
		} else {
			// Если не получилось, пробуем парсить как RFC3339 (с временем и часовым поясом)
			endDate, err = time.Parse(time.RFC3339, endDateStr)
			if err != nil {
				log.Printf("❌ Ошибка парсинга end_date: %v (значение: %s)", err, endDateStr)
				c.JSON(http.StatusBadRequest, gin.H{
					"status": "error",
					"error":  fmt.Sprintf("Неверный формат end_date: %s (ожидается YYYY-MM-DD или RFC3339)", endDateStr),
				})
				return
			}
			// Если endDate имеет время 00:00:00 (начало дня), добавляем время до конца дня
			if endDate.Hour() == 0 && endDate.Minute() == 0 && endDate.Second() == 0 {
				endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, endDate.Location())
			}
			log.Printf("✅ end_date распарсен как RFC3339: %s", endDate.Format(time.RFC3339))
		}
	} else {
		// По умолчанию - конец текущего дня
		now := time.Now()
		endDate = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, now.Location())
	}

	log.Printf("📅 Период поиска: start_date=%s, end_date=%s", startDate.Format(time.RFC3339), endDate.Format(time.RFC3339))

	// Получаем tenant DB из контекста
	tenantDB, exists := c.Get("tenant_db")
	if !exists {
		log.Printf("❌ Tenant DB не найдена в контексте")
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Tenant DB не найдена",
		})
		return
	}

	db := tenantDB.(*gorm.DB)

	// Получаем информацию о договоре для определения partner_company_id
	var contract models.Contract
	if err := db.Where("id = ? AND admin_account_id = ?", contractID, adminAccountID).First(&contract).Error; err != nil {
		log.Printf("❌ Договор не найден: %v", err)
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Договор не найден",
		})
		return
	}

	// Получаем снимки из базы данных (tenant schema)
	// Ищем снимки по contract_id, а если не найдены - по partner_company_id
	// Это нужно для случаев, когда снимок был создан с contract_id=0 (для "Объекты наших клиентов")
	var snapshots []models.PartnerDailySnapshot

	// Сначала ищем по contract_id
	query := db.Where("contract_id = ? AND admin_account_id = ? AND snapshot_date >= ? AND snapshot_date <= ?",
		contractID, adminAccountID, startDate, endDate)

	// Если у договора есть partner_company_id, также ищем снимки по partner_company_id
	// Это нужно для случаев, когда снимок был создан с contract_id=0
	if contract.PartnerCompanyID != nil && *contract.PartnerCompanyID > 0 {
		query = db.Where("admin_account_id = ? AND snapshot_date >= ? AND snapshot_date <= ? AND ((contract_id = ?) OR (partner_company_id = ? AND contract_id = 0))",
			adminAccountID, startDate, endDate, contractID, *contract.PartnerCompanyID)
	}

	if err := query.Order("snapshot_date ASC").Find(&snapshots).Error; err != nil {
		log.Printf("❌ Ошибка получения снимков: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения снимков",
		})
		return
	}

	log.Printf("✅ Найдено снимков: %d (для договора ID=%s, partner_company_id=%v)", len(snapshots), contractID, contract.PartnerCompanyID)

	// Если у договора есть partner_company_id, заполняем пропуски данными из axenta_account_snapshots
	if contract.PartnerCompanyID != nil && *contract.PartnerCompanyID > 0 {
		// Создаем карту существующих снимков по датам
		// Также проверяем, не содержат ли они подозрительно мало объектов (возможно, это старые неправильные данные)
		existingSnapshotsByDate := make(map[string]bool)
		suspiciousSnapshotsByDate := make(map[string]bool) // Снимки с подозрительно малым количеством объектов
		for _, snapshot := range snapshots {
			dateKey := snapshot.SnapshotDate.Format("2006-01-02")
			existingSnapshotsByDate[dateKey] = true
			// Если снимок содержит 1 объект или меньше, считаем его подозрительным
			// (обычно у партнеров должно быть больше объектов)
			if snapshot.ActiveObjectsCount <= 1 {
				suspiciousSnapshotsByDate[dateKey] = true
				log.Printf("⚠️ Обнаружен подозрительный снимок для даты %s: только %d объектов, проверим axenta_account_snapshots", dateKey, snapshot.ActiveObjectsCount)
			}
		}

		// Генерируем все даты в периоде
		// Нормализуем даты до начала дня для точного сравнения
		currentDate := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
		// endDay должен быть именно днем окончания (не следующий день)
		// Если endDate имеет время 23:59:59, то это все еще тот же день
		endDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, time.UTC)

		// Получаем тарифный план договора
		var tariffPlan models.BillingPlan
		publicDB := database.DB.Session(&gorm.Session{})
		if err := publicDB.Exec("SET search_path TO public").Error; err == nil {
			if contract.TariffPlanID != nil && *contract.TariffPlanID > 0 {
				if err := publicDB.Where("id = ? AND admin_account_id = ?", *contract.TariffPlanID, adminAccountID).First(&tariffPlan).Error; err != nil {
					log.Printf("⚠️ Не удалось загрузить тарифный план ID=%d: %v", *contract.TariffPlanID, err)
				}
			}
		}

		// Для каждой даты в периоде проверяем, есть ли снимок
		for !currentDate.After(endDay) {
			dateKey := currentDate.Format("2006-01-02")

			// Если снимка нет для этой даты ИЛИ снимок подозрительный (мало объектов),
			// пытаемся получить данные из axenta_object_snapshots (исторические данные на конкретную дату)
			if !existingSnapshotsByDate[dateKey] || suspiciousSnapshotsByDate[dateKey] {
				log.Printf("🔍 Снимок для даты %s не найден или подозрительный, считаем объекты из axenta_object_snapshots для partner_company_id=%d", dateKey, *contract.PartnerCompanyID)

				// Подсчитываем объекты из axenta_object_snapshots на конкретную дату
				// Используем last_synced_at для определения, какие объекты были актуальны на эту дату
				snapshotEndOfDay := time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 23, 59, 59, 999999999, time.UTC)

				// Находим все дочерние аккаунты партнера из axenta_account_snapshots
				// API Axenta при запросе с accountId=partner_company_id возвращает объекты партнера и всех дочерних аккаунтов
				// Эти объекты сохраняются с account_external_id дочерних аккаунтов
				// ВАЖНО: Аккаунты могут быть в любой tenant схеме, поэтому ищем во всех схемах
				var childAccountIDs []int64
				childAccountIDs = append(childAccountIDs, int64(*contract.PartnerCompanyID)) // Добавляем сам партнер

				// Получаем все компании для поиска аккаунтов во всех tenant схемах
				var allCompanies []models.Company
				if err := database.DB.Find(&allCompanies).Error; err != nil {
					log.Printf("⚠️ Ошибка получения списка компаний для поиска аккаунтов: %v", err)
					allCompanies = []models.Company{} // Продолжаем с пустым списком
				}

				// Ищем дочерние аккаунты партнера во всех схемах
				// Иерархия может содержать ID партнера в разных форматах: "/123/", "123", "/123"
				// Ищем аккаунты, у которых в hierarchy есть ID партнера
				partnerIDStr := fmt.Sprintf("%d", *contract.PartnerCompanyID)
				seenAccountIDs := make(map[int64]bool)
				seenAccountIDs[int64(*contract.PartnerCompanyID)] = true // Помечаем сам партнер

				for _, company := range allCompanies {
					tenantDBForSearch := database.GetTenantDBByID(company.ID)
					if tenantDBForSearch == nil {
						continue
					}

					// Ф3-F-followup: admin_account_id-фильтр СОХРАНЁН (Codex Q2 confirm
					// поймал: эта функция итерирует ПО ВСЕМ companies через
					// database.GetTenantDBByID(company.ID), не одна tenant-схема
					// → без admin-фильтра hierarchy LIKE идёт ПО ВСЕМ схемам =
					// cross-tenant leakage). Pre-existing keying-mismatch
					// (snapshot keyed firstCompany.ID/долг#2 ≠ trusted company.ID
					// → 0 строк для non-first tenant) НЕ регрессия (axenta-mode
					// тоже не совпадал: Axenta accountId ≠ firstCompany.ID).
					// Корректный фикс — отдельная задача: использовать lookup
					// firstCompany.ID для admin_account_id ИЛИ перейти на
					// company_id-field в snapshot (Ф3-F шаг последующий).
					var childAccounts []models.AxentaAccountSnapshot
					if err := tenantDBForSearch.Model(&models.AxentaAccountSnapshot{}).
						Where("admin_account_id = ?", adminAccountID).
						Where("(hierarchy LIKE ? OR hierarchy LIKE ? OR hierarchy LIKE ? OR external_account_id = ?)",
							fmt.Sprintf("%%/%%%s/%%", partnerIDStr), // формат: /.../123/...
							fmt.Sprintf("%%/%%%s%%", partnerIDStr),  // формат: /.../123...
							fmt.Sprintf("%%/%s/%%", partnerIDStr),   // формат: /123/...
							int64(*contract.PartnerCompanyID)).      // сам партнер
						Find(&childAccounts).Error; err == nil {
						for _, acc := range childAccounts {
							// Добавляем только уникальные аккаунты
							if !seenAccountIDs[acc.ExternalAccountID] {
								childAccountIDs = append(childAccountIDs, acc.ExternalAccountID)
								seenAccountIDs[acc.ExternalAccountID] = true
							}
						}
						if len(childAccounts) > 0 {
							log.Printf("🔍 Найдено аккаунтов в схеме %s для партнера %d: %d",
								company.DatabaseSchema, *contract.PartnerCompanyID, len(childAccounts))
						}
					}
				}

				log.Printf("🔍 Всего найдено аккаунтов для партнера %d (во всех схемах): %d (включая сам партнер и дочерние)",
					*contract.PartnerCompanyID, len(childAccountIDs))

				// Подсчитываем объекты партнера и его дочерних аккаунтов на эту дату
				// ВАЖНО: Объекты могут быть в любой tenant схеме, поэтому ищем во всех схемах
				// Используем подзапрос для получения последнего снимка каждого объекта на эту дату
				// Это гарантирует, что мы берем актуальные данные на дату снимка
				var totalObjectsCount int64
				var activeObjectsCount int64

				log.Printf("🔍 Ищем объекты для partner_company_id=%d во всех tenant схемах (%d компаний)", *contract.PartnerCompanyID, len(allCompanies))

				// Сначала проверяем, есть ли вообще объекты для этих аккаунтов во всех схемах
				var testCount int64
				for _, company := range allCompanies {
					tenantDBForSearch := database.GetTenantDBByID(company.ID)
					if tenantDBForSearch == nil {
						continue
					}
					var count int64
					tenantDBForSearch.Model(&models.AxentaObjectSnapshot{}).
						Where("account_external_id IN ?", childAccountIDs).
						Count(&count)
					testCount += count
				}
				log.Printf("🔍 Всего объектов в БД для аккаунтов %v (во всех схемах): %d", childAccountIDs, testCount)

				// Собираем все уникальные external_object_id из всех схем
				allObjectIDs := make(map[int64]bool)
				activeObjectIDs := make(map[int64]bool)

				// Ищем объекты во всех tenant схемах
				for _, company := range allCompanies {
					tenantDBForSearch := database.GetTenantDBByID(company.ID)
					if tenantDBForSearch == nil {
						continue
					}

					// Подсчитываем всего объектов партнера и его дочерних аккаунтов на эту дату
					// ВАЖНО: Ищем объекты, которые СУЩЕСТВОВАЛИ на дату снимка (по axenta_created_at и axenta_deleted_at),
					// независимо от того, когда они были синхронизированы (last_synced_at).
					var objects []struct {
						ExternalObjectID int64 `gorm:"column:external_object_id"`
						IsActive         bool  `gorm:"column:is_active"`
					}

					err := tenantDBForSearch.Raw(`
						SELECT DISTINCT aos1.external_object_id, aos1.is_active
						FROM axenta_object_snapshots aos1
						WHERE aos1.account_external_id IN ?
							AND (aos1.axenta_created_at IS NULL OR aos1.axenta_created_at <= ?)
							AND (aos1.axenta_deleted_at IS NULL OR aos1.axenta_deleted_at > ?)
							AND aos1.last_synced_at = (
								SELECT MAX(aos2.last_synced_at)
								FROM axenta_object_snapshots aos2
								WHERE aos2.external_object_id = aos1.external_object_id
							)
					`, childAccountIDs, snapshotEndOfDay, snapshotEndOfDay).
						Scan(&objects).Error

					if err != nil {
						log.Printf("⚠️ Ошибка поиска объектов в схеме %s для даты %s (partner_company_id=%d): %v",
							company.DatabaseSchema, dateKey, *contract.PartnerCompanyID, err)
						continue
					}

					// Добавляем найденные объекты в общий список
					for _, obj := range objects {
						allObjectIDs[obj.ExternalObjectID] = true
						if obj.IsActive {
							activeObjectIDs[obj.ExternalObjectID] = true
						}
					}

					if len(objects) > 0 {
						log.Printf("✅ Найдено объектов в схеме %s для даты %s: %d", company.DatabaseSchema, dateKey, len(objects))
					}
				}

				totalObjectsCount = int64(len(allObjectIDs))
				activeObjectsCount = int64(len(activeObjectIDs))

				log.Printf("✅ Всего найдено уникальных объектов для даты %s: %d (активных: %d)", dateKey, totalObjectsCount, activeObjectsCount)

				// Если не нашли объекты в axenta_object_snapshots, проверяем, есть ли они вообще в БД
				if totalObjectsCount == 0 && activeObjectsCount == 0 {
					// Проверяем, есть ли объекты для этих аккаунтов вообще (без фильтра по дате) во всех схемах
					var anyObjectsCount int64
					for _, company := range allCompanies {
						tenantDBForSearch := database.GetTenantDBByID(company.ID)
						if tenantDBForSearch == nil {
							continue
						}
						var count int64
						tenantDBForSearch.Model(&models.AxentaObjectSnapshot{}).
							Where("account_external_id IN ?", childAccountIDs).
							Count(&count)
						anyObjectsCount += count
					}

					if anyObjectsCount > 0 {
						log.Printf("⚠️ Объекты в axenta_object_snapshots для даты %s не найдены, но есть %d объектов для этих аккаунтов в БД (возможно, они были синхронизированы в другие дни)",
							dateKey, anyObjectsCount)
					} else {
						log.Printf("⚠️ Объекты в axenta_object_snapshots для даты %s не найдены (partner_company_id=%d). Объектов для этих аккаунтов в БД нет.",
							dateKey, *contract.PartnerCompanyID)
					}

					log.Printf("⚠️ Объекты в axenta_object_snapshots для даты %s не найдены (partner_company_id=%d). Создаем запись об отсутствии данных.",
						dateKey, *contract.PartnerCompanyID)

					// Создаем запись об отсутствии данных, чтобы пользователь мог видеть, что нужно запросить снимок
					missingDataSnapshot := models.PartnerDailySnapshot{
						AdminAccountID:     adminAccountID,
						CompanyID:          adminAccountID,
						ContractID:         contract.ID,
						SnapshotDate:       currentDate,
						PartnerCompanyID:   *contract.PartnerCompanyID,
						TariffPlanID:       0, // Нет тарифа, т.к. данных нет
						MonthlyPrice:       decimal.Zero,
						DailyPrice:         decimal.Zero,
						TotalObjectsCount:  0,
						ActiveObjectsCount: 0,
						DiscountType:       "none",
						DiscountPercent:    decimal.Zero,
						DiscountFixed:      decimal.Zero,
						CostBeforeDiscount: decimal.Zero,
						DiscountAmount:     decimal.Zero,
						DailyCost:          decimal.Zero,
						Status:             "missing_data", // Специальный статус для отсутствующих данных
						Notes:              fmt.Sprintf("Данные за %s отсутствуют. Необходимо запросить снимок через Axenta Cloud API.", dateKey),
					}

					// Если это подозрительный снимок, удаляем его из массива перед добавлением записи об отсутствии данных
					if suspiciousSnapshotsByDate[dateKey] {
						for i := len(snapshots) - 1; i >= 0; i-- {
							if snapshots[i].SnapshotDate.Format("2006-01-02") == dateKey {
								log.Printf("🗑️ Удаляем подозрительный снимок для даты %s (было %d объектов)", dateKey, snapshots[i].ActiveObjectsCount)
								snapshots = append(snapshots[:i], snapshots[i+1:]...)
								break
							}
						}
					}

					snapshots = append(snapshots, missingDataSnapshot)
					log.Printf("📋 Создана запись об отсутствии данных для даты %s", dateKey)

					currentDate = currentDate.AddDate(0, 0, 1)
					continue
				}

				// Если нашли объекты, создаем виртуальный снимок на основе данных из axenta_object_snapshots
				log.Printf("✅ Найдены объекты в axenta_object_snapshots для даты %s: активных=%d, всего=%d",
					dateKey, activeObjectsCount, totalObjectsCount)

				if !tariffPlan.Price.IsZero() {
					// Рассчитываем дневную цену
					dailyPrice := tariffPlan.Price.Div(decimal.NewFromInt(30))

					// Рассчитываем скидку
					discountPercent := contract.GetDiscountPercent(int(activeObjectsCount))
					discountFixed := contract.GetDiscountFixed()

					// Стоимость до скидки
					costBeforeDiscount := dailyPrice.Mul(decimal.NewFromInt(activeObjectsCount))

					// Сумма скидки
					var discountAmount decimal.Decimal
					if discountFixed.GreaterThan(decimal.Zero) {
						// Фиксированная скидка применяется к месячному тарифу
						effectiveMonthlyPrice := tariffPlan.Price.Sub(discountFixed)
						if effectiveMonthlyPrice.IsNegative() {
							effectiveMonthlyPrice = decimal.Zero
						}
						effectiveDailyPrice := effectiveMonthlyPrice.Div(decimal.NewFromInt(30))
						dailyPrice = effectiveDailyPrice
						discountAmount = discountFixed.Div(decimal.NewFromInt(30)).Mul(decimal.NewFromInt(activeObjectsCount))
					} else if discountPercent.GreaterThan(decimal.Zero) {
						discountMultiplier := discountPercent.Div(decimal.NewFromInt(100))
						discountAmount = costBeforeDiscount.Mul(discountMultiplier)
					}

					// Итоговая стоимость
					dailyCost := costBeforeDiscount.Sub(discountAmount)

					// Определяем тип скидки
					discountType := "none"
					if discountFixed.GreaterThan(decimal.Zero) {
						discountType = "manual"
					} else if discountPercent.GreaterThan(decimal.Zero) {
						discountType = "auto"
					}

					// Создаем виртуальный снимок
					virtualSnapshot := models.PartnerDailySnapshot{
						AdminAccountID:     adminAccountID,
						CompanyID:          adminAccountID,
						ContractID:         contract.ID,
						SnapshotDate:       currentDate,
						PartnerCompanyID:   *contract.PartnerCompanyID,
						TariffPlanID:       tariffPlan.ID,
						MonthlyPrice:       tariffPlan.Price,
						DailyPrice:         dailyPrice.Round(6),
						TotalObjectsCount:  int(totalObjectsCount),
						ActiveObjectsCount: int(activeObjectsCount),
						DiscountType:       discountType,
						DiscountPercent:    discountPercent,
						DiscountFixed:      discountFixed,
						CostBeforeDiscount: costBeforeDiscount.Round(4),
						DiscountAmount:     discountAmount.Round(4),
						DailyCost:          dailyCost.Round(4),
						Status:             "completed",
						Notes:              fmt.Sprintf("Создан из axenta_object_snapshots на дату %s", dateKey),
					}

					// Если это подозрительный снимок, удаляем его из массива перед добавлением виртуального
					if suspiciousSnapshotsByDate[dateKey] {
						// Удаляем подозрительный снимок из массива
						for i := len(snapshots) - 1; i >= 0; i-- {
							if snapshots[i].SnapshotDate.Format("2006-01-02") == dateKey {
								log.Printf("🗑️ Удаляем подозрительный снимок для даты %s (было %d объектов)", dateKey, snapshots[i].ActiveObjectsCount)
								snapshots = append(snapshots[:i], snapshots[i+1:]...)
								break
							}
						}
					}

					snapshots = append(snapshots, virtualSnapshot)
					log.Printf("✅ Создан виртуальный снимок для даты %s из axenta_object_snapshots: активных=%d, всего=%d, стоимость=%.2f₽",
						dateKey, activeObjectsCount, totalObjectsCount, dailyCost.InexactFloat64())
				} else {
					log.Printf("⚠️ Не удалось создать виртуальный снимок: тарифный план не найден или цена = 0")
				}
			}

			// Переходим к следующему дню
			currentDate = currentDate.AddDate(0, 0, 1)
		}

		// Сортируем снимки по дате
		for i := 0; i < len(snapshots)-1; i++ {
			for j := i + 1; j < len(snapshots); j++ {
				if snapshots[i].SnapshotDate.After(snapshots[j].SnapshotDate) {
					snapshots[i], snapshots[j] = snapshots[j], snapshots[i]
				}
			}
		}

		log.Printf("✅ После заполнения пропусков: найдено снимков: %d", len(snapshots))
	}

	// Финальная фильтрация снимков по выбранному периоду
	// Нормализуем даты для точного сравнения
	startDay := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
	endDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, time.UTC)

	filteredSnapshots := make([]models.PartnerDailySnapshot, 0)
	for _, snapshot := range snapshots {
		snapshotDay := time.Date(snapshot.SnapshotDate.Year(), snapshot.SnapshotDate.Month(), snapshot.SnapshotDate.Day(), 0, 0, 0, 0, time.UTC)
		// Включаем снимок только если его дата находится в выбранном диапазоне (включительно)
		// snapshotDay >= startDay && snapshotDay <= endDay
		if !snapshotDay.Before(startDay) && !snapshotDay.After(endDay) {
			filteredSnapshots = append(filteredSnapshots, snapshot)
		}
	}

	log.Printf("✅ После фильтрации по периоду: найдено снимков: %d (было %d)", len(filteredSnapshots), len(snapshots))
	snapshots = filteredSnapshots

	// Рассчитываем сводную информацию
	summary := calculateSnapshotsSummary(snapshots, startDate, endDate)

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"snapshots": snapshots,
		"summary":   summary,
	})
}

// calculateSnapshotsSummary рассчитывает сводную информацию по снимкам
func calculateSnapshotsSummary(snapshots []models.PartnerDailySnapshot, startDate, endDate time.Time) map[string]interface{} {
	// Нормализуем даты до начала дня для точного расчета
	startDay := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	endDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, endDate.Location())

	// Рассчитываем количество дней в периоде (включительно)
	// Например, с 1 по 3 декабря = 3 дня (1, 2, 3)
	daysDiff := int(endDay.Sub(startDay).Hours() / 24)
	totalDays := daysDiff + 1

	log.Printf("📅 Период: %s - %s, количество дней в периоде: %d (найдено снимков: %d)",
		startDay.Format("2006-01-02"), endDay.Format("2006-01-02"), totalDays, len(snapshots))
	totalCost := decimal.Zero
	totalObjects := 0
	baseDailyPrice := decimal.Zero   // Базовая дневная цена БЕЗ скидки
	baseMonthlyPrice := decimal.Zero // Базовая месячная цена БЕЗ скидки

	// Для расчета эффективных цен с учетом скидок
	totalCostBeforeDiscount := decimal.Zero
	totalDiscountAmount := decimal.Zero
	discountType := "none" // Тип скидки (берем из первого снимка)

	for _, snapshot := range snapshots {
		totalCost = totalCost.Add(snapshot.DailyCost)
		totalObjects += snapshot.ActiveObjectsCount
		totalCostBeforeDiscount = totalCostBeforeDiscount.Add(snapshot.CostBeforeDiscount)
		totalDiscountAmount = totalDiscountAmount.Add(snapshot.DiscountAmount)

		if baseDailyPrice.IsZero() {
			baseDailyPrice = snapshot.DailyPrice
			baseMonthlyPrice = snapshot.MonthlyPrice
			discountType = snapshot.DiscountType
		}
	}

	// Рассчитываем среднее количество объектов (с точностью decimal для правильных расчетов)
	avgObjectsDecimal := decimal.Zero
	avgObjectsInt := 0
	if totalDays > 0 {
		avgObjectsDecimal = decimal.NewFromInt(int64(totalObjects)).Div(decimal.NewFromInt(int64(totalDays)))
		avgObjectsInt = totalObjects / totalDays
	}

	// Рассчитываем эффективную (реальную) дневную цену с учетом скидки
	// Это средняя цена за объект в день с учетом всех скидок
	effectiveDailyPrice := decimal.Zero
	if totalObjects > 0 {
		effectiveDailyPrice = totalCost.Div(decimal.NewFromInt(int64(totalObjects)))
	}

	// Рассчитываем эффективную месячную цену с учетом скидки
	// Это цена, которую партнер реально платит за месяц (30 дней)
	// Если скидок нет, используем базовую цену для точности
	var effectiveMonthlyPrice decimal.Decimal
	if discountType == "none" && totalDiscountAmount.IsZero() {
		// Нет скидок - используем базовую месячную цену
		effectiveMonthlyPrice = baseMonthlyPrice
		// Пересчитываем effectiveDailyPrice из базовой цены для точности
		effectiveDailyPrice = baseMonthlyPrice.Div(decimal.NewFromInt(30))
	} else {
		// Есть скидки - рассчитываем эффективную цену
		effectiveMonthlyPrice = effectiveDailyPrice.Mul(decimal.NewFromInt(30))
	}

	// Расчет цены за объект за период (pricePerObjectForPeriod) С УЧЕТОМ СКИДКИ
	// Формула: total_cost / avg_objects (используем точное decimal значение)
	// Это цена, при умножении на которую среднее количество объектов даст общую стоимость
	// avg_objects × price_per_object_for_period = total_cost
	pricePerObjectForPeriod := decimal.Zero
	if avgObjectsDecimal.GreaterThan(decimal.Zero) {
		pricePerObjectForPeriod = totalCost.Div(avgObjectsDecimal)
	}

	log.Printf("💰 Расчет цены с учетом скидок:")
	log.Printf("   Всего дней: %d", totalDays)
	log.Printf("   Всего объектов: %d", totalObjects)
	log.Printf("   Средних объектов (точное): %.2f", avgObjectsDecimal.InexactFloat64())
	log.Printf("   Средних объектов (округл.): %d", avgObjectsInt)
	log.Printf("   Общая стоимость: %.2f ₽", totalCost.InexactFloat64())
	log.Printf("   Базовая месячная цена: %.2f ₽", baseMonthlyPrice.InexactFloat64())
	log.Printf("   Эффективная месячная цена (с учетом скидок): %.4f ₽", effectiveMonthlyPrice.InexactFloat64())
	log.Printf("   Базовая дневная цена: %.4f ₽", baseDailyPrice.InexactFloat64())
	log.Printf("   Эффективная дневная цена (с учетом скидок): %.4f ₽", effectiveDailyPrice.InexactFloat64())
	log.Printf("   Цена за объект/период: %.4f ₽", pricePerObjectForPeriod.InexactFloat64())
	log.Printf("   Общая скидка за период: %.2f ₽", totalDiscountAmount.InexactFloat64())
	log.Printf("   ✅ Проверка: %.2f × %.4f = %.2f ₽ (должно быть %.2f ₽)",
		avgObjectsDecimal.InexactFloat64(),
		pricePerObjectForPeriod.InexactFloat64(),
		avgObjectsDecimal.Mul(pricePerObjectForPeriod).InexactFloat64(),
		totalCost.InexactFloat64())

	// Средняя дневная скидка
	avgDailyDiscount := decimal.Zero
	if totalDays > 0 {
		avgDailyDiscount = totalDiscountAmount.Div(decimal.NewFromInt(int64(totalDays)))
	}

	result := map[string]interface{}{
		"total_days":                  totalDays,
		"total_cost":                  totalCost.Round(2).InexactFloat64(),             // Округляем до 2 знаков
		"avg_objects":                 avgObjectsDecimal.Round(2).InexactFloat64(),     // Округляем до 2 знаков
		"daily_price":                 effectiveDailyPrice.Round(6).InexactFloat64(),   // Округляем до 6 знаков
		"monthly_price":               effectiveMonthlyPrice.Round(2).InexactFloat64(), // Округляем до 2 знаков (рубли)
		"total_objects":               totalObjects,
		"price_per_object_for_period": pricePerObjectForPeriod.Round(4).InexactFloat64(), // Округляем до 4 знаков
		"base_monthly_price":          baseMonthlyPrice.Round(2).InexactFloat64(),        // Округляем до 2 знаков
		"base_daily_price":            baseDailyPrice.Round(6).InexactFloat64(),          // Округляем до 6 знаков
		"total_discount":              totalDiscountAmount.Round(2).InexactFloat64(),     // Округляем до 2 знаков
		"discount_type":               discountType,                                      // Тип скидки
		"avg_daily_discount":          avgDailyDiscount.Round(2).InexactFloat64(),        // Округляем до 2 знаков
	}

	log.Printf("📦 Возвращаем summary: %+v", result)

	return result
}

// CreatePartnerSnapshots создает снимки для всех партнерских договоров (ручной запуск)
func CreatePartnerSnapshots(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	log.Printf("📸 Ручное создание снимков партнерских договоров (admin_account_id=%d)", adminAccountID)

	// Получаем токен пользователя из заголовка
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Отсутствует токен авторизации",
		})
		return
	}

	// Извлекаем токен (формат: "Token XXXXX")
	var userToken string
	if len(authHeader) > 6 && authHeader[:6] == "Token " {
		userToken = authHeader[6:]
	} else {
		userToken = authHeader
	}

	// Получаем tenant DB из контекста (договоры находятся в tenant схеме)
	tenantDB, exists := c.Get("tenant_db")
	if !exists {
		log.Printf("❌ Tenant DB не найдена в контексте")
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Tenant DB не найдена",
		})
		return
	}

	db := tenantDB.(*gorm.DB)

	// Получаем все партнерские договоры из tenant схемы
	var contracts []models.Contract
	if err := db.
		Where("contract_type = ? AND partner_company_id IS NOT NULL AND tariff_plan_id IS NOT NULL AND admin_account_id = ?",
			"partner", adminAccountID).
		Find(&contracts).Error; err != nil {
		log.Printf("❌ Ошибка получения договоров: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения партнерских договоров",
		})
		return
	}

	log.Printf("📋 Найдено партнерских договоров: %d", len(contracts))

	if len(contracts) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "Нет партнерских договоров для создания снимков",
		})
		return
	}

	// Создаем сервис снимков
	snapshotService := services.NewPartnerSnapshotService()

	// Дата снимка - сегодня
	now := time.Now().UTC()
	snapshotDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Создаем снимки для каждого договора, используя токен пользователя
	successCount := 0
	errorCount := 0

	for _, contract := range contracts {
		if err := snapshotService.CreateSnapshotForContractWithTokenAndDB(&contract, snapshotDate, userToken, db); err != nil {
			log.Printf("❌ Ошибка создания снимка для договора %d: %v", contract.ID, err)
			errorCount++
		} else {
			successCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":          "success",
		"message":         "Снимки созданы",
		"success_count":   successCount,
		"error_count":     errorCount,
		"total_contracts": len(contracts),
	})
}

// GeneratePartnerSnapshotsForPeriod создает снимки для конкретного договора за указанный период
func GeneratePartnerSnapshotsForPeriod(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	contractID := c.Param("contract_id")
	log.Printf("📸 Создание снимков для договора ID=%s за период", contractID)

	// Получаем токен пользователя из заголовка
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Отсутствует токен авторизации",
		})
		return
	}

	// Извлекаем токен (формат: "Token XXXXX")
	var userToken string
	if len(authHeader) > 6 && authHeader[:6] == "Token " {
		userToken = authHeader[6:]
	} else {
		userToken = authHeader
	}

	// Парсим тело запроса
	var requestBody struct {
		StartDate  string `json:"start_date"`  // Формат: YYYY-MM-DD
		EndDate    string `json:"end_date"`    // Формат: YYYY-MM-DD
		DataSource string `json:"data_source"` // Опционально: "db" (из БД) или "api" (из Axenta API), по умолчанию "api"
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат запроса",
		})
		return
	}

	// Парсим даты
	startDate, err := time.Parse("2006-01-02", requestBody.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат start_date (ожидается YYYY-MM-DD)",
		})
		return
	}

	endDate, err := time.Parse("2006-01-02", requestBody.EndDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат end_date (ожидается YYYY-MM-DD)",
		})
		return
	}

	// Проверяем, что startDate <= endDate
	if startDate.After(endDate) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Дата начала не может быть позже даты окончания",
		})
		return
	}

	// Получаем tenant DB из контекста
	tenantDB, exists := c.Get("tenant_db")
	if !exists {
		log.Printf("❌ Tenant DB не найдена в контексте")
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Tenant DB не найдена",
		})
		return
	}

	db := tenantDB.(*gorm.DB)

	// Получаем договор
	var contract models.Contract
	if err := db.
		Where("id = ? AND admin_account_id = ?", contractID, adminAccountID).
		First(&contract).Error; err != nil {
		log.Printf("❌ Договор не найден: %v", err)
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Договор не найден",
		})
		return
	}

	// Проверяем, что это партнерский договор
	if contract.ContractType != "partner" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Это не партнерский договор",
		})
		return
	}

	// Создаем сервис снимков
	snapshotService := services.NewPartnerSnapshotService()

	// Определяем источник данных (по умолчанию "api" для получения актуальных данных)
	dataSource := requestBody.DataSource
	if dataSource == "" {
		dataSource = "api" // По умолчанию используем API
	}
	if dataSource != "db" && dataSource != "api" {
		dataSource = "api" // Если неверное значение, используем API
	}

	// Создаем снимки для каждого дня в периоде
	successCount := 0
	errorCount := 0
	currentDate := startDate

	for !currentDate.After(endDate) {
		// Устанавливаем время на начало дня в UTC
		snapshotDate := time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 0, 0, 0, 0, time.UTC)

		// Создаем снимок для этой даты с указанным источником данных
		if err := snapshotService.CreateSnapshotForContractWithTokenAndDBAndSource(&contract, snapshotDate, userToken, db, dataSource); err != nil {
			log.Printf("❌ Ошибка создания снимка для договора %d на дату %s: %v",
				contract.ID, snapshotDate.Format("2006-01-02"), err)
			errorCount++
		} else {
			successCount++
		}

		// Переходим к следующему дню
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	log.Printf("✅ Снимки за период созданы: успешно %d, ошибок %d", successCount, errorCount)

	// Нормализуем даты до начала дня для точного расчета количества дней
	startDay := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	endDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, endDate.Location())
	periodDays := int(endDay.Sub(startDay).Hours()/24) + 1

	c.JSON(http.StatusOK, gin.H{
		"status":        "success",
		"message":       "Снимки за период созданы",
		"success_count": successCount,
		"error_count":   errorCount,
		"period_days":   periodDays,
	})
}

// GenerateAllPartnerSnapshotsForPeriod создает снимки для ВСЕХ партнерских договоров за период
// POST /api/auth/contracts/partner-snapshots/generate-all
func GenerateAllPartnerSnapshotsForPeriod(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Токен пользователя
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Отсутствует токен авторизации",
		})
		return
	}
	var userToken string
	if len(authHeader) > 6 && authHeader[:6] == "Token " {
		userToken = authHeader[6:]
	} else {
		userToken = authHeader
	}

	// Тело запроса
	var requestBody struct {
		StartDate string `json:"start_date"` // YYYY-MM-DD
		EndDate   string `json:"end_date"`   // YYYY-MM-DD
	}
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат запроса",
		})
		return
	}

	startDate, err := time.Parse("2006-01-02", requestBody.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат start_date (ожидается YYYY-MM-DD)",
		})
		return
	}
	endDate, err := time.Parse("2006-01-02", requestBody.EndDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат end_date (ожидается YYYY-MM-DD)",
		})
		return
	}
	if startDate.After(endDate) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Дата начала не может быть позже даты окончания",
		})
		return
	}

	// Получаем tenant DB
	tenantDBVal, exists := c.Get("tenant_db")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Tenant DB не найдена",
		})
		return
	}
	tenantDB := tenantDBVal.(*gorm.DB)

	// Ищем все активные партнерские договоры с указанным партнером
	var contracts []models.Contract
	if err := tenantDB.
		Where("partner_company_id IS NOT NULL").
		Where("is_active = ?", true).
		Find(&contracts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка получения договоров: %v", err),
		})
		return
	}

	if len(contracts) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"status":              "success",
			"message":             "Партнерские договоры не найдены",
			"processed_contracts": 0,
		})
		return
	}

	snapshotService := services.NewPartnerSnapshotService()
	successCount := 0
	errorCount := 0

	for _, contract := range contracts {
		// Проверяем принадлежность администратору (мультитенантный guard)
		if contract.AdminAccountID != adminAccountID {
			continue
		}

		// Убеждаемся, что есть привязка партнера
		if contract.PartnerCompanyID == nil {
			continue
		}

		for date := startDate; !date.After(endDate); date = date.AddDate(0, 0, 1) {
			if err := snapshotService.CreateSnapshotForContractWithTokenAndDB(&contract, date, userToken, tenantDB); err != nil {
				log.Printf("❌ Ошибка создания снимка для договора %d на %s: %v", contract.ID, date.Format("2006-01-02"), err)
				errorCount++
			} else {
				successCount++
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":              "success",
		"message":             "Снимки за период созданы для всех партнерских договоров",
		"success_count":       successCount,
		"error_count":         errorCount,
		"processed_contracts": len(contracts),
	})
}

// LoadAllCurrentObjects выполняет разовую загрузку всех текущих объектов из Axenta
// POST /api/auth/snapshots/load-all-current
func LoadAllCurrentObjects(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	log.Printf("🔄 Запрос разовой загрузки всех текущих объектов (admin_account_id=%d)", adminAccountID)

	// Получаем токен из настроек снимков
	var firstCompany models.Company
	if err := database.DB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Не найдено активных компаний",
		})
		return
	}

	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Не удалось получить tenant DB для компании %d", firstCompany.ID),
		})
		return
	}

	var snapshotSettings models.SnapshotSettings
	if err := tenantDB.Where("company_id = ? AND is_active = ?", 1, true).First(&snapshotSettings).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Токен Axenta не настроен. Пожалуйста, настройте токен в настройках снимков.",
		})
		return
	}

	if snapshotSettings.AxentaToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Токен Axenta не установлен. Пожалуйста, установите токен в настройках снимков.",
		})
		return
	}

	// Инициализируем статус загрузки
	startTime := time.Now()
	updateLoadProgress(LoadProgressStatus{
		IsLoading: true,
		Progress:  0,
		Loaded:    0,
		Total:     0,
		Status:    "loading",
		StartTime: &startTime,
	})

	// Запускаем загрузку в фоне
	go func() {
		defer func() {
			// После завершения загрузки обновляем статус
			updateLoadProgress(LoadProgressStatus{
				IsLoading: false,
				Progress:  100,
				Status:    "completed",
			})
		}()

		accumulationService := services.NewSnapshotAccumulationService()
		// Сохраняем startTime для использования в callback
		startTimePtr := &startTime

		// Создаем callback для обновления прогресса
		progressCallback := func(loaded int, total int, currentPage int, totalPages int, status string) {
			var progress float64
			if total > 0 {
				progress = float64(loaded) / float64(total) * 100
			}
			updateLoadProgress(LoadProgressStatus{
				IsLoading:   true,
				Progress:    progress,
				Loaded:      loaded,
				Total:       total,
				CurrentPage: currentPage,
				TotalPages:  totalPages,
				Status:      status,
				StartTime:   startTimePtr,
			})
		}

		// Передаем callback для обновления прогресса
		if err := accumulationService.LoadAllCurrentObjectsWithProgress(snapshotSettings.AxentaToken, progressCallback); err != nil {
			log.Printf("❌ Ошибка загрузки всех текущих объектов: %v", err)
			updateLoadProgress(LoadProgressStatus{
				IsLoading:    false,
				Status:       "error",
				ErrorMessage: err.Error(),
			})
			return
		}

		// После успешной загрузки объектов создаем/обновляем запись в snapshot_jobs
		log.Printf("📊 Подсчитываем объекты и создаем запись в snapshot_jobs...")

		// Получаем первую компанию
		var firstCompany models.Company
		if err := database.DB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
			log.Printf("⚠️ Не найдено активных компаний для создания записи snapshot_job")
			return
		}

		tenantDB := database.GetTenantDBByID(firstCompany.ID)
		if tenantDB == nil {
			log.Printf("⚠️ Не удалось получить tenant DB для компании %d", firstCompany.ID)
			return
		}

		// Используем текущую дату для снимка
		snapshotDate := time.Now().UTC()
		snapshotDate = time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 0, 0, 0, 0, time.UTC)

		// Подсчитываем объекты на текущую дату
		totalObjects, activeObjects, err := accumulationService.CalculateObjectsCountForDate(snapshotDate, tenantDB)
		if err != nil {
			log.Printf("⚠️ Ошибка подсчета объектов: %v", err)
		} else {
			log.Printf("📊 Найдено объектов: всего=%d, активных=%d", totalObjects, activeObjects)

			// Создаем или обновляем запись в snapshot_jobs
			publicDB := database.DB.Session(&gorm.Session{})
			if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
				log.Printf("⚠️ Ошибка переключения на схему public: %v", err)
				return
			}

			dateStr := snapshotDate.Format("2006-01-02")

			// 1. Создаем/обновляем запись типа billing_start с фиксированной датой 2024-03-14 13:12:04
			billingStartDate := time.Date(2024, 3, 14, 13, 12, 4, 0, time.UTC)
			billingStartDateStr := billingStartDate.Format("2006-01-02")

			// Ищем самый ранний объект для billing_start
			var earliestObject *models.AxentaObjectSnapshot
			var earliestObj models.AxentaObjectSnapshot
			dayEnd := time.Date(billingStartDate.Year(), billingStartDate.Month(), billingStartDate.Day(), 23, 59, 59, 999999999, time.UTC)
			query := tenantDB.
				Where("axenta_created_at IS NOT NULL").
				Where("(axenta_created_at IS NULL OR axenta_created_at <= ?) AND (axenta_deleted_at IS NULL OR axenta_deleted_at > ?)",
					dayEnd, dayEnd).
				Order("axenta_created_at ASC").
				Limit(1)

			if err := query.First(&earliestObj).Error; err == nil {
				earliestObject = &earliestObj
			}

			// Подсчитываем объекты на дату billing_start
			billingStartTotalObjects, billingStartActiveObjects, _ := accumulationService.CalculateObjectsCountForDate(billingStartDate, tenantDB)

			// Создаем/обновляем запись billing_start
			var billingStartJob models.SnapshotJob
			if err := publicDB.
				Where("job_type = ? AND date_from::date = ?::date", models.SnapshotJobTypeBillingStart, billingStartDateStr).
				First(&billingStartJob).Error; err == nil {
				// Обновляем существующую запись
				log.Printf("📝 Обновляем запись billing_start (ID: %d)...", billingStartJob.ID)
				billingStartJob.TotalObjects = billingStartTotalObjects
				billingStartJob.ActiveObjects = billingStartActiveObjects
				billingStartJob.FinishJob(models.SnapshotJobStatusCompleted, "")
				if err := publicDB.Save(&billingStartJob).Error; err != nil {
					log.Printf("❌ Ошибка обновления записи billing_start: %v", err)
				} else {
					log.Printf("✅ Запись billing_start обновлена (ID: %d)", billingStartJob.ID)
				}
			} else {
				// Создаем новую запись billing_start
				log.Printf("➕ Создаем новую запись billing_start...")
				if err := accumulationService.SaveBillingStartDateToHistory(tenantDB, billingStartDate, billingStartTotalObjects, billingStartActiveObjects, earliestObject); err != nil {
					log.Printf("❌ Ошибка создания записи billing_start: %v", err)
				} else {
					log.Printf("✅ Запись billing_start создана")
				}
			}

			// 2. Создаем/обновляем запись типа daily_auto для текущей даты
			var existingJob models.SnapshotJob
			if err := publicDB.
				Where("job_type = ? AND date_from::date = ?::date", models.SnapshotJobTypeDailyAuto, dateStr).
				First(&existingJob).Error; err == nil {
				// Обновляем существующую запись
				log.Printf("📝 Обновляем запись daily_auto (ID: %d)...", existingJob.ID)
				existingJob.TotalObjects = totalObjects
				existingJob.ActiveObjects = activeObjects
				existingJob.FinishJob(models.SnapshotJobStatusCompleted, "")
				if err := publicDB.Save(&existingJob).Error; err != nil {
					log.Printf("❌ Ошибка обновления записи daily_auto: %v", err)
				} else {
					log.Printf("✅ Запись daily_auto обновлена (ID: %d)", existingJob.ID)
				}
			} else {
				// Создаем новую запись
				log.Printf("➕ Создаем новую запись daily_auto...")
				now := time.Now()
				job := models.SnapshotJob{
					JobType:            models.SnapshotJobTypeDailyAuto,
					StartedAt:          now,
					DateFrom:           snapshotDate,
					DateTo:             snapshotDate,
					Status:             models.SnapshotJobStatusCompleted,
					TotalObjects:       totalObjects,
					ActiveObjects:      activeObjects,
					SuccessCount:       1,
					TotalDaysProcessed: 1,
					TriggeredBy:        "manual",
				}
				job.FinishJob(models.SnapshotJobStatusCompleted, "")

				if err := publicDB.Create(&job).Error; err != nil {
					log.Printf("❌ Ошибка создания записи daily_auto: %v", err)
				} else {
					log.Printf("✅ Запись daily_auto создана (ID: %d)", job.ID)
				}
			}
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Загрузка всех текущих объектов запущена в фоновом режиме",
	})
}

// GetLoadProgress возвращает текущий статус загрузки объектов
// GET /api/auth/snapshots/load-progress
func GetLoadProgress(c *gin.Context) {
	progress := getLoadProgress()
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   progress,
	})
}

// GetBillingStartDate возвращает стартовую дату биллинга
// GET /api/auth/snapshots/billing-start-date
func GetBillingStartDate(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	log.Printf("📅 Запрос стартовой даты биллинга (admin_account_id=%d)", adminAccountID)

	// Получаем tenant DB
	var firstCompany models.Company
	if err := database.DB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Не найдено активных компаний",
		})
		return
	}

	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Не удалось получить tenant DB для компании %d", firstCompany.ID),
		})
		return
	}

	accumulationService := services.NewSnapshotAccumulationService()
	startDate, earliestObject, err := accumulationService.GetBillingStartDate(tenantDB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Проверяем, создана ли запись в истории
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err == nil {
		var historyJob models.SnapshotJob
		if err := publicDB.
			Where("job_type = ? AND date_from::date = ?::date", models.SnapshotJobTypeBillingStart, startDate.Format("2006-01-02")).
			First(&historyJob).Error; err == nil {
			log.Printf("✅ Запись в истории найдена: ID=%d, дата=%s", historyJob.ID, historyJob.DateFrom.Format("2006-01-02"))
		} else {
			log.Printf("⚠️ Запись в истории не найдена после создания (это нормально, если была ошибка)")
		}
	}

	response := gin.H{
		"status":         "success",
		"start_date":     startDate.Format("2006-01-02"),
		"start_date_iso": startDate.Format(time.RFC3339),
	}

	// Добавляем информацию о найденном объекте, если он есть
	if earliestObject != nil {
		response["earliest_object"] = gin.H{
			"id":              earliestObject.ExternalObjectID,
			"name":            earliestObject.ObjectName,
			"created_at":      earliestObject.AxentaCreatedAt,
			"created_at_date": earliestObject.AxentaCreatedAt.Format("2006-01-02"),
		}
	}

	// Добавляем информацию о записи в истории
	response["message"] = fmt.Sprintf("Стартовая дата биллинга определена: %s. Запись должна быть создана в истории снимков.", startDate.Format("2006-01-02"))
	response["check_history"] = "Используйте GET /api/auth/snapshots/check-billing-start-in-history для проверки"

	c.JSON(http.StatusOK, response)
}

// ProcessDailyAccumulation обрабатывает ежедневное накопление объектов
// POST /api/auth/snapshots/daily-accumulation
func ProcessDailyAccumulation(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	log.Printf("🔄 Запрос ежедневного накопления объектов (admin_account_id=%d)", adminAccountID)

	// Получаем токен из настроек снимков
	var firstCompany models.Company
	if err := database.DB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Не найдено активных компаний",
		})
		return
	}

	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Не удалось получить tenant DB для компании %d", firstCompany.ID),
		})
		return
	}

	var snapshotSettings models.SnapshotSettings
	if err := tenantDB.Where("company_id = ? AND is_active = ?", 1, true).First(&snapshotSettings).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Токен Axenta не настроен",
		})
		return
	}

	if snapshotSettings.AxentaToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Токен Axenta не установлен",
		})
		return
	}

	// Запускаем обработку в фоне
	go func() {
		accumulationService := services.NewSnapshotAccumulationService()
		if err := accumulationService.ProcessDailyAccumulation(snapshotSettings.AxentaToken, tenantDB); err != nil {
			log.Printf("❌ Ошибка ежедневного накопления: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Ежедневное накопление запущено в фоновом режиме",
	})
}

// GetObjectsCountForDate возвращает количество объектов на указанную дату
// GET /api/auth/snapshots/objects-count?date=2025-01-01
func GetObjectsCountForDate(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	dateStr := c.Query("date")
	if dateStr == "" {
		dateStr = time.Now().UTC().Format("2006-01-02")
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Неверный формат даты: %s (ожидается YYYY-MM-DD)", dateStr),
		})
		return
	}

	log.Printf("📊 Запрос количества объектов на дату %s (admin_account_id=%d)", dateStr, adminAccountID)

	// Получаем tenant DB
	var firstCompany models.Company
	if err := database.DB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Не найдено активных компаний",
		})
		return
	}

	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Не удалось получить tenant DB для компании %d", firstCompany.ID),
		})
		return
	}

	accumulationService := services.NewSnapshotAccumulationService()
	total, active, err := accumulationService.CalculateObjectsCountForDate(date, tenantDB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":           "success",
		"date":             dateStr,
		"total_objects":    total,
		"active_objects":   active,
		"inactive_objects": total - active,
	})
}

// CheckBillingStartDateInHistory проверяет, есть ли запись о стартовой дате биллинга в истории
// GET /api/auth/snapshots/check-billing-start-in-history
func CheckBillingStartDateInHistory(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	log.Printf("🔍 Проверка записи о стартовой дате биллинга в истории (admin_account_id=%d)", adminAccountID)

	// Таблица snapshot_jobs находится в схеме public
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка переключения на схему public",
		})
		return
	}

	// Проверяем, что таблица существует
	if !publicDB.Migrator().HasTable(&models.SnapshotJob{}) {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Таблица snapshot_jobs не существует в схеме public",
		})
		return
	}

	// Ищем записи типа billing_start
	var jobs []models.SnapshotJob
	query := publicDB.
		Where("job_type = ?", models.SnapshotJobTypeBillingStart).
		Order("started_at DESC")

	log.Printf("🔍 Поиск записей billing_start в таблице snapshot_jobs...")
	if err := query.Find(&jobs).Error; err != nil {
		log.Printf("❌ Ошибка поиска записей: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка поиска записей: %v", err),
		})
		return
	}

	log.Printf("📊 Найдено записей billing_start: %d", len(jobs))

	if len(jobs) == 0 {
		log.Printf("⚠️ Записи billing_start не найдены в таблице snapshot_jobs")
		// Проверяем, есть ли вообще записи в таблице
		var totalJobs int64
		if err := publicDB.Model(&models.SnapshotJob{}).Count(&totalJobs).Error; err == nil {
			log.Printf("   Всего записей в таблице snapshot_jobs: %d", totalJobs)
		}
		c.JSON(http.StatusOK, gin.H{
			"status":     "not_found",
			"message":    "Записи о стартовой дате биллинга не найдены в истории",
			"hint":       "Вызовите GET /api/auth/snapshots/billing-start-date для создания записи",
			"total_jobs": totalJobs,
		})
		return
	}

	// Форматируем найденные записи
	var formattedJobs []gin.H
	for _, job := range jobs {
		formattedJobs = append(formattedJobs, gin.H{
			"id":             job.ID,
			"job_type":       job.JobType,
			"date_from":      job.DateFrom.Format("2006-01-02"),
			"date_to":        job.DateTo.Format("2006-01-02"),
			"status":         job.Status,
			"total_objects":  job.TotalObjects,
			"active_objects": job.ActiveObjects,
			"started_at":     job.StartedAt.Format("2006-01-02 15:04:05"),
			"finished_at":    job.FinishedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "found",
		"count":   len(jobs),
		"jobs":    formattedJobs,
		"message": fmt.Sprintf("Найдено %d записей о стартовой дате биллинга", len(jobs)),
	})
}
