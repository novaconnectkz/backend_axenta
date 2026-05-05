package api

import (
	"fmt"
	"log"
	"net/http"
	"net/url"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// GetContractBillingAnalysis возвращает полный анализ биллинга по договору (отладочный эндпоинт)
func GetContractBillingAnalysis(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	contractNumber := c.Param("number")
	if contractNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не указан номер договора",
		})
		return
	}

	// Декодируем номер договора на случай, если он был URL-encoded
	decodedNumber, err := url.QueryUnescape(contractNumber)
	if err == nil && decodedNumber != contractNumber {
		log.Printf("🔍 Декодирован номер договора: %s → %s", contractNumber, decodedNumber)
		contractNumber = decodedNumber
	}

	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных компании",
		})
		return
	}

	// Ищем договор по номеру
	var contract models.Contract
	if err := tenantDB.Where("number = ?", contractNumber).First(&contract).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Договор с номером %s не найден", contractNumber),
		})
		return
	}

	// Получаем все счета по договору
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var invoices []models.Invoice
	if err := publicDB.
		Where("contract_id = ? AND admin_account_id = ?", contract.ID, adminAccountID).
		Order("invoice_date DESC").
		Find(&invoices).Error; err != nil {
		log.Printf("⚠️ Ошибка получения счетов: %v", err)
	}

	// Получаем историю платежей
	var billingHistory []models.BillingHistory
	if err := publicDB.
		Where("contract_id = ? AND admin_account_id = ?", contract.ID, adminAccountID).
		Order("created_at DESC").
		Find(&billingHistory).Error; err != nil {
		log.Printf("⚠️ Ошибка получения истории: %v", err)
	}

	// Подсчитываем статистику
	totalInvoiced := 0.0
	totalPaid := 0.0
	totalOutstanding := 0.0

	for _, invoice := range invoices {
		totalAmount := parseDecimal(invoice.TotalAmount)
		paidAmount := parseDecimal(invoice.PaidAmount)
		outstanding := totalAmount - paidAmount

		totalInvoiced += totalAmount
		totalPaid += paidAmount
		if invoice.Status != "paid" && invoice.Status != "cancelled" {
			totalOutstanding += outstanding
		}
	}

	// Формируем ответ
	response := gin.H{
		"status": "success",
		"data": gin.H{
			"contract": gin.H{
				"id":            contract.ID,
				"number":        contract.Number,
				"title":         contract.Title,
				"client_name":   contract.ClientName,
				"contract_type": contract.ContractType,
				"status":        contract.Status,
				"start_date":    contract.StartDate,
				"end_date":      contract.EndDate,
			},
			"statistics": gin.H{
				"total_invoiced":    totalInvoiced,
				"total_paid":        totalPaid,
				"total_outstanding": totalOutstanding,
				"invoices_count":    len(invoices),
				"paid_invoices":     countByStatus(invoices, "paid"),
				"unpaid_invoices":   countUnpaid(invoices),
			},
			"invoices":        formatInvoices(invoices),
			"payment_history": formatBillingHistory(billingHistory),
		},
	}

	c.JSON(http.StatusOK, response)
}

// Вспомогательные функции
func parseDecimal(d decimal.Decimal) float64 {
	f, _ := d.Float64()
	return f
}

func countByStatus(invoices []models.Invoice, status string) int {
	count := 0
	for _, inv := range invoices {
		if inv.Status == status {
			count++
		}
	}
	return count
}

func countUnpaid(invoices []models.Invoice) int {
	count := 0
	for _, inv := range invoices {
		if inv.Status != "paid" && inv.Status != "cancelled" {
			count++
		}
	}
	return count
}

func formatInvoices(invoices []models.Invoice) []gin.H {
	result := make([]gin.H, 0, len(invoices))
	for _, inv := range invoices {
		totalAmount := parseDecimal(inv.TotalAmount)
		paidAmount := parseDecimal(inv.PaidAmount)
		result = append(result, gin.H{
			"id":                   inv.ID,
			"number":               inv.Number,
			"invoice_date":         inv.InvoiceDate,
			"due_date":             inv.DueDate,
			"total_amount":         totalAmount,
			"paid_amount":          paidAmount,
			"outstanding":          totalAmount - paidAmount,
			"status":               inv.Status,
			"billing_period_start": inv.BillingPeriodStart,
			"billing_period_end":   inv.BillingPeriodEnd,
		})
	}
	return result
}

func formatBillingHistory(history []models.BillingHistory) []gin.H {
	result := make([]gin.H, 0, len(history))
	for _, h := range history {
		result = append(result, gin.H{
			"id":          h.ID,
			"operation":   h.Operation,
			"amount":      parseDecimal(h.Amount),
			"currency":    h.Currency,
			"description": h.Description,
			"status":      h.Status,
			"created_at":  h.CreatedAt,
			"invoice_id":  h.InvoiceID,
		})
	}
	return result
}
