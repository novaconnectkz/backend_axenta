package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
)

// CacheService предоставляет методы для кэширования
type CacheService struct {
	redis  *redis.Client
	logger *log.Logger
}

// NewCacheService создает новый экземпляр CacheService
func NewCacheService(redisClient *redis.Client, logger *log.Logger) *CacheService {
	return &CacheService{
		redis:  redisClient,
		logger: logger,
	}
}

// Get получает значение из кэша
func (cs *CacheService) Get(ctx context.Context, key string) (string, error) {
	if cs.redis == nil {
		return "", fmt.Errorf("Redis не подключен")
	}

	val, err := cs.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("ключ не найден")
	}
	return val, err
}

// Set сохраняет значение в кэш
func (cs *CacheService) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if cs.redis == nil {
		if cs.logger != nil {
			cs.logger.Printf("Redis не подключен, пропускаем кэширование для ключа: %s", key)
		}
		return nil // Не возвращаем ошибку, просто пропускаем кэширование
	}

	return cs.redis.Set(ctx, key, value, ttl).Err()
}

// Del удаляет значение из кэша
func (cs *CacheService) Del(ctx context.Context, key string) error {
	if cs.redis == nil {
		return nil
	}

	return cs.redis.Del(ctx, key).Err()
}

// Константы для TTL кэша
const (
	CacheTTLShort  = 5 * time.Minute  // Для часто изменяемых данных
	CacheTTLMedium = 15 * time.Minute // Для умеренно изменяемых данных
	CacheTTLLong   = 1 * time.Hour    // Для редко изменяемых данных
	CacheTTLStatic = 24 * time.Hour   // Для статических данных
)

// CacheObject кэширует объект
func (cs *CacheService) CacheObject(tenantID uint, object *models.Object) error {
	key := database.GenerateObjectCacheKey(tenantID, object.ID, "data")
	return database.CacheSetJSON(key, object, CacheTTLMedium)
}

// GetCachedObject получает объект из кэша
func (cs *CacheService) GetCachedObject(tenantID uint, objectID uint) (*models.Object, error) {
	key := database.GenerateObjectCacheKey(tenantID, objectID, "data")
	var object models.Object
	err := database.CacheGetJSON(key, &object)
	if err != nil {
		return nil, err
	}
	return &object, nil
}

// InvalidateObjectCache инвалидирует кэш объекта
func (cs *CacheService) InvalidateObjectCache(tenantID uint, objectID uint) error {
	key := database.GenerateObjectCacheKey(tenantID, objectID, "data")
	return database.CacheDel(key)
}

// CacheObjectList кэширует список объектов
func (cs *CacheService) CacheObjectList(tenantID uint, cacheKey string, objects []models.Object) error {
	key := database.GenerateCacheKey(tenantID, "objects", cacheKey)
	return database.CacheSetJSON(key, objects, CacheTTLShort)
}

// GetCachedObjectList получает список объектов из кэша
func (cs *CacheService) GetCachedObjectList(tenantID uint, cacheKey string) ([]models.Object, error) {
	key := database.GenerateCacheKey(tenantID, "objects", cacheKey)
	var objects []models.Object
	err := database.CacheGetJSON(key, &objects)
	if err != nil {
		return nil, err
	}
	return objects, nil
}

// CacheUser кэширует пользователя
func (cs *CacheService) CacheUser(tenantID uint, user *models.User) error {
	key := database.GenerateUserCacheKey(tenantID, user.ID, "data")
	return database.CacheSetJSON(key, user, CacheTTLMedium)
}

// GetCachedUser получает пользователя из кэша
func (cs *CacheService) GetCachedUser(tenantID uint, userID uint) (*models.User, error) {
	key := database.GenerateUserCacheKey(tenantID, userID, "data")
	var user models.User
	err := database.CacheGetJSON(key, &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// InvalidateUserCache инвалидирует кэш пользователя
func (cs *CacheService) InvalidateUserCache(tenantID uint, userID uint) error {
	key := database.GenerateUserCacheKey(tenantID, userID, "data")
	return database.CacheDel(key)
}

// CachePermissions кэширует права пользователя
func (cs *CacheService) CachePermissions(tenantID uint, userID uint, permissions []string) error {
	key := database.GenerateUserCacheKey(tenantID, userID, "permissions")
	return database.CacheSetJSON(key, permissions, CacheTTLLong)
}

// GetCachedPermissions получает права пользователя из кэша
func (cs *CacheService) GetCachedPermissions(tenantID uint, userID uint) ([]string, error) {
	key := database.GenerateUserCacheKey(tenantID, userID, "permissions")
	var permissions []string
	err := database.CacheGetJSON(key, &permissions)
	if err != nil {
		return nil, err
	}
	return permissions, nil
}

// InvalidatePermissionsCache инвалидирует кэш прав пользователя
func (cs *CacheService) InvalidatePermissionsCache(tenantID uint, userID uint) error {
	key := database.GenerateUserCacheKey(tenantID, userID, "permissions")
	return database.CacheDel(key)
}

// CacheStats кэширует статистику
func (cs *CacheService) CacheStats(tenantID uint, statsType string, data interface{}) error {
	key := database.GenerateCacheKey(tenantID, "stats", statsType)
	return database.CacheSetJSON(key, data, CacheTTLShort)
}

// GetCachedStats получает статистику из кэша
func (cs *CacheService) GetCachedStats(tenantID uint, statsType string, dest interface{}) error {
	key := database.GenerateCacheKey(tenantID, "stats", statsType)
	return database.CacheGetJSON(key, dest)
}

// CacheTemplate кэширует шаблон
func (cs *CacheService) CacheTemplate(tenantID uint, templateType string, templateID uint, template interface{}) error {
	key := fmt.Sprintf("tenant:%d:template:%s:%d", tenantID, templateType, templateID)
	return database.CacheSetJSON(key, template, CacheTTLLong)
}

// GetCachedTemplate получает шаблон из кэша
func (cs *CacheService) GetCachedTemplate(tenantID uint, templateType string, templateID uint, dest interface{}) error {
	key := fmt.Sprintf("tenant:%d:template:%s:%d", tenantID, templateType, templateID)
	return database.CacheGetJSON(key, dest)
}

// InvalidateTemplateCache инвалидирует кэш шаблона
func (cs *CacheService) InvalidateTemplateCache(tenantID uint, templateType string, templateID uint) error {
	key := fmt.Sprintf("tenant:%d:template:%s:%d", tenantID, templateType, templateID)
	return database.CacheDel(key)
}

// CacheSearchResults кэширует результаты поиска
func (cs *CacheService) CacheSearchResults(tenantID uint, searchHash string, results interface{}) error {
	key := database.GenerateCacheKey(tenantID, "search", searchHash)
	return database.CacheSetJSON(key, results, CacheTTLShort)
}

// GetCachedSearchResults получает результаты поиска из кэша
func (cs *CacheService) GetCachedSearchResults(tenantID uint, searchHash string, dest interface{}) error {
	key := database.GenerateCacheKey(tenantID, "search", searchHash)
	return database.CacheGetJSON(key, dest)
}

// CacheConfiguration кэширует конфигурацию системы
func (cs *CacheService) CacheConfiguration(tenantID uint, configType string, config interface{}) error {
	key := database.GenerateCacheKey(tenantID, "config", configType)
	return database.CacheSetJSON(key, config, CacheTTLStatic)
}

// GetCachedConfiguration получает конфигурацию из кэша
func (cs *CacheService) GetCachedConfiguration(tenantID uint, configType string, dest interface{}) error {
	key := database.GenerateCacheKey(tenantID, "config", configType)
	return database.CacheGetJSON(key, dest)
}

// InvalidateAllUserCache инвалидирует весь кэш пользователя
func (cs *CacheService) InvalidateAllUserCache(tenantID uint, userID uint) error {
	// Инвалидируем данные пользователя
	if err := cs.InvalidateUserCache(tenantID, userID); err != nil {
		return err
	}

	// Инвалидируем права пользователя
	if err := cs.InvalidatePermissionsCache(tenantID, userID); err != nil {
		return err
	}

	return nil
}

// InvalidateAllObjectCache инвалидирует весь кэш объекта
func (cs *CacheService) InvalidateAllObjectCache(tenantID uint, objectID uint) error {
	// Инвалидируем данные объекта
	if err := cs.InvalidateObjectCache(tenantID, objectID); err != nil {
		return err
	}

	// Инвалидируем списки объектов (они могут содержать этот объект)
	pattern := fmt.Sprintf("tenant:%d:objects:*", tenantID)
	return cs.invalidateByPattern(pattern)
}

// invalidateByPattern инвалидирует кэш по паттерну
func (cs *CacheService) invalidateByPattern(pattern string) error {
	redis := database.GetRedis()
	if redis == nil {
		return nil // Redis не подключен
	}

	keys, err := redis.Keys(database.Ctx, pattern).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return redis.Del(database.Ctx, keys...).Err()
	}

	return nil
}

// GetCacheStats возвращает статистику использования кэша
func (cs *CacheService) GetCacheStats() (map[string]interface{}, error) {
	redis := database.GetRedis()
	if redis == nil {
		return map[string]interface{}{
			"status": "disabled",
		}, nil
	}

	info, err := redis.Info(database.Ctx, "memory").Result()
	if err != nil {
		return nil, err
	}

	keyCount, err := redis.DBSize(database.Ctx).Result()
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"status":    "enabled",
		"key_count": keyCount,
		"memory":    info,
	}, nil
}

// WarmupCache прогревает кэш для компании
func (cs *CacheService) WarmupCache(tenantID uint) error {
	// TODO: Реализовать прогрев кэша для часто используемых данных
	// Например, загрузить популярные объекты, пользователей, шаблоны
	return nil
}
