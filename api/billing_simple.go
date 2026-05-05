package api

import (
	"github.com/gin-gonic/gin"
)

// GetBillingPlansSimple получает упрощенные планы биллинга
func GetBillingPlansSimple(c *gin.Context) {
	// Возвращаем пустой массив вместо демо-данных
	plans := []map[string]interface{}{}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   plans,
	})
}

// GetSubscriptionsSimple получает упрощенные подписки
func GetSubscriptionsSimple(c *gin.Context) {
	// Возвращаем пустой массив вместо демо-данных
	subscriptions := []map[string]interface{}{}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   subscriptions,
	})
}

// GetBillingSettingsSimple получает упрощенные настройки биллинга
func GetBillingSettingsSimple(c *gin.Context) {
	settings := map[string]interface{}{
		"id":                        1,
		"company_id":                24,
		"auto_generate_invoices":    true,
		"invoice_generation_day":    1,
		"invoice_payment_term_days": 14,
		"default_tax_rate":          "20.00",
		"tax_included":              false,
		"notify_before_invoice":     3,
		"notify_before_due":         3,
		"notify_overdue":            1,
		"invoice_number_prefix":     "INV",
		"invoice_number_format":     "%s-%04d",
		"currency":                  "RUB",
		"default_payment_method":    "bank_transfer",
		"allow_partial_payments":    true,
		"require_payment_confirm":   false,
		"enable_inactive_discounts": true,
		"inactive_discount_ratio":   "0.50",
		"min_days_for_full_month":   5,
		"created_at":                "2024-01-01T00:00:00Z",
		"updated_at":                "2024-01-01T00:00:00Z",
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   settings,
	})
}
