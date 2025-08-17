package database

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

var Redis *redis.Client
var RedisClient *redis.Client // Экспортируемый клиент
var Ctx = context.Background()

// InitRedis инициализирует подключение к Redis
func InitRedis() error {
	// Получаем настройки Redis из переменных окружения
	host := getEnv("REDIS_HOST", "localhost")
	port := getEnv("REDIS_PORT", "6379")
	password := getEnv("REDIS_PASSWORD", "")
	dbStr := getEnv("REDIS_DB", "0")

	// Конвертируем номер БД в int
	db, err := strconv.Atoi(dbStr)
	if err != nil {
		db = 0
	}

	// Создаем клиент Redis
	Redis = redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", host, port),
		Password:     password,
		DB:           db,
		PoolSize:     10,
		MinIdleConns: 5,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
		IdleTimeout:  300 * time.Second,
	})

	// Устанавливаем экспортируемый клиент
	RedisClient = Redis

	// Проверяем подключение
	if err := Redis.Ping(Ctx).Err(); err != nil {
		return fmt.Errorf("не удалось подключиться к Redis: %w", err)
	}

	log.Println("✅ Успешно подключено к Redis")
	return nil
}

// GetRedis возвращает экземпляр Redis клиента
func GetRedis() *redis.Client {
	return Redis
}

// CacheSet сохраняет значение в кэш с TTL
func CacheSet(key string, value interface{}, ttl time.Duration) error {
	return Redis.Set(Ctx, key, value, ttl).Err()
}

// CacheGet получает значение из кэша
func CacheGet(key string) (string, error) {
	return Redis.Get(Ctx, key).Result()
}

// CacheDel удаляет значение из кэша
func CacheDel(key string) error {
	return Redis.Del(Ctx, key).Err()
}

// CacheExists проверяет существование ключа в кэше
func CacheExists(key string) (bool, error) {
	count, err := Redis.Exists(Ctx, key).Result()
	return count > 0, err
}

// CacheSetJSON сохраняет JSON объект в кэш
func CacheSetJSON(key string, value interface{}, ttl time.Duration) error {
	jsonData, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("ошибка сериализации JSON: %w", err)
	}
	return CacheSet(key, string(jsonData), ttl)
}

// CacheGetJSON получает JSON объект из кэша
func CacheGetJSON(key string, dest interface{}) error {
	jsonData, err := CacheGet(key)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(jsonData), dest); err != nil {
		return fmt.Errorf("ошибка десериализации JSON: %w", err)
	}

	return nil
}

// CacheSetHash сохраняет хэш в кэш
func CacheSetHash(key string, fields map[string]interface{}, ttl time.Duration) error {
	pipe := Redis.Pipeline()
	pipe.HSet(Ctx, key, fields)
	if ttl > 0 {
		pipe.Expire(Ctx, key, ttl)
	}
	_, err := pipe.Exec(Ctx)
	return err
}

// CacheGetHash получает хэш из кэша
func CacheGetHash(key string) (map[string]string, error) {
	return Redis.HGetAll(Ctx, key).Result()
}

// CacheIncr увеличивает счетчик
func CacheIncr(key string) (int64, error) {
	return Redis.Incr(Ctx, key).Result()
}

// CacheExpire устанавливает TTL для ключа
func CacheExpire(key string, ttl time.Duration) error {
	return Redis.Expire(Ctx, key, ttl).Err()
}

// CacheFlushDB очищает текущую БД Redis (для тестов)
func CacheFlushDB() error {
	return Redis.FlushDB(Ctx).Err()
}

// GenerateCacheKey генерирует ключ кэша для мультитенантности
func GenerateCacheKey(tenantID uint, prefix string, suffix string) string {
	return fmt.Sprintf("tenant:%d:%s:%s", tenantID, prefix, suffix)
}

// GenerateUserCacheKey генерирует ключ кэша для пользователя
func GenerateUserCacheKey(tenantID uint, userID uint, suffix string) string {
	return fmt.Sprintf("tenant:%d:user:%d:%s", tenantID, userID, suffix)
}

// GenerateObjectCacheKey генерирует ключ кэша для объекта
func GenerateObjectCacheKey(tenantID uint, objectID uint, suffix string) string {
	return fmt.Sprintf("tenant:%d:object:%d:%s", tenantID, objectID, suffix)
}

// CacheTenantData кэширует данные компании
func CacheTenantData(tenantID uint, dataType string, data interface{}, ttl time.Duration) error {
	key := GenerateCacheKey(tenantID, "data", dataType)
	return CacheSetJSON(key, data, ttl)
}

// GetCachedTenantData получает кэшированные данные компании
func GetCachedTenantData(tenantID uint, dataType string, dest interface{}) error {
	key := GenerateCacheKey(tenantID, "data", dataType)
	return CacheGetJSON(key, dest)
}

// ClearTenantCache очищает весь кэш компании
func ClearTenantCache(tenantID uint) error {
	pattern := fmt.Sprintf("tenant:%d:*", tenantID)
	keys, err := Redis.Keys(Ctx, pattern).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return Redis.Del(Ctx, keys...).Err()
	}

	return nil
}

// RateLimitCheck проверяет rate limit для пользователя
func RateLimitCheck(tenantID uint, userID uint, action string, limit int64, window time.Duration) (bool, error) {
	key := fmt.Sprintf("ratelimit:tenant:%d:user:%d:%s", tenantID, userID, action)

	pipe := Redis.Pipeline()
	incr := pipe.Incr(Ctx, key)
	pipe.Expire(Ctx, key, window)
	_, err := pipe.Exec(Ctx)

	if err != nil {
		return false, err
	}

	count, err := incr.Result()
	if err != nil {
		return false, err
	}

	return count <= limit, nil
}

// SessionStore сохраняет данные сессии
func SessionStore(sessionID string, data interface{}, ttl time.Duration) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return CacheSetJSON(key, data, ttl)
}

// SessionGet получает данные сессии
func SessionGet(sessionID string, dest interface{}) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return CacheGetJSON(key, dest)
}

// SessionDelete удаляет сессию
func SessionDelete(sessionID string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return CacheDel(key)
}
