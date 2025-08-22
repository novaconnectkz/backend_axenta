# Система производительности и безопасности Axenta CRM

## Обзор

Система производительности и безопасности обеспечивает мониторинг, оптимизацию и защиту веб-приложения Axenta CRM. Включает в себя кэширование, индексы БД, rate limiting, аудит логи и нагрузочное тестирование.

## Компоненты системы

### 1. Кэширование (Redis)

#### PerformanceCacheService

- **Расположение**: `services/cache.go`
- **Функции**:
  - Кэширование объектов, пользователей, шаблонов
  - Метрики производительности (hit rate, miss count)
  - Прогрев кэша для часто используемых данных
  - Автоматическая инвалидация при изменениях

#### Конфигурация TTL

```go
const (
    CacheTTLShort  = 5 * time.Minute   // Часто изменяемые данные
    CacheTTLMedium = 15 * time.Minute  // Умеренно изменяемые данные
    CacheTTLLong   = 1 * time.Hour     // Редко изменяемые данные
    CacheTTLStatic = 24 * time.Hour    // Статические данные
)
```

#### API Endpoints

- `GET /api/performance/cache/metrics` - Метрики кэша
- `POST /api/performance/cache/warmup` - Прогрев кэша
- `DELETE /api/performance/cache/clear` - Очистка кэша

### 2. Индексы базы данных

#### Система индексов

- **Расположение**: `database/indexes.go`
- **Типы индексов**:
  - B-tree для обычных запросов
  - GIN для полнотекстового поиска
  - Составные индексы для сложных запросов
  - Уникальные индексы для ограничений

#### Основные индексы

```sql
-- Объекты
CREATE INDEX idx_objects_tenant_status ON objects (tenant_id, status);
CREATE INDEX idx_objects_imei ON objects (imei);
CREATE INDEX idx_objects_fulltext ON objects USING GIN (to_tsvector('russian', name || ' ' || description));

-- Пользователи
CREATE UNIQUE INDEX idx_users_email ON users (email);
CREATE INDEX idx_users_tenant_active ON users (tenant_id, is_active);

-- Установки
CREATE INDEX idx_installations_tenant_date ON installations (tenant_id, scheduled_date);
```

#### API Endpoints

- `GET /api/performance/database/indexes` - Информация об индексах
- `POST /api/performance/database/indexes/create` - Создание индексов
- `GET /api/performance/database/indexes/usage` - Статистика использования

### 3. Rate Limiting

#### Middleware

- **Расположение**: `middleware/rate_limiting.go`
- **Функции**:
  - Ограничение частоты запросов
  - Настраиваемые лимиты по пользователям/IP
  - Sliding window алгоритм
  - Автоматические заголовки X-RateLimit-\*

#### Предустановленные конфигурации

```go
// Строгое ограничение для критических endpoints
StrictRateLimit(): 10 запросов/минуту

// Умеренное ограничение для обычных API
ModerateRateLimit(): 100 запросов/минуту

// Ограничение для авторизации
AuthRateLimit(): 5 запросов/минуту
```

#### API Endpoints

- `GET /api/performance/rate-limit/info` - Информация о лимитах
- `DELETE /api/performance/rate-limit/clear` - Сброс лимитов

### 4. Аудит логи

#### AuditService

- **Расположение**: `services/audit_service.go`
- **Функции**:
  - Логирование критических операций
  - Статистика и аналитика
  - Алерты безопасности
  - Экспорт данных

#### Типы действий

```go
const (
    ActionUserLogin    = "user.login"
    ActionObjectCreate = "object.create"
    ActionContractUpdate = "contract.update"
    ActionSystemConfig = "system.config"
    ActionSecurityBreach = "security.breach"
)
```

#### Структура лога

```go
type AuditLog struct {
    TenantID   uint      `json:"tenant_id"`
    UserID     *uint     `json:"user_id"`
    Action     string    `json:"action"`
    Resource   string    `json:"resource"`
    IPAddress  string    `json:"ip_address"`
    Success    bool      `json:"success"`
    CreatedAt  time.Time `json:"created_at"`
}
```

#### API Endpoints

- `GET /api/performance/audit/logs` - Получение логов
- `GET /api/performance/audit/stats` - Статистика
- `GET /api/performance/audit/security-alerts` - Алерты безопасности
- `POST /api/performance/audit/export` - Экспорт логов

### 5. Нагрузочное тестирование

#### LoadTestService

- **Расположение**: `services/load_test_service.go`
- **Функции**:
  - Симуляция нагрузки на API
  - Метрики производительности
  - Анализ времени ответа
  - Детальные отчеты

#### Конфигурация теста

```go
type LoadTestConfig struct {
    ConcurrentUsers  int           // Одновременные пользователи
    DurationSeconds  int           // Длительность теста
    RampUpSeconds    int           // Время разгона
    Endpoints        []string      // Тестируемые endpoints
    ThinkTimeMs      int           // Пауза между запросами
}
```

#### Предустановленные сценарии

- **Light**: 10 пользователей, 60 секунд
- **Moderate**: 50 пользователей, 300 секунд
- **Heavy**: 100 пользователей, 600 секунд
- **Stress**: 200 пользователей, 300 секунд

#### API Endpoints

- `GET /api/performance/load-test/configs` - Конфигурации тестов
- `POST /api/performance/load-test/run` - Запуск теста
- `GET /api/performance/load-test/results/:id` - Результаты теста

## Frontend интерфейс

### Главная страница Performance.vue

- **Расположение**: `src/views/Performance.vue`
- **Вкладки**:
  1. **Обзор** - Ключевые метрики и системное здоровье
  2. **Кэширование** - Управление Redis кэшем
  3. **База данных** - Мониторинг БД и индексов
  4. **Безопасность** - Алерты и rate limiting
  5. **Аудит логи** - Просмотр и анализ логов

### Компоненты

- `PerformanceOverview.vue` - Обзор системы
- `CacheManagement.vue` - Управление кэшем
- `MetricCard.vue` - Карточка метрики
- `DatabasePerformance.vue` - Мониторинг БД
- `SecurityMonitoring.vue` - Мониторинг безопасности
- `AuditLogs.vue` - Аудит логи

### Сервисы

- `performanceService.ts` - API клиент
- Типы в `types/performance.ts`

## Использование

### Инициализация в main.go

```go
// Регистрируем API производительности
performanceAPI := api.NewPerformanceAPI()
performanceAPI.RegisterRoutes(v1)

// Применяем rate limiting middleware
router.Use(middleware.ModerateRateLimit())
```

### Использование кэша

```go
// Кэширование объекта
cacheService.CacheObject(tenantID, object)

// Получение из кэша
cachedObject, err := cacheService.GetCachedObject(tenantID, objectID)

// Прогрев кэша
cacheService.WarmupCache(tenantID)
```

### Аудит логирование

```go
// Логирование успешного действия
auditService.LogSuccess(services.AuditContext{
    TenantID:  tenantID,
    UserID:    &userID,
    Action:    services.ActionObjectCreate,
    Resource:  "object",
    IPAddress: c.ClientIP(),
    NewValues: object,
})
```

### Создание индексов

```go
// Создание всех индексов производительности
database.CreatePerformanceIndexes(db)

// Анализ использования индексов
usage, err := database.AnalyzeIndexUsage(db)
```

### Нагрузочное тестирование

```go
// Конфигурация теста
config := services.LoadTestConfig{
    ConcurrentUsers: 50,
    DurationSeconds: 300,
    Endpoints: []string{"/api/objects", "/api/users"},
}

// Запуск теста
result, err := loadTestService.RunLoadTest(ctx, config)
```

## Мониторинг и алерты

### Ключевые метрики

- **Cache Hit Rate** - Процент попаданий в кэш
- **Response Time** - Время ответа API
- **Error Rate** - Процент ошибок
- **Active Connections** - Активные подключения к БД
- **Memory Usage** - Использование памяти Redis

### Алерты безопасности

- Множественные неудачные попытки входа
- Подозрительная активность пользователей
- Превышение rate limits
- Ошибки аутентификации

### Автоматическая очистка

- Старые аудит логи (настраиваемый период)
- Неиспользуемые ключи кэша
- Временные файлы нагрузочных тестов

## Настройки производительности

### Переменные окружения

```env
# Redis
REDIS_URL=redis://localhost:6379
REDIS_PASSWORD=
REDIS_DB=0

# Rate Limiting
RATE_LIMIT_ENABLED=true
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_WINDOW=60s

# Аудит
AUDIT_ENABLED=true
AUDIT_RETENTION_DAYS=90

# Кэш
CACHE_ENABLED=true
CACHE_DEFAULT_TTL=15m
```

### Рекомендации по оптимизации

1. **Кэширование**: Используйте подходящие TTL для разных типов данных
2. **Индексы**: Регулярно анализируйте использование индексов
3. **Rate Limiting**: Настройте лимиты в зависимости от нагрузки
4. **Мониторинг**: Следите за ключевыми метриками
5. **Тестирование**: Проводите регулярные нагрузочные тесты

## Интеграция с существующими системами

### Middleware интеграция

- Автоматическое применение rate limiting
- Логирование всех критических операций
- Кэширование результатов API запросов

### Database интеграция

- Автоматическое создание индексов при миграциях
- Мониторинг производительности запросов
- Оптимизация схемы БД

### Frontend интеграция

- Демо интерфейс с реальными данными
- Интеграция с Apple Design System
- Responsive дизайн для всех устройств

## Безопасность

### Защищенные endpoints

- Все API производительности требуют аутентификации
- Проверка прав доступа администратора
- Валидация входных данных

### Конфиденциальность данных

- Хэширование чувствительных данных в логах
- Шифрование паролей интеграций
- Безопасное хранение метрик

### Аудит безопасности

- Логирование всех административных действий
- Мониторинг подозрительной активности
- Автоматические алерты при нарушениях

## Поддержка и разработка

### Структура проекта

```
backend/
├── services/
│   ├── cache.go                    # Кэширование
│   ├── audit_service.go           # Аудит логи
│   └── load_test_service.go       # Нагрузочные тесты
├── middleware/
│   └── rate_limiting.go           # Rate limiting
├── database/
│   └── indexes.go                 # Индексы БД
└── api/
    └── performance.go             # API endpoints

frontend/
├── views/
│   └── Performance.vue            # Главная страница
├── components/Performance/
│   ├── PerformanceOverview.vue    # Обзор
│   ├── CacheManagement.vue       # Управление кэшем
│   └── MetricCard.vue            # Карточка метрики
├── services/
│   └── performanceService.ts     # API клиент
└── types/
    └── performance.ts            # TypeScript типы
```

### Расширение функциональности

1. Добавление новых типов метрик
2. Интеграция с внешними системами мониторинга
3. Расширение алертов безопасности
4. Добавление новых сценариев нагрузочного тестирования

### Тестирование

- Unit тесты для всех сервисов
- Integration тесты для API
- E2E тесты для frontend компонентов
- Benchmark тесты для производительности

Система производительности и безопасности Axenta CRM обеспечивает комплексный мониторинг, оптимизацию и защиту приложения, предоставляя администраторам все необходимые инструменты для поддержания высокой производительности и безопасности системы.
