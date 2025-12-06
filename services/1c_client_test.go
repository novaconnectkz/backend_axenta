package services

import (
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewOneCClient тестирует создание нового OneCClient
func TestNewOneCClient(t *testing.T) {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	client := NewOneCClient(logger)

	assert.NotNil(t, client)
	assert.NotNil(t, client.HTTPClient)
	assert.NotNil(t, client.Logger)
}

// TestOneCCredentials_Validation тестирует валидацию OneCCredentials
func TestOneCCredentials_Validation(t *testing.T) {
	creds := OneCCredentials{
		BaseURL:    "https://1c.example.com",
		Username:   "user",
		Password:   "pass",
		Database:   "database",
		APIVersion: "v1",
	}

	assert.NotEmpty(t, creds.BaseURL)
	assert.NotEmpty(t, creds.Username)
	assert.NotEmpty(t, creds.Password)
}

// TestOneCCounterparty_Structure тестирует структуру OneCCounterparty
func TestOneCCounterparty_Structure(t *testing.T) {
	counterparty := OneCCounterparty{
		ID:          "test-id",
		Code:        "TEST001",
		Description: "Test Counterparty",
		INN:         "1234567890",
		IsActive:    true,
	}

	assert.Equal(t, "test-id", counterparty.ID)
	assert.Equal(t, "TEST001", counterparty.Code)
	assert.True(t, counterparty.IsActive)
}

// TestOneCPayment_Structure тестирует структуру OneCPayment
func TestOneCPayment_Structure(t *testing.T) {
	payment := OneCPayment{
		ID:     "payment-id",
		Number: "PAY-001",
		Posted: true,
		Amount: 1000.0,
	}

	assert.Equal(t, "payment-id", payment.ID)
	assert.Equal(t, "PAY-001", payment.Number)
	assert.True(t, payment.Posted)
	assert.Equal(t, 1000.0, payment.Amount)
}

// TestOneCContract_Structure тестирует структуру OneCContract
func TestOneCContract_Structure(t *testing.T) {
	contract := OneCContract{
		ID:          "contract-id",
		Code:        "CONT-001",
		Description: "Test Contract",
		IsActive:    true,
	}

	assert.Equal(t, "contract-id", contract.ID)
	assert.Equal(t, "CONT-001", contract.Code)
	assert.True(t, contract.IsActive)
}

// TestOneCPaymentRegistry_Structure тестирует структуру OneCPaymentRegistry
func TestOneCPaymentRegistry_Structure(t *testing.T) {
	registry := OneCPaymentRegistry{
		RegistryNumber: "REG-001",
		TotalAmount:    5000.0,
		PaymentsCount:  5,
		Status:         "pending",
	}

	assert.Equal(t, "REG-001", registry.RegistryNumber)
	assert.Equal(t, 5000.0, registry.TotalAmount)
	assert.Equal(t, 5, registry.PaymentsCount)
}
