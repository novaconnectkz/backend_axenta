package api

import (
	"github.com/gin-gonic/gin"
)

// GetBillingPlansSimple получает упрощенные планы биллинга
func GetBillingPlansSimple(c *gin.Context) {
	plans := []map[string]interface{}{
		{
			"id":          1,
			"name":        "Базовый",
			"price":       1000.0,
			"currency":    "RUB",
			"description": "Базовый тарифный план",
			"features":    []string{"До 10 объектов", "Базовая поддержка"},
			"is_active":   true,
		},
		{
			"id":          2,
			"name":        "Профессиональный",
			"price":       2500.0,
			"currency":    "RUB",
			"description": "Расширенный тарифный план",
			"features":    []string{"До 50 объектов", "Приоритетная поддержка", "Отчеты"},
			"is_active":   true,
		},
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   plans,
	})
}

// GetSubscriptionsSimple получает упрощенные подписки
func GetSubscriptionsSimple(c *gin.Context) {
	subscriptions := []map[string]interface{}{
		{
			"id":         1,
			"plan_name":  "Профессиональный",
			"status":     "active",
			"start_date": "2025-01-01T00:00:00Z",
			"end_date":   "2025-12-31T23:59:59Z",
			"price":      2500.0,
			"currency":   "RUB",
		},
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   subscriptions,
	})
}
