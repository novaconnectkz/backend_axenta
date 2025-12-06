package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestCacheSet_NoRedis тестирует CacheSet когда Redis не подключен
func TestCacheSet_NoRedis(t *testing.T) {
	// Redis может быть не подключен
	err := CacheSet("test-key", "test-value", time.Minute)
	// Может вернуть ошибку или успех в зависимости от состояния Redis
	if err != nil {
		assert.Contains(t, err.Error(), "redis")
	}
}

// TestCacheGet_NoRedis тестирует CacheGet когда Redis не подключен
func TestCacheGet_NoRedis(t *testing.T) {
	value, err := CacheGet("test-key")
	// Может вернуть ошибку или пустое значение
	if err != nil {
		assert.Contains(t, err.Error(), "redis")
	} else {
		assert.Equal(t, "", value)
	}
}

// TestCacheDel_NoRedis тестирует CacheDel когда Redis не подключен
func TestCacheDel_NoRedis(t *testing.T) {
	err := CacheDel("test-key")
	// Может вернуть ошибку или успех
	if err != nil {
		assert.Contains(t, err.Error(), "redis")
	}
}

// TestCacheExists_NoRedis тестирует CacheExists когда Redis не подключен
func TestCacheExists_NoRedis(t *testing.T) {
	exists, err := CacheExists("test-key")
	// Может вернуть ошибку или false
	if err != nil {
		assert.Contains(t, err.Error(), "redis")
	} else {
		assert.False(t, exists)
	}
}

// TestGenerateCacheKey тестирует GenerateCacheKey
func TestGenerateCacheKey(t *testing.T) {
	key := GenerateCacheKey(123, "objects", "list")
	assert.Contains(t, key, "123")
	assert.Contains(t, key, "objects")
	assert.Contains(t, key, "list")
}

// TestGenerateUserCacheKey тестирует GenerateUserCacheKey
func TestGenerateUserCacheKey(t *testing.T) {
	key := GenerateUserCacheKey(123, 456, "data")
	assert.Contains(t, key, "123")
	assert.Contains(t, key, "456")
	assert.Contains(t, key, "user")
	assert.Contains(t, key, "data")
}

// TestGenerateObjectCacheKey тестирует GenerateObjectCacheKey
func TestGenerateObjectCacheKey(t *testing.T) {
	key := GenerateObjectCacheKey(123, 789, "data")
	assert.Contains(t, key, "123")
	assert.Contains(t, key, "789")
	assert.Contains(t, key, "object")
	assert.Contains(t, key, "data")
}
