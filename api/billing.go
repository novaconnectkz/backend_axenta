package api

import (
	"net/http"
	"strconv"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
)

// GetBillingPlans получает список всех тарифных планов
func GetBillingPlans(c *gin.Context) {
	var plans []models.BillingPlan

	// Получаем только активные планы по умолчанию
	query := database.DB.Where("is_active = ?", true)

	// Опциональная фильтрация по компании
	if companyID := c.Query("company_id"); companyID != "" {
		query = query.Where("company_id = ? OR company_id IS NULL", companyID)
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

	var plan models.BillingPlan
	if err := database.DB.First(&plan, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден",
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

	if err := c.ShouldBindJSON(&plan); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Валидация обязательных полей
	if plan.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Название тарифного плана обязательно",
		})
		return
	}

	if plan.Price < 0 {
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

	if err := database.DB.Create(&plan).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при создании тарифного плана",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   plan,
	})
}

// UpdateBillingPlan обновляет существующий тарифный план
func UpdateBillingPlan(c *gin.Context) {
	id := c.Param("id")

	var plan models.BillingPlan
	if err := database.DB.First(&plan, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден",
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
	if updateData.Price < 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Цена не может быть отрицательной",
		})
		return
	}

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

	var plan models.BillingPlan
	if err := database.DB.First(&plan, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден",
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
