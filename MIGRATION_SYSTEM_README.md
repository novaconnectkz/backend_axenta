# Система миграций базы данных Axenta CRM

## 📋 Обзор

Создана комплексная система миграций базы данных с автоматической проверкой наличия и структуры таблиц. Система поддерживает мультитенантную архитектуру и обеспечивает безопасное создание и обновление структуры БД.

## 🎯 Основные возможности

### ✅ Автоматическая проверка и создание
- ✅ Проверка существования таблиц
- ✅ Анализ структуры существующих таблиц  
- ✅ Сравнение с GORM моделями
- ✅ Автоматическое создание недостающих таблиц
- ✅ Безопасное обновление структуры

### 🏢 Мультитенантность
- ✅ Глобальные таблицы (схема public)
- ✅ Тенантные таблицы (схемы компаний)
- ✅ Автоматическое создание схем для новых компаний
- ✅ Изоляция данных между компаниями

### 📊 Мониторинг и логирование
- ✅ Подробное логирование всех операций
- ✅ Статистика выполнения миграций
- ✅ Отчеты об изменениях
- ✅ Обработка ошибок

### 🛠️ Удобные инструменты
- ✅ Утилита командной строки
- ✅ Bash-скрипт для автоматизации
- ✅ Режим dry-run для предварительного просмотра
- ✅ Программный API

## 📁 Структура файлов

```
backend_axenta/
├── database/
│   ├── database.go           # Основные функции БД
│   └── migrations.go         # Система миграций ⭐ НОВЫЙ
├── cmd/
│   └── migrate/
│       └── main.go          # Утилита миграций ⭐ НОВЫЙ
├── scripts/
│   └── migrate.sh           # Скрипт автоматизации ⭐ НОВЫЙ
├── examples/
│   └── migration_example.go # Примеры использования ⭐ НОВЫЙ
├── DATABASE_MIGRATION_GUIDE.md      # Подробное руководство ⭐ НОВЫЙ
├── MIGRATION_QUICK_START.md         # Быстрый старт ⭐ НОВЫЙ
└── MIGRATION_SYSTEM_README.md       # Этот файл ⭐ НОВЫЙ
```

## 🚀 Быстрый старт

### 1. Настройка окружения
```bash
# Настроить переменные окружения
cp env.example .env
# Отредактировать .env с настройками БД
```

### 2. Проверка состояния
```bash
# Показать план миграций
./scripts/migrate.sh --dry-run
```

### 3. Выполнение миграций
```bash
# Выполнить все миграции
./scripts/migrate.sh
```

## 🔧 Основные команды

### Через скрипт (рекомендуется)
```bash
./scripts/migrate.sh                    # Все миграции
./scripts/migrate.sh --dry-run          # План без выполнения
./scripts/migrate.sh --global           # Только глобальные таблицы
./scripts/migrate.sh --build            # Пересобрать и запустить
```

### Через утилиту
```bash
go build -o migrate ./cmd/migrate/      # Собрать
./migrate                               # Выполнить все
./migrate -global                       # Только глобальные
./migrate -dry-run                      # План миграций
```

## 📊 Поддерживаемые таблицы

### Глобальные таблицы (схема public)
| Таблица | Модель | Описание |
|---------|--------|----------|
| `companies` | `models.Company` | Компании (тенанты) |
| `billing_plans` | `models.BillingPlan` | Тарифные планы |
| `subscriptions` | `models.Subscription` | Подписки |
| `integrations` | `models.Integration` | Интеграции |
| `integration_errors` | `models.IntegrationError` | Ошибки интеграций |

### Тенантные таблицы (схемы компаний)
| Таблица | Модель | Описание |
|---------|--------|----------|
| `users` | `models.User` | Пользователи |
| `roles` | `models.Role` | Роли |
| `permissions` | `models.Permission` | Разрешения |
| `objects` | `models.Object` | Объекты мониторинга |
| `contracts` | `models.Contract` | Договоры |
| `equipment` | `models.Equipment` | Оборудование |
| `installations` | `models.Installation` | Монтажи |
| `warehouse_operations` | `models.WarehouseOperation` | Складские операции |
| И другие... | | |

## 🔍 Функции проверки

### Проверка существования таблиц
```go
exists, err := database.CheckTableExists(db, "users")
```

### Анализ структуры таблиц
```go
tableInfo, err := database.GetTableInfo(db, "users")
// Возвращает информацию о колонках, индексах, типах данных
```

### Сравнение с моделями
```go
differences, err := database.CompareTableStructure(db, migration)
// Находит различия между БД и GORM моделью
```

## 🏗️ Процесс миграции

### 1. Глобальные таблицы
- Проверка существования в схеме `public`
- Создание/обновление структуры
- Логирование результатов

### 2. Тенантные таблицы
- Получение списка активных компаний
- Переключение на схему каждой компании
- Проверка и создание таблиц
- Возврат к схеме `public`

### 3. Создание новых схем
- Создание схемы PostgreSQL
- Выполнение всех тенантных миграций
- Настройка прав доступа

## 📈 Логирование и мониторинг

### Пример вывода миграций
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

### Детальная информация
- Время выполнения каждой операции
- Список обнаруженных изменений
- Ошибки с подробным описанием
- Статистика по результатам

## 🔒 Безопасность

### Принципы безопасности
- ✅ Никогда не удаляет существующие данные
- ✅ Только добавляет новые структуры
- ✅ Проверка прав доступа
- ✅ Транзакционность критических операций
- ✅ Подробное логирование всех действий

### Рекомендации
- Создавайте резервные копии перед миграциями
- Тестируйте на dev-окружении
- Используйте `--dry-run` для предварительного просмотра

## 🛠️ Добавление новых миграций

### 1. Создайте модель
```go
type NewModel struct {
    ID        uint           `json:"id" gorm:"primarykey"`
    CreatedAt time.Time      `json:"created_at"`
    // ... поля модели
}
```

### 2. Добавьте в список миграций
```go
// В database/migrations.go -> GetAllMigrations()
{
    TableName:   "new_models",
    Model:       &models.NewModel{},
    Description: "Описание новой таблицы",
    IsGlobal:    false, // true для глобальной таблицы
},
```

### 3. Протестируйте
```bash
./scripts/migrate.sh --dry-run  # Проверить план
./scripts/migrate.sh            # Выполнить миграции
```

## 🔧 API для разработчиков

### Основные функции
```go
// Выполнение всех миграций
err := database.RunAllMigrations(globalOnly bool)

// Создание схемы для компании
err := database.CreateTenantSchema(companyID uint, schemaName string)

// Проверка таблицы
exists, err := database.CheckTableExists(db *gorm.DB, tableName string)

// Информация о таблице
info, err := database.GetTableInfo(db *gorm.DB, tableName string)

// Сравнение структуры
diffs, err := database.CompareTableStructure(db *gorm.DB, migration MigrationInfo)

// Выполнение одной миграции
result := database.RunMigration(db *gorm.DB, migration MigrationInfo)
```

### Структуры данных
```go
type MigrationInfo struct {
    TableName   string      // Имя таблицы
    Model       interface{} // GORM модель
    Description string      // Описание
    IsGlobal    bool        // Глобальная/тенантная
}

type MigrationResult struct {
    TableName string        // Имя таблицы
    Action    string        // created/updated/skipped/error
    Changes   []string      // Список изменений
    Error     error         // Ошибка если есть
    Duration  time.Duration // Время выполнения
}
```

## 📚 Документация

- **[DATABASE_MIGRATION_GUIDE.md](DATABASE_MIGRATION_GUIDE.md)** - Подробное руководство
- **[MIGRATION_QUICK_START.md](MIGRATION_QUICK_START.md)** - Быстрый старт
- **[examples/migration_example.go](examples/migration_example.go)** - Примеры кода

## 🐛 Устранение проблем

### Частые ошибки и решения

**Ошибка подключения к БД:**
```bash
# Проверить настройки
cat .env
pg_isready -h localhost -p 5432
```

**Ошибки прав доступа:**
```sql
GRANT ALL PRIVILEGES ON DATABASE axenta_crm TO axenta_user;
```

**Конфликты схем:**
```bash
./scripts/migrate.sh --create-schema tenant_123 --company-id 123
```

## 🎯 Интеграция в проект

### Автоматические миграции при старте
Система уже интегрирована в `database/database.go`:

```go
// Выполняем миграции с проверкой структуры
if err := RunAllMigrations(false); err != nil {
    log.Printf("⚠️ Ошибка выполнения миграций: %v", err)
    log.Println("Продолжаем работу - некоторые функции могут быть недоступны")
} else {
    log.Println("✅ Миграции выполнены успешно")
}
```

### Создание новых компаний
```go
// При создании компании автоматически создается схема
company := &models.Company{
    Name:           "New Company",
    DatabaseSchema: "tenant_" + generateRandomString(8),
    // ... другие поля
}
db.Create(company)

// Создаем схему и таблицы
err := database.CreateTenantSchema(company.ID, company.DatabaseSchema)
```

## ✅ Заключение

Система миграций полностью готова к использованию и обеспечивает:

- 🔄 Автоматическую проверку и создание всех необходимых таблиц
- 🏢 Полную поддержку мультитенантной архитектуры  
- 🛡️ Безопасное обновление структуры БД без потери данных
- 📊 Подробное логирование и мониторинг процесса
- 🛠️ Удобные инструменты для разработчиков и администраторов

Система готова к production использованию и не нарушает существующую функциональность проекта.
