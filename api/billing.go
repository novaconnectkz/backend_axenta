package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// GetBillingPlans получает список всех тарифных планов
func GetBillingPlans(c *gin.Context) {
	var plans []models.BillingPlan

	// Убеждаемся, что мы в схеме public для глобальных таблиц
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	// Получаем только активные планы по умолчанию
	query := database.DB.Where("is_active = ?", true)

	// Фильтрация по компании (обязательна для изоляции)
	if companyID := c.Query("company_id"); companyID != "" {
		// Логируем для отладки
		fmt.Printf("GetBillingPlans: фильтруем по company_id=%s\n", companyID)

		// Парсим company_id в uint для корректного сравнения
		if companyIDUint, err := strconv.ParseUint(companyID, 10, 32); err == nil {
			// Возвращаем только планы этой компании
			query = query.Where("company_id = ?", uint(companyIDUint))
			fmt.Printf("GetBillingPlans: используем company_id=%d (uint)\n", uint(companyIDUint))
		} else {
			fmt.Printf("GetBillingPlans: ошибка парсинга company_id: %v\n", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Неверный формат company_id: %v", err),
			})
			return
		}
	} else {
		// Если company_id не указан, возвращаем пустой список (безопасность)
		fmt.Printf("GetBillingPlans: company_id не указан, возвращаем пустой список\n")
		query = query.Where("1 = 0") // Никогда не вернет результаты
	}

	if err := query.Find(&plans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при получении тарифных планов",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   plans,
		"count":  len(plans),
	})
}

// GetBillingPlan получает конкретный тарифный план по ID
func GetBillingPlan(c *gin.Context) {
	id := c.Param("id")
	companyIDStr := c.Query("company_id")

	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var plan models.BillingPlan
	query := database.DB.Where("id = ?", id)

	// Проверяем принадлежность к компании
	if companyIDStr != "" {
		query = query.Where("company_id = ?", companyIDStr)
	} else {
		// Если company_id не указан, возвращаем ошибку (безопасность)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	if err := query.First(&plan).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден или не принадлежит вашей компании",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   plan,
	})
}

// CreateBillingPlan создает новый тарифный план
func CreateBillingPlan(c *gin.Context) {
	var plan models.BillingPlan

	// Логируем входящие данные для отладки
	fmt.Printf("CreateBillingPlan: получен запрос, query params: %v\n", c.Request.URL.Query())

	if err := c.ShouldBindJSON(&plan); err != nil {
		fmt.Printf("CreateBillingPlan: ошибка парсинга JSON: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Неверный формат данных: %v", err),
		})
		return
	}

	fmt.Printf("CreateBillingPlan: распарсенные данные плана: Name=%s, Price=%s, CompanyID=%v\n",
		plan.Name, plan.Price.String(), plan.CompanyID)

	// Валидация обязательных полей
	if plan.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Название тарифного плана обязательно",
		})
		return
	}

	// Проверяем, что Price корректно установлен
	if plan.Price.IsZero() && plan.Price.IsNegative() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Цена должна быть положительным числом",
		})
		return
	}

	if plan.Price.LessThan(decimal.Zero) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Цена не может быть отрицательной",
		})
		return
	}

	// Устанавливаем значения по умолчанию
	if plan.Currency == "" {
		plan.Currency = "RUB"
	}
	if plan.BillingPeriod == "" {
		plan.BillingPeriod = "monthly"
	}

	// Устанавливаем company_id из query параметра (если не указан в теле запроса)
	if plan.CompanyID == nil {
		companyIDStr := c.Query("company_id")
		fmt.Printf("CreateBillingPlan: company_id из query: %s\n", companyIDStr)
		if companyIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Параметр company_id обязателен для создания тарифного плана",
			})
			return
		}
		if companyID, err := strconv.ParseUint(companyIDStr, 10, 32); err == nil {
			companyIDUint := uint(companyID)
			plan.CompanyID = &companyIDUint
			fmt.Printf("CreateBillingPlan: установлен company_id: %d\n", companyIDUint)
		} else {
			fmt.Printf("CreateBillingPlan: ошибка парсинга company_id: %v\n", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Неверный формат company_id: %v", err),
			})
			return
		}
	}

	// Убеждаемся, что мы в схеме public для глобальных таблиц
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		fmt.Printf("CreateBillingPlan: ошибка установки search_path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка подключения к базе данных: %v", err),
		})
		return
	}

	fmt.Printf("CreateBillingPlan: создаем план с данными: Name=%s, Price=%s, CompanyID=%v\n",
		plan.Name, plan.Price.String(), plan.CompanyID)

	// Проверяем, нет ли уже плана с таким же именем в этой компании
	var existingPlan models.BillingPlan
	if plan.CompanyID != nil {
		if err := database.DB.Where("name = ? AND company_id = ?", plan.Name, *plan.CompanyID).First(&existingPlan).Error; err == nil {
			fmt.Printf("CreateBillingPlan: план с именем '%s' уже существует для компании %d\n", plan.Name, *plan.CompanyID)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Тарифный план с названием '%s' уже существует для вашей компании", plan.Name),
			})
			return
		}
	}

	if err := database.DB.Create(&plan).Error; err != nil {
		// Логируем детальную информацию об ошибке
		fmt.Printf("CreateBillingPlan: ОШИБКА при создании тарифного плана: %v\n", err)
		fmt.Printf("CreateBillingPlan: Данные плана: Name=%s, Price=%s, CompanyID=%v, Currency=%s, BillingPeriod=%s\n",
			plan.Name, plan.Price.String(), plan.CompanyID, plan.Currency, plan.BillingPeriod)

		// Проверяем тип ошибки для более понятного сообщения
		errorMsg := "Ошибка при создании тарифного плана"
		errStr := err.Error()

		// Проверяем на дубликат имени (может быть из-за старого индекса)
		if strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "UNIQUE constraint") || strings.Contains(errStr, "violates unique constraint") {
			errorMsg = fmt.Sprintf("Тарифный план с названием '%s' уже существует", plan.Name)
		} else if errStr != "" {
			errorMsg = fmt.Sprintf("Ошибка при создании тарифного плана: %v", err)
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  errorMsg,
		})
		return
	}

	fmt.Printf("CreateBillingPlan: план успешно создан с ID: %d\n", plan.ID)

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   plan,
	})
}

// UpdateBillingPlan обновляет существующий тарифный план
func UpdateBillingPlan(c *gin.Context) {
	id := c.Param("id")
	companyIDStr := c.Query("company_id")

	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var plan models.BillingPlan
	query := database.DB.Where("id = ?", id)

	// Проверяем принадлежность к компании
	if companyIDStr != "" {
		query = query.Where("company_id = ?", companyIDStr)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	if err := query.First(&plan).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден или не принадлежит вашей компании",
		})
		return
	}

	var updateData models.BillingPlan
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Валидация
	if updateData.Price.LessThan(decimal.Zero) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Цена не может быть отрицательной",
		})
		return
	}

	// Игнорируем попытку изменить company_id через обновление
	updateData.CompanyID = nil

	if err := database.DB.Model(&plan).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при обновлении тарифного плана",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   plan,
	})
}

// DeleteBillingPlan удаляет тарифный план (мягкое удаление)
func DeleteBillingPlan(c *gin.Context) {
	id := c.Param("id")
	companyIDStr := c.Query("company_id")

	// Параметр company_id обязателен
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	// Парсим company_id в uint
	companyIDUint, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		fmt.Printf("DeleteBillingPlan: ошибка парсинга company_id: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Неверный формат company_id: %v", err),
		})
		return
	}
	companyID := uint(companyIDUint)

	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		fmt.Printf("DeleteBillingPlan: ошибка установки search_path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var plan models.BillingPlan
	query := database.DB.Where("id = ? AND company_id = ?", id, companyID)

	if err := query.First(&plan).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден или не принадлежит вашей компании",
		})
		return
	}

	// Проверяем, есть ли активные подписки на этот план (только для этой компании)
	var subscriptionCount int64
	database.DB.Model(&models.Subscription{}).Where("billing_plan_id = ? AND company_id = ? AND status = 'active'", plan.ID, companyID).Count(&subscriptionCount)

	if subscriptionCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Нельзя удалить тарифный план с активными подписками",
		})
		return
	}

	if err := database.DB.Delete(&plan).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при удалении тарифного плана",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Тарифный план успешно удален",
	})
}

// GetSubscriptions получает список подписок для компании
func GetSubscriptions(c *gin.Context) {
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат company_id",
		})
		return
	}

	var subscriptions []models.Subscription
	if err := database.DB.Preload("BillingPlan").Where("company_id = ?", uint(companyID)).Find(&subscriptions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при получении подписок",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   subscriptions,
		"count":  len(subscriptions),
	})
}

// CreateSubscription создает новую подписку
func CreateSubscription(c *gin.Context) {
	var subscription models.Subscription

	if err := c.ShouldBindJSON(&subscription); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Валидация обязательных полей
	if subscription.CompanyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Поле company_id обязательно",
		})
		return
	}

	if subscription.BillingPlanID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Поле billing_plan_id обязательно",
		})
		return
	}

	// Проверяем существование тарифного плана
	var plan models.BillingPlan
	if err := database.DB.First(&plan, subscription.BillingPlanID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден",
		})
		return
	}

	// Устанавливаем значения по умолчанию
	if subscription.StartDate.IsZero() {
		subscription.StartDate = time.Now()
	}
	if subscription.Status == "" {
		subscription.Status = "active"
	}

	// Вычисляем дату следующего платежа
	if subscription.NextPaymentDate == nil && plan.BillingPeriod != "one-time" {
		var nextPayment time.Time
		switch plan.BillingPeriod {
		case "monthly":
			nextPayment = subscription.StartDate.AddDate(0, 1, 0)
		case "yearly":
			nextPayment = subscription.StartDate.AddDate(1, 0, 0)
		default:
			nextPayment = subscription.StartDate.AddDate(0, 1, 0)
		}
		subscription.NextPaymentDate = &nextPayment
	}

	if err := database.DB.Create(&subscription).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при создании подписки",
		})
		return
	}

	// Загружаем связанные данные для ответа
	database.DB.Preload("BillingPlan").First(&subscription, subscription.ID)

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   subscription,
	})
}

// UpdateSubscription обновляет статус подписки
func UpdateSubscription(c *gin.Context) {
	id := c.Param("id")

	var subscription models.Subscription
	if err := database.DB.First(&subscription, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Подписка не найдена",
		})
		return
	}

	var updateData models.Subscription
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	if err := database.DB.Model(&subscription).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при обновлении подписки",
		})
		return
	}

	// Загружаем обновленные данные с связями
	database.DB.Preload("BillingPlan").First(&subscription, subscription.ID)

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   subscription,
	})
}

// DeleteSubscription удаляет подписку
func DeleteSubscription(c *gin.Context) {
	id := c.Param("id")

	var subscription models.Subscription
	if err := database.DB.First(&subscription, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Подписка не найдена",
		})
		return
	}

	if err := database.DB.Delete(&subscription).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при удалении подписки",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Подписка успешно удалена",
	})
}

// ===== НОВЫЕ ENDPOINTS ДЛЯ СИСТЕМЫ БИЛЛИНГА =====

// CalculateBilling рассчитывает стоимость биллинга для договора
func CalculateBilling(c *gin.Context) {
	contractIDStr := c.Param("contract_id")
	contractID, err := strconv.ParseUint(contractIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат contract_id",
		})
		return
	}

	// Получаем параметры периода
	periodStartStr := c.Query("period_start")
	periodEndStr := c.Query("period_end")

	var periodStart, periodEnd time.Time

	if periodStartStr != "" && periodEndStr != "" {
		periodStart, err = time.Parse("2006-01-02", periodStartStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат period_start (ожидается YYYY-MM-DD)",
			})
			return
		}

		periodEnd, err = time.Parse("2006-01-02", periodEndStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат period_end (ожидается YYYY-MM-DD)",
			})
			return
		}
	} else {
		// По умолчанию текущий месяц
		now := time.Now()
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodEnd = periodStart.AddDate(0, 1, -1)
	}

	// Создаем сервис биллинга
	billingService := services.NewBillingService()

	// Рассчитываем биллинг
	calculation, err := billingService.CalculateBillingForContract(uint(contractID), periodStart, periodEnd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   calculation,
	})
}

// GenerateInvoice создает счет для договора
func GenerateInvoice(c *gin.Context) {
	contractIDStr := c.Param("contract_id")
	contractID, err := strconv.ParseUint(contractIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат contract_id",
		})
		return
	}

	// Получаем параметры из тела запроса
	var requestData struct {
		PeriodStart string `json:"period_start"`
		PeriodEnd   string `json:"period_end"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	var periodStart, periodEnd time.Time

	if requestData.PeriodStart != "" && requestData.PeriodEnd != "" {
		periodStart, err = time.Parse("2006-01-02", requestData.PeriodStart)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат period_start (ожидается YYYY-MM-DD)",
			})
			return
		}

		periodEnd, err = time.Parse("2006-01-02", requestData.PeriodEnd)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат period_end (ожидается YYYY-MM-DD)",
			})
			return
		}
	} else {
		// По умолчанию текущий месяц
		now := time.Now()
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodEnd = periodStart.AddDate(0, 1, -1)
	}

	// Создаем сервис биллинга
	billingService := services.NewBillingService()

	// Генерируем счет
	invoice, err := billingService.GenerateInvoiceForContract(uint(contractID), periodStart, periodEnd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   invoice,
	})
}

// GetInvoices получает список счетов
func GetInvoices(c *gin.Context) {
	// Проверяем demo-режим
	if isDemoMode(c) {
		demoInvoices := []models.Invoice{
			{
				ID:             1,
				Number:         "INV-2024-001",
				Title:          "Счет за услуги мониторинга",
				InvoiceDate:    time.Now().AddDate(0, 0, -5),
				DueDate:        time.Now().AddDate(0, 0, 9),
				SubtotalAmount: decimal.NewFromInt(10000),
				TaxAmount:      decimal.NewFromInt(2000),
				TotalAmount:    decimal.NewFromInt(12000),
				PaidAmount:     decimal.Zero,
				Currency:       "RUB",
				Status:         "sent",
			},
		}

		c.JSON(http.StatusOK, gin.H{
			"status":      "success",
			"data":        demoInvoices,
			"count":       len(demoInvoices),
			"total":       1,
			"demo_notice": "Это демо-данные. Добавьте ?demo=0 для получения реальных данных.",
		})
		return
	}

	companyIDStr := c.Query("company_id")
	contractIDStr := c.Query("contract_id")
	status := c.Query("status")

	// Пагинация
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// Убеждаемся, что мы в схеме public для глобальных таблиц
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		fmt.Printf("GetInvoices: ошибка установки search_path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка подключения к базе данных: %v", err),
		})
		return
	}

	// Базовый запрос - без Preload сначала для фильтрации
	query := database.DB.Model(&models.Invoice{})

	// Фильтрация по компании обязательна для изоляции
	if companyIDStr != "" {
		companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
		if err != nil {
			fmt.Printf("GetInvoices: ошибка парсинга company_id: %v\n", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Неверный формат company_id: %v", err),
			})
			return
		}
		fmt.Printf("GetInvoices: фильтруем по company_id=%d\n", uint(companyID))
		query = query.Where("company_id = ?", uint(companyID))
	} else {
		// Если company_id не указан, возвращаем пустой список (безопасность)
		fmt.Printf("GetInvoices: company_id не указан, возвращаем пустой список\n")
		query = query.Where("1 = 0") // Никогда не вернет результаты
	}

	if contractIDStr != "" {
		contractID, err := strconv.ParseUint(contractIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат contract_id",
			})
			return
		}
		query = query.Where("contract_id = ?", uint(contractID))
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Подсчитываем общее количество (без Preload)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		fmt.Printf("GetInvoices: ОШИБКА при подсчете счетов: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка при подсчете счетов: %v", err),
		})
		return
	}

	fmt.Printf("GetInvoices: найдено счетов: %d\n", total)

	// Добавляем Preload только для основных записей (без Contract, так как он в tenant схеме)
	// TariffPlan нужно загружать из billing_plans, но в модели Invoice используется TariffPlan
	// Попробуем загрузить только Items, а TariffPlan попробуем через Join или отдельно
	queryWithPreload := query.Preload("Items")

	// Получаем счета с пагинацией
	var invoices []models.Invoice
	if err := queryWithPreload.Limit(limit).Offset(offset).Order("created_at DESC").Find(&invoices).Error; err != nil {
		fmt.Printf("GetInvoices: ОШИБКА при получении счетов: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка при получении счетов: %v", err),
		})
		return
	}

	fmt.Printf("GetInvoices: успешно получено %d счетов\n", len(invoices))

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   invoices,
		"count":  len(invoices),
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetInvoice получает конкретный счет по ID
func GetInvoice(c *gin.Context) {
	id := c.Param("id")

	var invoice models.Invoice
	if err := database.DB.Preload("Contract").Preload("TariffPlan").Preload("Items").First(&invoice, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Счет не найден",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   invoice,
	})
}

// SendInvoice отправляет счет клиенту (POST /api/invoices/:id/send согласно roadmap)
func SendInvoice(c *gin.Context) {
	id := c.Param("id")
	invoiceID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат ID счета",
		})
		return
	}

	// Получаем счет
	var invoice models.Invoice
	if err := database.DB.First(&invoice, invoiceID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Счет не найден",
		})
		return
	}

	// Проверяем, можно ли отправить счет
	if invoice.Status == "paid" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Нельзя отправить уже оплаченный счет",
		})
		return
	}

	if invoice.Status == "cancelled" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Нельзя отправить отмененный счет",
		})
		return
	}

	// Обновляем статус на "sent"
	invoice.Status = "sent"
	if err := database.DB.Save(&invoice).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка обновления статуса счета",
		})
		return
	}

	// TODO: Здесь можно добавить отправку email/уведомления клиенту

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Счет успешно отправлен",
		"data":    invoice,
	})
}

// ProcessPayment обрабатывает платеж по счету (POST /api/invoices/:id/pay согласно roadmap)
func ProcessPayment(c *gin.Context) {
	id := c.Param("id")
	invoiceID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат ID счета",
		})
		return
	}

	var paymentData struct {
		Amount        string `json:"amount" binding:"required"`
		PaymentMethod string `json:"payment_method" binding:"required"`
		Notes         string `json:"notes"`
	}

	if err := c.ShouldBindJSON(&paymentData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Парсим сумму
	amount, err := decimal.NewFromString(paymentData.Amount)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат суммы",
		})
		return
	}

	if amount.LessThanOrEqual(decimal.Zero) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Сумма должна быть больше нуля",
		})
		return
	}

	// Создаем сервис биллинга
	billingService := services.NewBillingService()

	// Обрабатываем платеж
	if err := billingService.ProcessPayment(uint(invoiceID), amount, paymentData.PaymentMethod, paymentData.Notes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Получаем обновленный счет
	var invoice models.Invoice
	if err := database.DB.Preload("Contract").Preload("TariffPlan").Preload("Items").First(&invoice, invoiceID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения обновленного счета",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Платеж успешно обработан",
		"data":    invoice,
	})
}

// CancelInvoice отменяет счет
func CancelInvoice(c *gin.Context) {
	id := c.Param("id")
	invoiceID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат ID счета",
		})
		return
	}

	var cancelData struct {
		Reason string `json:"reason" binding:"required"`
	}

	if err := c.ShouldBindJSON(&cancelData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Создаем сервис биллинга
	billingService := services.NewBillingService()

	// Отменяем счет
	if err := billingService.CancelInvoice(uint(invoiceID), cancelData.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Счет успешно отменен",
	})
}

// GetBillingHistory получает историю биллинга
func GetBillingHistory(c *gin.Context) {
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyID, err := uuid.Parse(companyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат company_id",
		})
		return
	}

	// Пагинация
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// Создаем сервис биллинга
	billingService := services.NewBillingService()

	// Получаем историю
	history, total, err := billingService.GetBillingHistory(companyID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   history,
		"count":  len(history),
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetOverdueInvoices получает просроченные счета
func GetOverdueInvoices(c *gin.Context) {
	companyIDStr := c.Query("company_id")
	var companyID *uint

	if companyIDStr != "" {
		cIDUint, err := strconv.ParseUint(companyIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат company_id",
			})
			return
		}
		cID := uint(cIDUint)
		companyID = &cID
	}

	// Создаем сервис биллинга
	billingService := services.NewBillingService()

	// Получаем просроченные счета
	invoices, err := billingService.GetOverdueInvoices(companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   invoices,
		"count":  len(invoices),
	})
}

// GetBillingSettings получает настройки биллинга для компании
func GetBillingSettings(c *gin.Context) {
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		fmt.Printf("GetBillingSettings: company_id не указан\n")
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyIDUint, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		fmt.Printf("GetBillingSettings: ошибка парсинга company_id: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Неверный формат company_id: %v", err),
		})
		return
	}
	companyID := uint(companyIDUint)
	fmt.Printf("GetBillingSettings: запрос для company_id=%d\n", companyID)

	// Убеждаемся, что мы в схеме public для глобальных таблиц
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		fmt.Printf("GetBillingSettings: ошибка установки search_path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка подключения к базе данных: %v", err),
		})
		return
	}

	var settings models.BillingSettings
	if err := database.DB.Where("company_id = ?", companyID).First(&settings).Error; err != nil {
		// Создаем настройки по умолчанию
		settings = models.BillingSettings{
			CompanyID:                  companyID,
			AutoGenerateInvoices:       true,
			InvoiceGenerationDay:       1,
			InvoicePaymentTermDays:     14,
			DefaultTaxRate:             decimal.NewFromFloat(20),
			TaxIncluded:                false,
			NotifyBeforeInvoice:        3,
			NotifyBeforeDue:            3,
			NotifyOverdue:              1,
			InvoiceNumberPrefix:        "INV",
			InvoiceNumberFormat:        "%s-%04d",
			Currency:                   "RUB",
			AllowPartialPayments:       true,
			RequirePaymentConfirm:      false,
			EnableInactiveDiscounts:    true,
			InactiveDiscountRatio:      decimal.NewFromFloat(0.5),
			ContractNumberingMethod:    "manual",
			ContractDefaultNumeratorID: nil,
			Bitrix24DealNumberField:    "",
		}

		fmt.Printf("GetBillingSettings: настройки не найдены, создаем по умолчанию для company_id=%d\n", companyID)
		if err := database.DB.Create(&settings).Error; err != nil {
			fmt.Printf("GetBillingSettings: ОШИБКА при создании настроек: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Ошибка создания настроек биллинга: %v", err),
			})
			return
		}
		fmt.Printf("GetBillingSettings: настройки успешно созданы для company_id=%d\n", companyID)
	}

	fmt.Printf("GetBillingSettings: возвращаем настройки для company_id=%d\n", companyID)
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   settings,
	})
}

// UpdateBillingSettings обновляет настройки биллинга
func UpdateBillingSettings(c *gin.Context) {
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат company_id",
		})
		return
	}

	var settings models.BillingSettings
	if err := database.DB.Where("company_id = ?", uint(companyID)).First(&settings).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Настройки биллинга не найдены",
		})
		return
	}

	var updateData models.BillingSettings
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Валидация
	if updateData.InvoiceGenerationDay < 1 || updateData.InvoiceGenerationDay > 28 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "День генерации счета должен быть от 1 до 28",
		})
		return
	}

	if updateData.InvoicePaymentTermDays < 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Срок оплаты должен быть больше 0 дней",
		})
		return
	}

	if err := database.DB.Model(&settings).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при обновлении настроек биллинга",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   settings,
	})
}

// ===== ДОПОЛНИТЕЛЬНЫЕ ENDPOINTS ДЛЯ АВТОМАТИЗАЦИИ БИЛЛИНГА =====

// RunInvoicesGeneration запускает генерацию счетов (POST /api/invoices/run согласно roadmap)
func RunInvoicesGeneration(c *gin.Context) {
	// Получаем параметры из запроса
	var requestData struct {
		CompanyID uint   `json:"company_id"`
		Year      int    `json:"year"`
		Month     int    `json:"month"`
		Period    string `json:"period"` // Формат YYYY-MM, альтернатива year+month
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		// Если JSON не передан, пробуем получить из query параметров
		if companyIDStr := c.Query("company_id"); companyIDStr != "" {
			if id, err := strconv.ParseUint(companyIDStr, 10, 32); err == nil {
				requestData.CompanyID = uint(id)
			}
		}
		if periodStr := c.Query("period"); periodStr != "" {
			requestData.Period = periodStr
		}
		if yearStr := c.Query("year"); yearStr != "" {
			if y, err := strconv.Atoi(yearStr); err == nil {
				requestData.Year = y
			}
		}
		if monthStr := c.Query("month"); monthStr != "" {
			if m, err := strconv.Atoi(monthStr); err == nil {
				requestData.Month = m
			}
		}
	}

	// Определяем год и месяц
	var year, month int
	if requestData.Period != "" {
		// Парсим период в формате YYYY-MM
		periodTime, err := time.Parse("2006-01", requestData.Period)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат периода (ожидается YYYY-MM)",
			})
			return
		}
		year = periodTime.Year()
		month = int(periodTime.Month())
	} else {
		if requestData.Year == 0 {
			year = time.Now().Year()
		} else {
			year = requestData.Year
		}
		if requestData.Month == 0 {
			month = int(time.Now().Month())
		} else {
			month = requestData.Month
		}
	}

	// Создаем сервис автоматизации биллинга
	automationService := services.NewBillingAutomationService()

	// Генерируем счета
	if err := automationService.AutoGenerateInvoicesForMonth(year, month); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("Счета за %d-%02d успешно сгенерированы", year, month),
		"year":    year,
		"month":   month,
	})
}

// AutoGenerateInvoices автоматически генерирует счета за месяц
func AutoGenerateInvoices(c *gin.Context) {
	yearStr := c.Query("year")
	monthStr := c.Query("month")

	if yearStr == "" || monthStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметры year и month обязательны",
		})
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2000 || year > 3000 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат года",
		})
		return
	}

	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат месяца (1-12)",
		})
		return
	}

	// Создаем сервис автоматизации биллинга
	automationService := services.NewBillingAutomationService()

	// Генерируем счета
	if err := automationService.AutoGenerateInvoicesForMonth(year, month); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("Счета за %d-%02d успешно сгенерированы", year, month),
	})
}

// ProcessScheduledDeletions обрабатывает плановые удаления объектов
func ProcessScheduledDeletions(c *gin.Context) {
	// Создаем сервис автоматизации биллинга
	automationService := services.NewBillingAutomationService()

	// Обрабатываем плановые удаления
	if err := automationService.ProcessScheduledDeletions(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Плановые удаления успешно обработаны",
	})
}

// GetBillingStatistics получает статистику биллинга
func GetBillingStatistics(c *gin.Context) {
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат company_id",
		})
		return
	}

	yearStr := c.Query("year")
	monthStr := c.Query("month")

	if yearStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр year обязателен",
		})
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат года",
		})
		return
	}

	var month *int
	if monthStr != "" {
		m, err := strconv.Atoi(monthStr)
		if err != nil || m < 1 || m > 12 {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат месяца (1-12)",
			})
			return
		}
		month = &m
	}

	// Создаем сервис автоматизации биллинга
	automationService := services.NewBillingAutomationService()

	// Получаем статистику
	stats, err := automationService.GetBillingStatistics(uint(companyID), year, month)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// GetInvoicesByPeriod получает счета за период
func GetInvoicesByPeriod(c *gin.Context) {
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	companyIDStr := c.Query("company_id")

	if startDateStr == "" || endDateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметры start_date и end_date обязательны",
		})
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат start_date (ожидается YYYY-MM-DD)",
		})
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат end_date (ожидается YYYY-MM-DD)",
		})
		return
	}

	var companyID *uint
	if companyIDStr != "" {
		cID, err := strconv.ParseUint(companyIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат company_id",
			})
			return
		}
		companyIDUint := uint(cID)
		companyID = &companyIDUint
	}

	// Создаем сервис автоматизации биллинга
	automationService := services.NewBillingAutomationService()

	// Получаем счета за период
	invoices, err := automationService.GetInvoicesByPeriod(companyID, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   invoices,
		"count":  len(invoices),
	})
}
