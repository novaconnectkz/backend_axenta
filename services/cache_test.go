package services

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestNewCacheService тестирует создание нового CacheService
func TestNewCacheService(t *testing.T) {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewCacheService(nil, logger)

	assert.NotNil(t, service)
	assert.Nil(t, service.redis) // Redis может быть nil
	assert.NotNil(t, service.logger)
}

// TestCacheService_Get_NoRedis тестирует Get когда Redis не подключен
func TestCacheService_Get_NoRedis(t *testing.T) {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewCacheService(nil, logger)

	ctx := context.Background()
	value, err := service.Get(ctx, "test-key")
	assert.Error(t, err)
	assert.Equal(t, "", value)
	assert.Contains(t, err.Error(), "не подключен")
}

// TestCacheService_Set_NoRedis тестирует Set когда Redis не подключен
func TestCacheService_Set_NoRedis(t *testing.T) {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewCacheService(nil, logger)

	ctx := context.Background()
	// Не должно возвращать ошибку, просто пропускает кэширование
	err := service.Set(ctx, "test-key", "test-value", time.Minute)
	assert.NoError(t, err)
}

// TestCacheService_Del_NoRedis тестирует Del когда Redis не подключен
func TestCacheService_Del_NoRedis(t *testing.T) {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewCacheService(nil, logger)

	ctx := context.Background()
	// Не должно возвращать ошибку
	err := service.Del(ctx, "test-key")
	assert.NoError(t, err)
}

// TestNewPerformanceCacheService тестирует создание нового PerformanceCacheService
func TestNewPerformanceCacheService(t *testing.T) {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewPerformanceCacheService(nil, logger)

	assert.NotNil(t, service)
	assert.Nil(t, service.redis) // Redis может быть nil
	assert.NotNil(t, service.logger)
}

// TestPerformanceCacheService_GetMetrics_NoRedis тестирует GetCacheMetrics когда Redis не подключен
func TestPerformanceCacheService_GetMetrics_NoRedis(t *testing.T) {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewPerformanceCacheService(nil, logger)

	metrics, err := service.GetCacheMetrics()
	// Может вернуть ошибку или пустые метрики
	if err != nil {
		assert.Contains(t, err.Error(), "не подключен")
	} else {
		assert.NotNil(t, metrics)
	}
}

// TestCacheTTLConstants тестирует константы TTL
func TestCacheTTLConstants(t *testing.T) {
	assert.Equal(t, 1*time.Minute, CacheTTLVeryShort)
	assert.Equal(t, 5*time.Minute, CacheTTLShort)
	assert.Equal(t, 15*time.Minute, CacheTTLMedium)
	assert.Equal(t, 1*time.Hour, CacheTTLLong)
	assert.Equal(t, 24*time.Hour, CacheTTLStatic)
	assert.Equal(t, 30*time.Minute, CacheTTLAggregation)
	assert.Equal(t, 10*time.Minute, CacheTTLFilteredLists)
}
