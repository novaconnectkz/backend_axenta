# Мультитенантность в Axenta CRM

## Обзор

Система реализует мультитенантность на уровне схем PostgreSQL, обеспечивая полную изоляцию данных между компаниями.

## Архитектура

### 1. Модель Company

- Каждая компания имеет свою собственную схему БД
- Поле `database_schema` определяет имя схемы (например, `tenant_company123`)
- Поддерживается активация/деактивация компаний

### 2. TenantMiddleware

Middleware автоматически определяет текущую компанию и переключает контекст БД:

```go
// Создание middleware
tenantMiddleware := middleware.NewTenantMiddleware(database.DB)

// Применение к API группе
apiGroup.Use(tenantMiddleware.SetTenant())
```

### 3. Определение компании

Middleware поддерживает несколько способов определения компании:

1. **Заголовок X-Tenant-ID** (приоритет 1)

```bash
curl -H "X-Tenant-ID: 123" /api/objects
```

2. **Домен/поддомен** (приоритет 2)

```bash
# company.example.com или tenant123.example.com
```

3. **JWT токен** (приоритет 3)

- Извлечение `company_id`/`tenant_id` из токена
- Запрос к Axenta Cloud API для получения данных компании

4. **Компания по умолчанию** (fallback)

### 4. Переключение схем БД

```go
// Получение БД текущей компании
tenantDB := middleware.GetTenantDB(c)

// Получение информации о компании
company := middleware.GetCurrentCompany(c)
companyID := middleware.GetCompanyID(c)
```

## Использование

### В контроллерах

```go
func GetObjects(c *gin.Context) {
    // Получаем БД текущей компании
    tenantDB := middleware.GetTenantDB(c)
    if tenantDB == nil {
        c.JSON(500, gin.H{"error": "Tenant DB not available"})
        return
    }

    var objects []models.Object
    tenantDB.Find(&objects)

    c.JSON(200, gin.H{"data": objects})
}
```

### Создание новой компании

```go
company := &models.Company{
    Name:           "New Company",
    DatabaseSchema: "tenant_new_company",
    AxetnaLogin:    "login",
    AxetnaPassword: "encrypted_password",
    ContactEmail:   "admin@newcompany.com",
    IsActive:       true,
}

// Сохраняем компанию
db.Create(company)

// Создаем схему БД для компании
tenantMiddleware := middleware.NewTenantMiddleware(db)
err := tenantMiddleware.CreateTenantSchema(company.GetSchemaName())
```

## Безопасность

### Изоляция данных

- Каждая компания работает в своей схеме PostgreSQL
- Полная изоляция пользователей, объектов, договоров и других данных
- Невозможность доступа к данным других компаний

### Кэширование

- Информация о компаниях кэшируется в Redis (15 минут)
- Кэш автоматически инвалидируется при изменениях

### Публичные маршруты

Следующие маршруты не требуют определения компании:

- `/ping`
- `/api/auth/login`
- `/health`
- `/metrics`

## Тестирование

### Запуск тестов мультитенантности

```bash
# Тесты middleware
go test ./middleware -v

# Тесты API с мультитенантностью
go test ./api -v -run TestMultiTenant

# Все тесты
go test ./... -v
```

### Тестовые сценарии

1. **Изоляция данных**: Проверка, что компании видят только свои данные
2. **Переключение схем**: Корректное переключение между схемами БД
3. **Производительность**: Бенчмарки переключения схем
4. **Граничные случаи**: Некорректные ID, деактивированные компании
5. **Конкурентность**: Одновременный доступ к разным компаниям

## Мониторинг

### Логирование

Все операции с компаниями логируются:

- Переключение схем
- Ошибки определения компании
- Проблемы с БД

### Метрики

- Время переключения схем
- Количество активных компаний
- Ошибки мультитенантности

## Развертывание

### База данных

```sql
-- Создание основной схемы
CREATE SCHEMA IF NOT EXISTS public;

-- Схемы компаний создаются автоматически
-- Например: tenant_company123, tenant_company456
```

### Переменные окружения

```env
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=axenta_db
DB_SSLMODE=disable
```

## Миграции

При создании новой схемы компании автоматически выполняются миграции для всех моделей:

- Users, Roles, Permissions
- Objects, Templates
- Contracts, Billing Plans
- Locations, Installers, Equipment

## Ограничения

1. **Глобальные данные**: Модель `Company` и глобальные настройки остаются в схеме `public`
2. **Производительность**: Переключение схем добавляет небольшую задержку
3. **Сложность**: Требует внимательности при разработке новых функций

## Troubleshooting

### Компания не найдена

```
ERROR: Не удалось определить компанию
```

- Проверьте заголовок `X-Tenant-ID`
- Убедитесь, что компания активна
- Проверьте настройки домена

### Ошибка переключения схемы

```
ERROR: Ошибка подключения к схеме компании
```

- Проверьте, что схема существует в БД
- Убедитесь в корректности прав доступа к БД
- Проверьте логи PostgreSQL

### Проблемы с производительностью

- Включите кэширование Redis
- Оптимизируйте запросы к БД
- Рассмотрите connection pooling
