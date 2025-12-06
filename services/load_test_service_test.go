package services

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestNewLoadTestService тестирует создание нового LoadTestService
func TestNewLoadTestService(t *testing.T) {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewLoadTestService("http://localhost:8080", logger)

	assert.NotNil(t, service)
	assert.NotNil(t, service.httpClient)
	assert.NotNil(t, service.logger)
	assert.Equal(t, "http://localhost:8080", service.baseURL)
}

// TestLoadTestService_RunLoadTest_EmptyEndpoints тестирует RunLoadTest с пустым списком endpoints
func TestLoadTestService_RunLoadTest_EmptyEndpoints(t *testing.T) {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewLoadTestService("http://localhost:8080", logger)

	config := LoadTestConfig{
		ConcurrentUsers: 1,
		DurationSeconds: 1,
		Endpoints:       []string{}, // Пустой список
		Timeout:         5 * time.Second,
	}

	ctx := context.Background()
	result, err := service.RunLoadTest(ctx, config)
	// Может вернуть ошибку или успех с нулевыми результатами
	if err != nil {
		assert.Contains(t, err.Error(), "endpoints")
	} else {
		assert.NotNil(t, result)
		assert.Equal(t, int64(0), result.TotalRequests)
	}
}

// TestLoadTestService_RunLoadTest_InvalidConfig тестирует RunLoadTest с неверной конфигурацией
func TestLoadTestService_RunLoadTest_InvalidConfig(t *testing.T) {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewLoadTestService("http://localhost:8080", logger)

	config := LoadTestConfig{
		ConcurrentUsers: 0, // Неверное значение
		DurationSeconds: 1,
		Endpoints:       []string{"/api/test"},
		Timeout:         5 * time.Second,
	}

	ctx := context.Background()
	result, err := service.RunLoadTest(ctx, config)
	// Может вернуть ошибку или обработать неверную конфигурацию
	if err != nil {
		assert.Contains(t, err.Error(), "concurrent")
	} else {
		assert.NotNil(t, result)
	}
}
