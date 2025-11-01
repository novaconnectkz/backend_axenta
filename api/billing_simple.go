package api

import (
	"github.com/gin-gonic/gin"
)

// GetBillingPlansSimple получает упрощенные планы биллинга
func GetBillingPlansSimple(c *gin.Context) {
	plans := []map[string]interface{}{
		{
			"id":               1,
			"name":             "Базовый",
			"description":      "Базовый тарифный план",
			"price":            1000.0,
			"currency":         "RUB",
			"billing_period":   "monthly",
			"max_devices":      10,
			"max_users":        5,
			"max_storage":      10,
			"has_analytics":    false,
			"has_api":          false,
			"has_support":      true,
			"has_custom_domain": false,
			"is_active":        true,
			"is_popular":       false,
		},
		{
			"id":               2,
			"name":             "Профессиональный",
			"description":      "Расширенный тарифный план",
			"price":            2500.0,
			"currency":         "RUB",
			"billing_period":   "monthly",
			"max_devices":      0,
			"max_users":        0,
			"max_storage":      0,
			"has_analytics":    true,
			"has_api":          true,
			"has_support":      true,
			"has_custom_domain": true,
			"is_active":        true,
			"is_popular":       true,
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
			"company_id": 24,
			"billing_plan_id": 1,
			"billing_plan": map[string]interface{}{
				"id":              1,
				"name":            "Базовый",
				"description":     "Базовый тарифный план",
				"price":           1000.0,
				"currency":        "RUB",
				"billing_period":  "monthly",
				"max_devices":     10,
				"max_users":       5,
				"max_storage":     10,
				"has_analytics":   false,
				"has_api":         false,
				"has_support":     true,
				"has_custom_domain": false,
				"is_active":       true,
				"is_popular":      false,
			},
			"start_date":      "2024-01-01T00:00:00Z",
			"end_date":        nil,
			"status":          "active",
			"is_auto_renew":   true,
			"last_payment_date": nil,
			"next_payment_date": "2024-02-01T00:00:00Z",
			"payment_method":  "",
		},
		{
			"id":         2,
			"company_id": 24,
			"billing_plan_id": 2,
			"billing_plan": map[string]interface{}{
				"id":              2,
				"name":            "Профессиональный",
				"description":     "Расширенный тарифный план",
				"price":           2500.0,
				"currency":        "RUB",
				"billing_period":  "monthly",
				"max_devices":     0,
				"max_users":       0,
				"max_storage":     0,
				"has_analytics":   true,
				"has_api":         true,
				"has_support":     true,
				"has_custom_domain": true,
				"is_active":       true,
				"is_popular":      true,
			},
			"start_date":      "2024-06-01T00:00:00Z",
			"end_date":        nil,
			"status":          "active",
			"is_auto_renew":   true,
			"last_payment_date": "2024-06-01T00:00:00Z",
			"next_payment_date": "2024-07-01T00:00:00Z",
			"payment_method":  "card",
		},
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   subscriptions,
	})
}

// GetBillingSettingsSimple получает упрощенные настройки биллинга
func GetBillingSettingsSimple(c *gin.Context) {
	settings := map[string]interface{}{
		"id":                         1,
		"company_id":                 24,
		"auto_generate_invoices":     true,
		"invoice_generation_day":     1,
		"invoice_payment_term_days":  14,
		"default_tax_rate":           "20.00",
		"tax_included":               false,
		"notify_before_invoice":      3,
		"notify_before_due":          3,
		"notify_overdue":             1,
		"invoice_number_prefix":      "INV",
		"invoice_number_format":      "%s-%04d",
		"currency":                   "RUB",
		"default_payment_method":     "bank_transfer",
		"allow_partial_payments":     true,
		"require_payment_confirm":    false,
		"enable_inactive_discounts":  true,
		"inactive_discount_ratio":    "0.50",
		"created_at":                 "2024-01-01T00:00:00Z",
		"updated_at":                 "2024-01-01T00:00:00Z",
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   settings,
	})
}
