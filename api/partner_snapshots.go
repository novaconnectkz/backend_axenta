package api

import (
	"log"
	"net/http"
	"time"

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

	contractID := c.Param("id")
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

	// Получаем снимки из базы данных (tenant schema)
	var snapshots []models.PartnerDailySnapshot
	if err := db.
		Where("contract_id = ? AND admin_account_id = ? AND snapshot_date >= ? AND snapshot_date <= ?",
			contractID, adminAccountID, startDate, endDate).
		Order("snapshot_date ASC").
		Find(&snapshots).Error; err != nil {
		log.Printf("❌ Ошибка получения снимков: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения снимков",
		})
		return
	}

	log.Printf("✅ Найдено снимков: %d", len(snapshots))

	// Рассчитываем сводную информацию
	summary := calculateSnapshotsSummary(snapshots)

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"snapshots": snapshots,
		"summary":   summary,
	})
}

// calculateSnapshotsSummary рассчитывает сводную информацию по снимкам
func calculateSnapshotsSummary(snapshots []models.PartnerDailySnapshot) map[string]interface{} {
	totalDays := len(snapshots)
	totalCost := decimal.Zero
	totalObjects := 0
	baseDailyPrice := decimal.Zero       // Базовая дневная цена БЕЗ скидки
	baseMonthlyPrice := decimal.Zero     // Базовая месячная цена БЕЗ скидки
	
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
		"avg_objects":                 avgObjectsDecimal.InexactFloat64(),       // Точное среднее с decimal
		"daily_price":                 effectiveDailyPrice.InexactFloat64(),      // Реальная дневная цена С УЧЕТОМ скидки
		"monthly_price":               effectiveMonthlyPrice.InexactFloat64(),    // Реальная месячная цена С УЧЕТОМ скидки
		"total_objects":               totalObjects,
		"price_per_object_for_period": pricePerObjectForPeriod.InexactFloat64(), // Реальная цена С УЧЕТОМ скидки
		"base_monthly_price":          baseMonthlyPrice.InexactFloat64(),        // Базовая цена БЕЗ скидки (для справки)
		"base_daily_price":            baseDailyPrice.InexactFloat64(),          // Базовая дневная цена БЕЗ скидки (для справки)
		"total_discount":              totalDiscountAmount.InexactFloat64(),      // Общая сумма скидки за период
		"discount_type":               discountType,                               // Тип скидки
		"avg_daily_discount":          avgDailyDiscount.InexactFloat64(),         // Средняя дневная скидка
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
		"status":         "success",
		"message":        "Снимки созданы",
		"success_count":  successCount,
		"error_count":    errorCount,
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

	contractID := c.Param("id")
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

	c.JSON(http.StatusOK, gin.H{
		"status":        "success",
		"message":       "Снимки за период созданы",
		"success_count": successCount,
		"error_count":   errorCount,
		"period_days":   int(endDate.Sub(startDate).Hours()/24) + 1,
	})
}

