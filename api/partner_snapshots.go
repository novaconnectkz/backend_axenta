package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

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
	var startDate, endDate time.Time
	if startDateStr != "" {
		startDate, err = time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			log.Printf("❌ Ошибка парсинга start_date: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат start_date",
			})
			return
		}
	} else {
		// По умолчанию - последние 30 дней
		startDate = time.Now().AddDate(0, 0, -30)
	}

	if endDateStr != "" {
		endDate, err = time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			log.Printf("❌ Ошибка парсинга end_date: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат end_date",
			})
			return
		}
		// Если endDate имеет время 00:00:00 (начало дня), добавляем время до конца дня
		// чтобы включить все снимки за этот день
		if endDate.Hour() == 0 && endDate.Minute() == 0 && endDate.Second() == 0 {
			endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, endDate.Location())
			log.Printf("📅 endDate был началом дня, установлен конец дня: %s", endDate.Format(time.RFC3339))
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
		existingSnapshotsByDate := make(map[string]bool)
		for _, snapshot := range snapshots {
			dateKey := snapshot.SnapshotDate.Format("2006-01-02")
			existingSnapshotsByDate[dateKey] = true
		}

		// Генерируем все даты в периоде
		currentDate := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
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

			// Если снимка нет для этой даты, пытаемся получить данные из axenta_account_snapshots
			if !existingSnapshotsByDate[dateKey] {
				log.Printf("🔍 Снимок для даты %s не найден, ищем в axenta_account_snapshots для partner_company_id=%d", dateKey, *contract.PartnerCompanyID)

				// Ищем ближайший снимок в axenta_account_snapshots (до или в этот день)
				var axentaSnapshot models.AxentaAccountSnapshot
				// Ищем снимок, который был синхронизирован до или в этот день
				snapshotEndOfDay := time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 23, 59, 59, 999999999, time.UTC)

				if err := db.Where("external_account_id = ? AND admin_account_id = ? AND last_synced_at <= ?",
					int64(*contract.PartnerCompanyID), adminAccountID, snapshotEndOfDay).
					Order("last_synced_at DESC").
					First(&axentaSnapshot).Error; err == nil {

					log.Printf("✅ Найден снимок axenta_account_snapshots для даты %s: активных=%d, всего=%d (last_synced_at=%s)",
						dateKey, axentaSnapshot.ObjectsActive, axentaSnapshot.ObjectsTotal, axentaSnapshot.LastSyncedAt.Format("2006-01-02 15:04:05"))

					// Создаем виртуальный снимок на основе данных из axenta_account_snapshots
					if !tariffPlan.Price.IsZero() {
						// Рассчитываем дневную цену
						dailyPrice := tariffPlan.Price.Div(decimal.NewFromInt(30))

						// Рассчитываем скидку
						discountPercent := contract.GetDiscountPercent(axentaSnapshot.ObjectsActive)
						discountFixed := contract.GetDiscountFixed()

						// Стоимость до скидки
						costBeforeDiscount := dailyPrice.Mul(decimal.NewFromInt(int64(axentaSnapshot.ObjectsActive)))

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
							discountAmount = discountFixed.Div(decimal.NewFromInt(30)).Mul(decimal.NewFromInt(int64(axentaSnapshot.ObjectsActive)))
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
							TotalObjectsCount:  axentaSnapshot.ObjectsTotal,
							ActiveObjectsCount: axentaSnapshot.ObjectsActive,
							DiscountType:       discountType,
							DiscountPercent:    discountPercent,
							DiscountFixed:      discountFixed,
							CostBeforeDiscount: costBeforeDiscount.Round(4),
							DiscountAmount:     discountAmount.Round(4),
							DailyCost:          dailyCost.Round(4),
							Status:             "completed",
							Notes:              fmt.Sprintf("Создан из axenta_account_snapshots (last_synced_at: %s)", axentaSnapshot.LastSyncedAt.Format("2006-01-02 15:04:05")),
						}

						snapshots = append(snapshots, virtualSnapshot)
						log.Printf("✅ Создан виртуальный снимок для даты %s: активных=%d, стоимость=%.2f₽",
							dateKey, axentaSnapshot.ObjectsActive, dailyCost.InexactFloat64())
					} else {
						log.Printf("⚠️ Не удалось создать виртуальный снимок: тарифный план не найден или цена = 0")
					}
				} else {
					log.Printf("⚠️ Не найден снимок в axenta_account_snapshots для даты %s (partner_company_id=%d): %v",
						dateKey, *contract.PartnerCompanyID, err)
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
	effectiveMonthlyPrice := effectiveDailyPrice.Mul(decimal.NewFromInt(30))

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
		"total_cost":                  totalCost.InexactFloat64(),
		"avg_objects":                 avgObjectsDecimal.InexactFloat64(),     // Точное среднее с decimal
		"daily_price":                 effectiveDailyPrice.InexactFloat64(),   // Реальная дневная цена С УЧЕТОМ скидки
		"monthly_price":               effectiveMonthlyPrice.InexactFloat64(), // Реальная месячная цена С УЧЕТОМ скидки
		"total_objects":               totalObjects,
		"price_per_object_for_period": pricePerObjectForPeriod.InexactFloat64(), // Реальная цена С УЧЕТОМ скидки
		"base_monthly_price":          baseMonthlyPrice.InexactFloat64(),        // Базовая цена БЕЗ скидки (для справки)
		"base_daily_price":            baseDailyPrice.InexactFloat64(),          // Базовая дневная цена БЕЗ скидки (для справки)
		"total_discount":              totalDiscountAmount.InexactFloat64(),     // Общая сумма скидки за период
		"discount_type":               discountType,                             // Тип скидки
		"avg_daily_discount":          avgDailyDiscount.InexactFloat64(),        // Средняя дневная скидка
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
		StartDate string `json:"start_date"` // Формат: YYYY-MM-DD
		EndDate   string `json:"end_date"`   // Формат: YYYY-MM-DD
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

	// Создаем снимки для каждого дня в периоде
	successCount := 0
	errorCount := 0
	currentDate := startDate

	for !currentDate.After(endDate) {
		// Устанавливаем время на начало дня в UTC
		snapshotDate := time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 0, 0, 0, 0, time.UTC)

		// Создаем снимок для этой даты
		if err := snapshotService.CreateSnapshotForContractWithTokenAndDB(&contract, snapshotDate, userToken, db); err != nil {
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
