package middleware

import (
	"backend_axenta/database"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	redis "github.com/go-redis/redis/v8"
)

// RateLimitConfig конфигурация rate limiting
type RateLimitConfig struct {
	Requests       int                       // Количество запросов
	Window         time.Duration             // Временное окно
	SkipSuccessful bool                      // Пропускать успешные запросы
	KeyGenerator   func(*gin.Context) string // Генератор ключей
}

// DefaultKeyGenerator генерирует ключ на основе IP адреса
func DefaultKeyGenerator(c *gin.Context) string {
	return c.ClientIP()
}

// UserKeyGenerator генерирует ключ на основе пользователя
func UserKeyGenerator(c *gin.Context) string {
	userID := c.GetString("user_id")
	if userID == "" {
		return c.ClientIP()
	}
	return "user:" + userID
}

// APIKeyGenerator генерирует ключ на основе API ключа
func APIKeyGenerator(c *gin.Context) string {
	apiKey := c.GetHeader("X-API-Key")
	if apiKey == "" {
		return c.ClientIP()
	}
	return "api:" + apiKey
}

// RateLimit создает middleware для ограничения частоты запросов
func RateLimit(config RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		redisClient := database.GetRedis()
		if redisClient == nil {
			// Если Redis недоступен, пропускаем rate limiting
			c.Next()
			return
		}

		key := "rate_limit:" + config.KeyGenerator(c)

		// Получаем текущее количество запросов
		current, err := redisClient.Get(database.Ctx, key).Int()
		if err != nil && err != redis.Nil {
			// В случае ошибки Redis пропускаем запрос
			c.Next()
			return
		}

		// Проверяем превышение лимита
		if current >= config.Requests {
			c.Header("X-RateLimit-Limit", strconv.Itoa(config.Requests))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(config.Window).Unix(), 10))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded",
				"message": fmt.Sprintf("Too many requests. Limit: %d requests per %v",
					config.Requests, config.Window),
				"retry_after": config.Window.Seconds(),
			})
			c.Abort()
			return
		}

		// Увеличиваем счетчик
		pipe := redisClient.Pipeline()
		pipe.Incr(database.Ctx, key)
		if current == 0 {
			// Устанавливаем TTL только для первого запроса
			pipe.Expire(database.Ctx, key, config.Window)
		}
		_, err = pipe.Exec(database.Ctx)
		if err != nil {
			// В случае ошибки пропускаем запрос
			c.Next()
			return
		}

		// Устанавливаем заголовки rate limit
		remaining := config.Requests - current - 1
		if remaining < 0 {
			remaining = 0
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(config.Requests))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(config.Window).Unix(), 10))

		c.Next()

		// Если настроено пропускать успешные запросы и запрос успешен
		if config.SkipSuccessful && c.Writer.Status() < 400 {
			redisClient.Decr(database.Ctx, key)
		}
	}
}

// Предустановленные конфигурации rate limiting

// StrictRateLimit строгое ограничение для критических endpoints
func StrictRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests:     10,
		Window:       time.Minute,
		KeyGenerator: UserKeyGenerator,
	})
}

// ModerateRateLimit умеренное ограничение для обычных API
func ModerateRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests:     100,
		Window:       time.Minute,
		KeyGenerator: UserKeyGenerator,
	})
}

// LenientRateLimit мягкое ограничение для публичных endpoints
func LenientRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests:     1000,
		Window:       time.Minute,
		KeyGenerator: DefaultKeyGenerator,
	})
}

// AuthRateLimit ограничение для авторизации
func AuthRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests:     5,
		Window:       time.Minute,
		KeyGenerator: DefaultKeyGenerator,
	})
}

// APIRateLimit ограничение для API ключей
func APIRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests:       10000,
		Window:         time.Hour,
		KeyGenerator:   APIKeyGenerator,
		SkipSuccessful: true,
	})
}

// BurstRateLimit ограничение для предотвращения burst атак
func BurstRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests:     20,
		Window:       time.Second * 10,
		KeyGenerator: DefaultKeyGenerator,
	})
}

// RateLimitInfo информация о rate limiting
type RateLimitInfo struct {
	Key       string `json:"key"`
	Current   int    `json:"current"`
	Limit     int    `json:"limit"`
	Remaining int    `json:"remaining"`
	ResetTime int64  `json:"reset_time"`
}

// GetRateLimitInfo получает информацию о rate limiting для ключа
func GetRateLimitInfo(keyGenerator func(*gin.Context) string, config RateLimitConfig, c *gin.Context) (*RateLimitInfo, error) {
	redisClient := database.GetRedis()
	if redisClient == nil {
		return nil, fmt.Errorf("Redis not available")
	}

	key := "rate_limit:" + keyGenerator(c)

	current, err := redisClient.Get(database.Ctx, key).Int()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	if err == redis.Nil {
		current = 0
	}

	remaining := config.Requests - current
	if remaining < 0 {
		remaining = 0
	}

	ttl, err := redisClient.TTL(database.Ctx, key).Result()
	if err != nil {
		ttl = 0
	}

	resetTime := time.Now().Add(ttl).Unix()

	return &RateLimitInfo{
		Key:       key,
		Current:   current,
		Limit:     config.Requests,
		Remaining: remaining,
		ResetTime: resetTime,
	}, nil
}

// ClearRateLimit очищает rate limit для ключа
func ClearRateLimit(keyGenerator func(*gin.Context) string, c *gin.Context) error {
	redisClient := database.GetRedis()
	if redisClient == nil {
		return fmt.Errorf("Redis not available")
	}

	key := "rate_limit:" + keyGenerator(c)
	return redisClient.Del(database.Ctx, key).Err()
}
