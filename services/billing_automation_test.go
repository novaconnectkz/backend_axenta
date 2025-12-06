package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupBillingAutomationTestDB создает тестовую базу данных для billing automation
func setupBillingAutomationTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Contract{},
		&models.Invoice{},
		&models.Subscription{},
		&models.Object{},
		&models.BillingHistory{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// TestNewBillingAutomationService тестирует создание нового сервиса
func TestNewBillingAutomationService(t *testing.T) {
	setupBillingAutomationTestDB(t)

	service := NewBillingAutomationService(123)
	assert.NotNil(t, service)
	assert.Equal(t, uint(123), service.adminAccountID)
	assert.NotNil(t, service.db)
	assert.NotNil(t, service.billingService)
}

// TestBillingAutomationService_AutoGenerateInvoicesForMonth_NoContracts тестирует AutoGenerateInvoicesForMonth без договоров
func TestBillingAutomationService_AutoGenerateInvoicesForMonth_NoContracts(t *testing.T) {
	setupBillingAutomationTestDB(t)
	service := NewBillingAutomationService(123)

	now := time.Now()
	err := service.AutoGenerateInvoicesForMonth(now.Year(), int(now.Month()))

	// Должно вернуть nil, так как нет договоров
	assert.NoError(t, err)
}

// TestBillingAutomationService_ProcessScheduledDeletions_NoObjects тестирует ProcessScheduledDeletions без объектов
func TestBillingAutomationService_ProcessScheduledDeletions_NoObjects(t *testing.T) {
	setupBillingAutomationTestDB(t)
	service := NewBillingAutomationService(123)

	err := service.ProcessScheduledDeletions()
	assert.NoError(t, err)
}

// TestBillingAutomationService_ActivateScheduledSubscriptions_NoSubscriptions тестирует ActivateScheduledSubscriptions без подписок
func TestBillingAutomationService_ActivateScheduledSubscriptions_NoSubscriptions(t *testing.T) {
	setupBillingAutomationTestDB(t)
	service := NewBillingAutomationService(123)

	err := service.ActivateScheduledSubscriptions()
	assert.NoError(t, err)
}

// TestBillingAutomationService_GetInvoicesByPeriod_NoInvoices тестирует GetInvoicesByPeriod без счетов
func TestBillingAutomationService_GetInvoicesByPeriod_NoInvoices(t *testing.T) {
	setupBillingAutomationTestDB(t)
	service := NewBillingAutomationService(123)

	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)

	invoices, err := service.GetInvoicesByPeriod(nil, startDate, endDate)
	require.NoError(t, err)
	assert.Empty(t, invoices)
}

// TestBillingAutomationService_GetInvoicesByPeriod_WithCompanyID тестирует GetInvoicesByPeriod с companyID
func TestBillingAutomationService_GetInvoicesByPeriod_WithCompanyID(t *testing.T) {
	db := setupBillingAutomationTestDB(t)
	service := NewBillingAutomationService(123)

	// Создаем счет
	invoice := models.Invoice{
		AdminAccountID: 123,
		CompanyID:      456,
		InvoiceDate:    time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		TotalAmount:    decimal.NewFromInt(1000),
		Status:         "pending",
	}
	db.Create(&invoice)

	companyID := uint(456)
	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)

	invoices, err := service.GetInvoicesByPeriod(&companyID, startDate, endDate)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(invoices), 1)
}

// TestBillingAutomationService_GetBillingStatistics_NoInvoices тестирует GetBillingStatistics без счетов
func TestBillingAutomationService_GetBillingStatistics_NoInvoices(t *testing.T) {
	setupBillingAutomationTestDB(t)
	service := NewBillingAutomationService(123)

	month := 1
	stats, err := service.GetBillingStatistics(456, 2025, &month)
	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, uint(456), stats.CompanyID)
	assert.Equal(t, 2025, stats.Year)
	assert.Equal(t, 1, *stats.Month)
	assert.Equal(t, 0, stats.TotalInvoices)
	assert.True(t, stats.TotalAmount.IsZero())
}

// TestBillingAutomationService_GetBillingStatistics_WithMonth тестирует GetBillingStatistics с месяцем
func TestBillingAutomationService_GetBillingStatistics_WithMonth(t *testing.T) {
	db := setupBillingAutomationTestDB(t)
	service := NewBillingAutomationService(123)

	// Создаем счет
	invoice := models.Invoice{
		AdminAccountID: 123,
		CompanyID:      456,
		InvoiceDate:    time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		TotalAmount:    decimal.NewFromInt(1000),
		Status:         "paid",
	}
	db.Create(&invoice)

	month := 1
	stats, err := service.GetBillingStatistics(456, 2025, &month)
	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 1, stats.TotalInvoices)
	assert.Equal(t, decimal.NewFromInt(1000), stats.TotalAmount)
}

// TestBillingAutomationService_GetBillingStatistics_WithoutMonth тестирует GetBillingStatistics без месяца (за год)
func TestBillingAutomationService_GetBillingStatistics_WithoutMonth(t *testing.T) {
	db := setupBillingAutomationTestDB(t)
	service := NewBillingAutomationService(123)

	// Создаем счета за разные месяцы
	invoice1 := models.Invoice{
		AdminAccountID: 123,
		CompanyID:      456,
		InvoiceDate:    time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		TotalAmount:    decimal.NewFromInt(1000),
		Status:         "paid",
	}
	invoice2 := models.Invoice{
		AdminAccountID: 123,
		CompanyID:      456,
		InvoiceDate:    time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
		TotalAmount:    decimal.NewFromInt(2000),
		Status:         "paid",
	}
	db.Create(&invoice1)
	db.Create(&invoice2)

	stats, err := service.GetBillingStatistics(456, 2025, nil)
	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 2, stats.TotalInvoices)
	assert.Equal(t, decimal.NewFromInt(3000), stats.TotalAmount)
}

// TestBillingAutomationService_GetBillingStatistics_StatusCounts тестирует подсчет счетов по статусам
func TestBillingAutomationService_GetBillingStatistics_StatusCounts(t *testing.T) {
	db := setupBillingAutomationTestDB(t)
	service := NewBillingAutomationService(123)

	// Создаем счета с разными статусами
	invoice1 := models.Invoice{
		AdminAccountID: 123,
		CompanyID:      456,
		InvoiceDate:    time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		TotalAmount:    decimal.NewFromInt(1000),
		Status:         "paid",
	}
	invoice2 := models.Invoice{
		AdminAccountID: 123,
		CompanyID:      456,
		InvoiceDate:    time.Date(2025, 1, 16, 0, 0, 0, 0, time.UTC),
		TotalAmount:    decimal.NewFromInt(2000),
		Status:         "pending",
	}
	invoice3 := models.Invoice{
		AdminAccountID: 123,
		CompanyID:      456,
		InvoiceDate:    time.Date(2025, 1, 17, 0, 0, 0, 0, time.UTC),
		TotalAmount:    decimal.NewFromInt(3000),
		Status:         "overdue",
	}
	db.Create(&invoice1)
	db.Create(&invoice2)
	db.Create(&invoice3)

	month := 1
	stats, err := service.GetBillingStatistics(456, 2025, &month)
	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 3, stats.TotalInvoices)
	assert.Equal(t, 1, stats.PaidInvoices)
	assert.Equal(t, 1, stats.PendingInvoices)
	assert.Equal(t, 1, stats.OverdueInvoices)
}
