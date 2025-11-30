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

	// Парсим даты
	var startDate, endDate time.Time
	if startDateStr != "" {
		startDate, err = time.Parse(time.RFC3339, startDateStr)
		if err != nil {
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
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат end_date",
			})
			return
		}
	} else {
		endDate = time.Now()
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
	dailyPrice := decimal.Zero
	monthlyPrice := decimal.Zero

	for _, snapshot := range snapshots {
		totalCost = totalCost.Add(snapshot.DailyCost)
		totalObjects += snapshot.ActiveObjectsCount
		if dailyPrice.IsZero() {
			dailyPrice = snapshot.DailyPrice
			monthlyPrice = snapshot.MonthlyPrice
		}
	}

	avgObjects := 0
	if totalDays > 0 {
		avgObjects = totalObjects / totalDays
	}

	return map[string]interface{}{
		"total_days":     totalDays,
		"total_cost":     totalCost,
		"avg_objects":    avgObjects,
		"daily_price":    dailyPrice,
		"monthly_price":  monthlyPrice,
		"total_objects":  totalObjects,
	}
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

