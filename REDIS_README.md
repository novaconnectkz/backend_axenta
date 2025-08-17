# Redis Кэширование в Axenta CRM

## Описание

Система использует Redis для кэширования данных и повышения производительности. Redis настроен с поддержкой мультитенантности и предоставляет различные уровни кэширования.

## Настройка

### Переменные окружения

```bash
# Конфигурация Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
```

### Установка Redis

#### macOS (через Homebrew)

```bash
brew install redis
brew services start redis
```

#### Ubuntu/Debian

```bash
sudo apt update
sudo apt install redis-server
sudo systemctl start redis-server
sudo systemctl enable redis-server
```

#### CentOS/RHEL

```bash
sudo yum install redis
sudo systemctl start redis
sudo systemctl enable redis
```

## Функциональность

### Основные возможности

1. **Мультитенантное кэширование** - изоляция данных по компаниям
2. **Кэширование объектов** - объекты мониторинга, пользователи, роли
3. **Кэширование поиска** - результаты поиска и фильтрации
4. **Кэширование конфигураций** - настройки системы и шаблоны
5. **Rate limiting** - ограничение частоты запросов
6. **Управление сессиями** - хранение данных сессий

### Структура ключей

```
tenant:{tenant_id}:data:{type}          # Общие данные компании
tenant:{tenant_id}:user:{user_id}:{type} # Данные пользователя
tenant:{tenant_id}:object:{id}:{type}   # Данные объекта
tenant:{tenant_id}:search:{hash}        # Результаты поиска
tenant:{tenant_id}:config:{type}        # Конфигурация
session:{session_id}                    # Данные сессии
ratelimit:tenant:{id}:user:{id}:{action} # Rate limiting
```

## API

### Базовые операции

```go
import "backend_axenta/database"

// Сохранение в кэш
err := database.CacheSet("key", "value", 5*time.Minute)

// Получение из кэша
value, err := database.CacheGet("key")

// Удаление из кэша
err := database.CacheDel("key")

// Проверка существования
exists, err := database.CacheExists("key")
```

### JSON операции

```go
// Сохранение объекта
data := map[string]interface{}{"name": "test", "value": 123}
err := database.CacheSetJSON("key", data, 10*time.Minute)

// Получение объекта
var result map[string]interface{}
err := database.CacheGetJSON("key", &result)
```

### Мультитенантные операции

```go
// Генерация ключей для компании
key := database.GenerateCacheKey(tenantID, "objects", "list")
userKey := database.GenerateUserCacheKey(tenantID, userID, "permissions")
objectKey := database.GenerateObjectCacheKey(tenantID, objectID, "data")

// Кэширование данных компании
err := database.CacheTenantData(tenantID, "stats", statsData, 5*time.Minute)

// Получение данных компании
var stats StatsData
err := database.GetCachedTenantData(tenantID, "stats", &stats)

// Очистка кэша компании
err := database.ClearTenantCache(tenantID)
```

### Rate Limiting

```go
// Проверка лимита (100 запросов в час)
allowed, err := database.RateLimitCheck(tenantID, userID, "api_call", 100, time.Hour)
if !allowed {
    return errors.New("превышен лимит запросов")
}
```

### Управление сессиями

```go
// Сохранение сессии
sessionData := map[string]interface{}{
    "user_id": 123,
    "tenant_id": 1,
    "permissions": []string{"read", "write"},
}
err := database.SessionStore("session_123", sessionData, 24*time.Hour)

// Получение сессии
var session map[string]interface{}
err := database.SessionGet("session_123", &session)

// Удаление сессии
err := database.SessionDelete("session_123")
```

## Сервис кэширования

### Использование CacheService

```go
import "backend_axenta/services"

cacheService := services.NewCacheService()

// Кэширование объекта
err := cacheService.CacheObject(tenantID, &object)

// Получение объекта из кэша
cachedObject, err := cacheService.GetCachedObject(tenantID, objectID)

// Инвалидация кэша объекта
err := cacheService.InvalidateObjectCache(tenantID, objectID)

// Кэширование списка объектов
err := cacheService.CacheObjectList(tenantID, "active_objects", objects)

// Получение списка из кэша
objects, err := cacheService.GetCachedObjectList(tenantID, "active_objects")
```

### TTL (Time To Live) константы

```go
const (
    CacheTTLShort  = 5 * time.Minute   // Часто изменяемые данные
    CacheTTLMedium = 15 * time.Minute  // Умеренно изменяемые данные
    CacheTTLLong   = 1 * time.Hour     // Редко изменяемые данные
    CacheTTLStatic = 24 * time.Hour    // Статические данные
)
```

## Стратегии кэширования

### 1. Cache-Aside (Lazy Loading)

```go
// Пробуем получить из кэша
data, err := cacheService.GetCachedObject(tenantID, objectID)
if err != nil {
    // Если нет в кэше, получаем из БД
    data, err = db.GetObject(objectID)
    if err == nil {
        // Сохраняем в кэш
        cacheService.CacheObject(tenantID, data)
    }
}
```

### 2. Write-Through

```go
// При обновлении данных
err := db.UpdateObject(&object)
if err == nil {
    // Обновляем кэш
    cacheService.CacheObject(tenantID, &object)
}
```

### 3. Write-Behind (Write-Back)

```go
// Сначала обновляем кэш
cacheService.CacheObject(tenantID, &object)

// Асинхронно обновляем БД
go func() {
    db.UpdateObject(&object)
}()
```

## Мониторинг и статистика

### Получение статистики кэша

```go
stats, err := cacheService.GetCacheStats()
// Возвращает информацию о количестве ключей, использовании памяти и т.д.
```

### Прогрев кэша

```go
// Прогрев кэша для компании при старте
err := cacheService.WarmupCache(tenantID)
```

## Обработка ошибок

Redis настроен как опциональный компонент. Если Redis недоступен:

1. **Graceful degradation** - система продолжает работать без кэширования
2. **Логирование** - все ошибки Redis логируются как предупреждения
3. **Fallback** - при ошибках кэша данные получаются из основной БД

```go
// Пример обработки ошибок кэша
data, err := cacheService.GetCachedObject(tenantID, objectID)
if err != nil {
    // Fallback к БД
    data, err = database.DB.GetObject(objectID)
}
```

## Лучшие практики

### 1. Выбор TTL

- **Статические данные** (роли, разрешения): 24 часа
- **Конфигурации**: 1 час
- **Данные пользователей**: 15 минут
- **Списки и поиск**: 5 минут

### 2. Инвалидация кэша

```go
// При изменении объекта
err := db.UpdateObject(&object)
if err == nil {
    // Инвалидируем связанные кэши
    cacheService.InvalidateAllObjectCache(tenantID, objectID)
}
```

### 3. Размер ключей

- Используйте короткие, но понятные имена ключей
- Избегайте очень длинных ключей (>250 символов)

### 4. Мониторинг

- Следите за hit/miss ratio
- Мониторьте использование памяти
- Логируйте медленные операции

## Безопасность

### Изоляция данных

- Каждая компания имеет изолированное пространство ключей
- Используется префикс `tenant:{id}:` для всех ключей компании

### Очистка данных

```go
// Очистка всех данных компании при удалении
err := database.ClearTenantCache(tenantID)
```

## Производительность

### Оптимизация

1. **Пайплайны** для множественных операций
2. **Сжатие** для больших объектов (автоматически)
3. **Connection pooling** (настроено по умолчанию)

### Настройки подключения

```go
Redis = redis.NewClient(&redis.Options{
    Addr:         "localhost:6379",
    PoolSize:     10,              // Размер пула соединений
    MinIdleConns: 5,               // Минимум idle соединений
    DialTimeout:  5 * time.Second, // Таймаут подключения
    ReadTimeout:  3 * time.Second, // Таймаут чтения
    WriteTimeout: 3 * time.Second, // Таймаут записи
    IdleTimeout:  300 * time.Second, // Таймаут idle соединения
})
```

