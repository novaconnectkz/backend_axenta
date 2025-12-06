package services

import (
	"io"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewAxetnaClient тестирует NewAxetnaClient
func TestNewAxetnaClient(t *testing.T) {
	baseURL := "https://axenta.cloud"
	logger := log.New(io.Discard, "", 0)

	client := NewAxetnaClient(baseURL, logger)

	assert.NotNil(t, client)
	assert.Equal(t, baseURL, client.BaseURL)
	assert.NotNil(t, client.HTTPClient)
	assert.NotNil(t, client.Logger)
}

// TestNewAxetnaClient_NilLogger тестирует NewAxetnaClient с nil logger
func TestNewAxetnaClient_NilLogger(t *testing.T) {
	baseURL := "https://axenta.cloud"

	client := NewAxetnaClient(baseURL, nil)

	assert.NotNil(t, client)
	assert.Equal(t, baseURL, client.BaseURL)
	assert.NotNil(t, client.HTTPClient)
	assert.NotNil(t, client.Logger) // Должен быть создан пустой логгер
}

// TestGetDefaultRetryConfig тестирует GetDefaultRetryConfig
func TestGetDefaultRetryConfig(t *testing.T) {
	config := GetDefaultRetryConfig()

	assert.Greater(t, config.MaxRetries, 0)
	assert.Greater(t, config.InitialDelay, 0)
	assert.Greater(t, config.MaxDelay, 0)
	assert.Greater(t, config.BackoffFactor, 0.0)
	assert.NotEmpty(t, config.RetryableErrors)
}

// TestGetDefaultRetryConfig_Values тестирует значения GetDefaultRetryConfig
func TestGetDefaultRetryConfig_Values(t *testing.T) {
	config := GetDefaultRetryConfig()

	// Проверяем разумные значения
	assert.GreaterOrEqual(t, config.MaxRetries, 1)
	assert.LessOrEqual(t, config.MaxRetries, 10)
	assert.GreaterOrEqual(t, config.InitialDelay, 100*1000000) // Минимум 100ms в наносекундах
	assert.GreaterOrEqual(t, config.MaxDelay, config.InitialDelay)
	assert.GreaterOrEqual(t, config.BackoffFactor, 1.0)
	assert.LessOrEqual(t, config.BackoffFactor, 3.0)
}
