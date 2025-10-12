# Быстрый старт: Миграции базы данных

## Установка и настройка

### 1. Настройка переменных окружения

Создайте файл `.env` в корне проекта:

```bash
# Основные настройки БД
DB_HOST=localhost
DB_PORT=5432
DB_USER=axenta_user
DB_PASSWORD=your_password
DB_NAME=axenta_crm
DB_SSLMODE=disable
```

### 2. Проверка состояния БД

```bash
# Показать план миграций (что будет сделано)
./scripts/migrate.sh --dry-run
```

### 3. Выполнение миграций

```bash
# Выполнить все миграции
./scripts/migrate.sh

# Или только глобальные таблицы
./scripts/migrate.sh --global
```

## Основные команды

### Через скрипт (рекомендуется)

```bash
# Все миграции
./scripts/migrate.sh

# Показать план без выполнения
./scripts/migrate.sh --dry-run

# Только глобальные таблицы
./scripts/migrate.sh --global

# Пересобрать утилиту и запустить
./scripts/migrate.sh --build

# Создать схему для новой компании
./scripts/migrate.sh --create-schema tenant_123 --company-id 123
```

### Через утилиту напрямую

```bash
# Собрать утилиту
go build -o migrate ./cmd/migrate/

# Выполнить миграции
./migrate

# Справка
./migrate -help
```

## Типичные сценарии

### Первый запуск проекта

```bash
# 1. Настроить .env файл
cp env.example .env
# Отредактировать .env

# 2. Проверить план миграций
./scripts/migrate.sh --dry-run

# 3. Выполнить все миграции
./scripts/migrate.sh
```

### Добавление новой компании

```bash
# Создать схему для компании с ID 123
./scripts/migrate.sh --create-schema tenant_company123 --company-id 123
```

### Обновление после изменений в коде

```bash
# Проверить, что изменилось
./scripts/migrate.sh --dry-run

# Применить изменения
./scripts/migrate.sh
```

### Проблемы с подключением

```bash
# Только глобальные таблицы (если проблемы с тенантами)
./scripts/migrate.sh --global

# Проверить настройки подключения
./scripts/migrate.sh --dry-run
```

## Что делает система миграций

### ✅ Автоматически создает:
- Базу данных (если не существует)
- Все необходимые таблицы
- Индексы и ограничения
- Схемы для компаний

### ✅ Автоматически проверяет:
- Существование таблиц
- Структуру таблиц
- Соответствие моделям GORM

### ✅ Безопасно обновляет:
- Добавляет новые колонки
- Создает новые индексы
- НЕ удаляет данные

### 📊 Подробно логирует:
- Все выполненные операции
- Время выполнения
- Обнаруженные изменения
- Ошибки и предупреждения

## Структура таблиц

### Глобальные (схема public)
- `companies` - компании-тенанты
- `billing_plans` - тарифные планы
- `subscriptions` - подписки
- `integrations` - интеграции
- `integration_errors` - ошибки интеграций

### Тенантные (схемы компаний)
- `users`, `roles`, `permissions` - пользователи и права
- `objects` - объекты мониторинга
- `contracts` - договоры
- `equipment` - оборудование
- `installations` - монтажи
- `warehouse_operations` - складские операции
- И другие...

## Мониторинг

### Проверка состояния
```bash
./scripts/migrate.sh --dry-run
```

### Логи миграций
Все операции подробно логируются с временными метками и статистикой.

### Пример вывода
```
🚀 Начинаем процесс миграции базы данных
📋 Выполняем миграции глобальных таблиц (схема public)
🔄 Проверяем таблицу: companies (Таблица компаний)
✅ Таблица companies актуальна
📊 Сводка по миграциям:
   ✅ Создано таблиц: 15
   🔄 Обновлено таблиц: 2
   ⏭️  Пропущено таблиц: 8
   ❌ Ошибок: 0
```

## Устранение проблем

### Ошибка подключения
```bash
# Проверить настройки в .env
cat .env

# Проверить доступность PostgreSQL
pg_isready -h localhost -p 5432
```

### Ошибки прав доступа
```sql
-- Дать права пользователю
GRANT ALL PRIVILEGES ON DATABASE axenta_crm TO axenta_user;
GRANT ALL ON SCHEMA public TO axenta_user;
```

### Конфликты схем
```bash
# Пересоздать схему для компании
./scripts/migrate.sh --create-schema tenant_123 --company-id 123
```

## Резервное копирование

### Перед миграциями
```bash
pg_dump -h localhost -U axenta_user axenta_crm > backup_$(date +%Y%m%d_%H%M%S).sql
```

### Восстановление
```bash
psql -h localhost -U axenta_user axenta_crm < backup_20241012_143000.sql
```

## Интеграция в код

### Автоматические миграции при старте
```go
// В main.go или database/database.go
if err := database.RunAllMigrations(false); err != nil {
    log.Printf("⚠️ Ошибка миграций: %v", err)
}
```

### Создание схемы для новой компании
```go
// При создании компании
err := database.CreateTenantSchema(company.ID, company.DatabaseSchema)
```

### Проверка таблиц программно
```go
exists, err := database.CheckTableExists(db, "users")
tableInfo, err := database.GetTableInfo(db, "users")
```
