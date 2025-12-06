package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupInvoiceSenderTestDB создает тестовую базу данных для invoice sender
func setupInvoiceSenderTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Invoice{},
		&models.Company{},
		&models.NotificationSettings{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// TestNewInvoiceSenderService тестирует создание нового сервиса
func TestNewInvoiceSenderService(t *testing.T) {
	db := setupInvoiceSenderTestDB(t)

	service := NewInvoiceSenderService(db)
	assert.NotNil(t, service)
	assert.NotNil(t, service.DB)
}

// TestInvoiceSenderService_SendInvoiceToClient_InvalidInvoice тестирует SendInvoiceToClient с неверным счетом
func TestInvoiceSenderService_SendInvoiceToClient_InvalidInvoice(t *testing.T) {
	db := setupInvoiceSenderTestDB(t)
	service := NewInvoiceSenderService(db)

	err := service.SendInvoiceToClient(nil, []string{"email"}, map[string]string{"email": "test@example.com"})
	assert.Error(t, err)
}

// TestInvoiceSenderService_SendInvoiceToClient_NoEmail тестирует SendInvoiceToClient без email
func TestInvoiceSenderService_SendInvoiceToClient_NoEmail(t *testing.T) {
	db := setupInvoiceSenderTestDB(t)
	service := NewInvoiceSenderService(db)

	// Создаем счет
	invoice := models.Invoice{
		ID:             1,
		AdminAccountID: 123,
		CompanyID:      456,
		TotalAmount:    decimal.NewFromInt(1000),
		Status:         "pending",
		Number:         "INV-001",
	}
	db.Create(&invoice)

	err := service.SendInvoiceToClient(&invoice, []string{"email"}, map[string]string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "email не указан")
}

// TestInvoiceSenderService_SendInvoiceToClient_NoSettings тестирует SendInvoiceToClient без настроек
func TestInvoiceSenderService_SendInvoiceToClient_NoSettings(t *testing.T) {
	db := setupInvoiceSenderTestDB(t)
	service := NewInvoiceSenderService(db)

	// Создаем счет
	invoice := models.Invoice{
		ID:             1,
		AdminAccountID: 123,
		CompanyID:      456,
		TotalAmount:    decimal.NewFromInt(1000),
		Status:         "pending",
		Number:         "INV-001",
	}
	db.Create(&invoice)

	err := service.SendInvoiceToClient(&invoice, []string{"email"}, map[string]string{"email": "test@example.com"})
	// Ожидаем ошибку, так как настройки не найдены
	assert.Error(t, err)
}

// TestInvoiceSenderService_SendInvoiceToClient_EmailDisabled тестирует SendInvoiceToClient когда email отключен
func TestInvoiceSenderService_SendInvoiceToClient_EmailDisabled(t *testing.T) {
	db := setupInvoiceSenderTestDB(t)
	service := NewInvoiceSenderService(db)

	// Создаем счет
	invoice := models.Invoice{
		ID:             1,
		AdminAccountID: 123,
		CompanyID:      456,
		TotalAmount:    decimal.NewFromInt(1000),
		Status:         "pending",
		Number:         "INV-001",
	}
	db.Create(&invoice)

	// Создаем настройки с отключенным email
	settings := models.NotificationSettings{
		CompanyID:    456,
		EmailEnabled: false,
	}
	db.Create(&settings)

	err := service.SendInvoiceToClient(&invoice, []string{"email"}, map[string]string{"email": "test@example.com"})
	// Ожидаем ошибку, так как email отключен
	assert.Error(t, err)
}

// TestInvoiceSenderService_SendInvoiceToClient_InvalidChannel тестирует SendInvoiceToClient с неверным каналом
func TestInvoiceSenderService_SendInvoiceToClient_InvalidChannel(t *testing.T) {
	db := setupInvoiceSenderTestDB(t)
	service := NewInvoiceSenderService(db)

	// Создаем счет
	invoice := models.Invoice{
		ID:             1,
		AdminAccountID: 123,
		CompanyID:      456,
		TotalAmount:    decimal.NewFromInt(1000),
		Status:         "pending",
		Number:         "INV-001",
	}
	db.Create(&invoice)

	err := service.SendInvoiceToClient(&invoice, []string{"invalid_channel"}, map[string]string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "неподдерживаемый канал")
}
