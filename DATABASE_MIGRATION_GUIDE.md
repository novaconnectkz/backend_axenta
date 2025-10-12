# Руководство по миграциям базы данных Axenta CRM

## Обзор

Система миграций Axenta CRM обеспечивает автоматическое создание и обновление структуры базы данных с проверкой наличия и структуры таблиц. Система поддерживает мультитенантную архитектуру с глобальными и тенантными таблицами.

## Архитектура

### Типы таблиц

1. **Глобальные таблицы** (схема `public`):
   - `companies` - компании (тенанты)
   - `billing_plans` - тарифные планы
   - `subscriptions` - подписки
   - `integrations` - интеграции с внешними системами
   - `integration_errors` - ошибки интеграций

2. **Тенантные таблицы** (схемы компаний):
   - `users` - пользователи
   - `roles`, `permissions` - роли и разрешения
   - `objects` - объекты мониторинга
   - `contracts` - договоры
   - `equipment` - оборудование
   - `installations` - монтажи
   - И другие...

### Структура миграций

```go
type MigrationInfo struct {
    TableName   string      // Имя таблицы
    Model       interface{} // GORM модель
    Description string      // Описание таблицы
    IsGlobal    bool        // Глобальная или тенантная таблица
}
```

## Использование

### Автоматические миграции

Миграции выполняются автоматически при запуске сервера:

```go
// В database/database.go
if err := RunAllMigrations(false); err != nil {
    log.Printf("⚠️ Ошибка выполнения миграций: %v", err)
}
```

### Ручной запуск миграций

#### Через утилиту командной строки

```bash
# Собрать утилиту миграции
go build -o migrate ./cmd/migrate/

# Выполнить все миграции
./migrate

# Только глобальные миграции
./migrate -global

# Показать план миграций (dry-run)
./migrate -dry-run

# Создать схему для новой компании
./migrate -create-schema tenant_123 -company-id 123

# Справка
./migrate -help
```

#### Через скрипт

```bash
# Выполнить все миграции
./scripts/migrate.sh

# Показать план миграций
./scripts/migrate.sh --dry-run

# Только глобальные таблицы
./scripts/migrate.sh --global

# Пересобрать и запустить
./scripts/migrate.sh --build

# Создать схему для компании
./scripts/migrate.sh --create-schema tenant_123 --company-id 123
```

### Программный интерфейс

```go
import "backend_axenta/database"

// Выполнить все миграции
err := database.RunAllMigrations(false)

// Только глобальные миграции
err := database.RunAllMigrations(true)

// Создать схему для компании
err := database.CreateTenantSchema(companyID, "tenant_123")

// Проверить существование таблицы
exists, err := database.CheckTableExists(db, "users")

// Получить информацию о структуре таблицы
tableInfo, err := database.GetTableInfo(db, "users")

// Сравнить структуру таблицы с моделью
differences, err := database.CompareTableStructure(db, migration)
```

## Функции системы миграций

### Проверка наличия таблиц

Система автоматически проверяет существование каждой таблицы в базе данных:

```sql
SELECT COUNT(*) 
FROM information_schema.tables 
WHERE table_schema = current_schema() 
AND table_name = ?
```

### Проверка структуры таблиц

Для существующих таблиц система сравнивает:
- Наличие колонок
- Типы данных колонок
- Индексы и ограничения
- Значения по умолчанию

### Автоматическое создание/обновление

- **Новые таблицы**: создаются автоматически через GORM AutoMigrate
- **Существующие таблицы**: обновляются при обнаружении различий
- **Безопасность**: система не удаляет существующие данные

### Логирование

Подробное логирование всех операций:

```
🚀 Начинаем процесс миграции базы данных
📋 Выполняем миграции глобальных таблиц (схема public)
🔄 Проверяем таблицу: companies (Таблица компаний)
✅ Таблица companies актуальна
🏢 Выполняем миграции для компании: Test Company (схема: tenant_test)
🔄 Проверяем таблицу: users (Пользователи)
✅ Таблица users создана
📊 Сводка по миграциям:
   ✅ Создано таблиц: 15
   🔄 Обновлено таблиц: 2
   ⏭️  Пропущено таблиц: 8
   ❌ Ошибок: 0
```

## Добавление новых миграций

### 1. Создание модели

```go
// models/new_model.go
type NewModel struct {
    ID        uint           `json:"id" gorm:"primarykey"`
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
    DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
    
    Name      string `json:"name" gorm:"not null;type:varchar(100)"`
    // ... другие поля
}

func (NewModel) TableName() string {
    return "new_models"
}
```

### 2. Добавление в список миграций

```go
// database/migrations.go
func GetAllMigrations() []MigrationInfo {
    return []MigrationInfo{
        // ... существующие миграции
        {
            TableName:   "new_models",
            Model:       &models.NewModel{},
            Description: "Описание новой таблицы",
            IsGlobal:    false, // или true для глобальной таблицы
        },
    }
}
```

### 3. Тестирование

```bash
# Проверить план миграций
./scripts/migrate.sh --dry-run

# Выполнить миграции
./scripts/migrate.sh
```

## Мультитенантность

### Создание новой компании

При создании новой компании автоматически:

1. Создается запись в таблице `companies`
2. Генерируется уникальное имя схемы
3. Создается схема в PostgreSQL
4. Выполняются все тенантные миграции

```go
// Создание компании с автоматической настройкой схемы
company := &models.Company{
    Name:           "New Company",
    DatabaseSchema: "tenant_" + generateRandomString(8),
    // ... другие поля
}

// Сохраняем компанию
db.Create(company)

// Создаем схему и таблицы
err := database.CreateTenantSchema(company.ID, company.DatabaseSchema)
```

### Переключение между схемами

```go
// Переключение на схему компании
db.Exec(fmt.Sprintf("SET search_path TO %s", company.DatabaseSchema))

// Работа с данными компании
var users []models.User
db.Find(&users) // Найдет пользователей только этой компании

// Возврат к глобальной схеме
db.Exec("SET search_path TO public")
```

## Мониторинг и обслуживание

### Проверка состояния

```bash
# Показать текущее состояние всех таблиц
./scripts/migrate.sh --dry-run
```

### Логи миграций

Все операции миграции логируются с временными метками и подробностями:

- Время выполнения каждой миграции
- Обнаруженные изменения
- Ошибки и предупреждения
- Статистика по результатам

### Резервное копирование

Перед выполнением миграций рекомендуется создать резервную копию:

```bash
# Создание резервной копии
pg_dump -h localhost -U username -d database_name > backup_before_migration.sql

# Выполнение миграций
./scripts/migrate.sh

# При необходимости восстановление
psql -h localhost -U username -d database_name < backup_before_migration.sql
```

## Устранение проблем

### Частые проблемы

1. **Ошибка подключения к БД**
   ```
   ❌ Ошибка подключения к базе данных: connection refused
   ```
   - Проверьте настройки подключения в `.env`
   - Убедитесь, что PostgreSQL запущен

2. **Ошибка прав доступа**
   ```
   ❌ Ошибка создания таблицы: permission denied for schema public
   ```
   - Проверьте права пользователя БД
   - Убедитесь, что пользователь может создавать таблицы

3. **Конфликт схем**
   ```
   ❌ Ошибка переключения на схему: schema "tenant_123" does not exist
   ```
   - Создайте схему вручную или через утилиту миграции
   - Проверьте корректность имени схемы в таблице companies

### Восстановление после ошибок

```bash
# Проверить состояние
./scripts/migrate.sh --dry-run

# Выполнить только глобальные миграции
./scripts/migrate.sh --global

# Создать недостающую схему
./scripts/migrate.sh --create-schema tenant_123 --company-id 123
```

## Переменные окружения

```bash
# Обязательные
DB_HOST=localhost
DB_PORT=5432
DB_USER=axenta_user
DB_PASSWORD=secure_password
DB_NAME=axenta_crm

# Опциональные
DB_SSLMODE=disable
DB_MAX_CONNECTIONS=100
DB_MAX_IDLE_CONNECTIONS=10
```

## Безопасность

- Система не удаляет существующие данные
- Все изменения логируются
- Поддержка транзакций для критических операций
- Проверка прав доступа перед выполнением миграций
- Возможность отката через резервные копии

## Производительность

- Параллельное выполнение проверок для разных компаний
- Кэширование информации о структуре таблиц
- Оптимизированные запросы к information_schema
- Минимальное время блокировки таблиц
