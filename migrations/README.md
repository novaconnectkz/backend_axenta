# Миграция для исправления NOT NULL ограничений на колонках contracts

## Проблема

При создании договора на продакшене возникает ошибка:
```
ERROR: null value in column "start_date" of relation "contracts" violates not-null constraint (SQLSTATE 23502)
```

### Причина

Существует расхождение между моделью GORM и старой миграцией:

1. **Модель GORM** (`models/contract.go`): `StartDate *time.Time` с `gorm:"default:NULL"` - **опционально**
2. **Старая миграция** (`cmd/create_missing_tables/main.go`): `start_date DATE NOT NULL` - **обязательно**

На продакшене таблица была создана старой миграцией с NOT NULL, а код пытается создать договор без start_date/end_date, что вызывает ошибку.

## Решение

### 1. Frontend исправления (уже применено)

В `CreateContract.vue` добавлена логика установки дефолтных дат:
- `start_date` по умолчанию = текущая дата
- `end_date` по умолчанию = текущая дата + 1 год

Это решает проблему на уровне клиента.

### 2. Backend миграция (требуется применить)

Чтобы привести структуру БД в соответствие с моделью GORM, необходимо изменить колонки на nullable.

## Применение миграции

### Способ 1: Через Go-скрипт (рекомендуется)

```bash
cd /Users/com/backend_axenta
go run cmd/migrate_contracts_dates/main.go
```

Скрипт автоматически:
- Найдет все tenant-схемы
- Проверит существование таблицы contracts в каждой схеме
- Проверит текущее состояние колонок
- Применит миграцию только там, где это необходимо
- Выведет подробный отчет

### Способ 2: Через bash-скрипт

```bash
cd /Users/com/backend_axenta/migrations
chmod +x apply_contracts_migration.sh

# Установите переменные окружения
export DB_HOST=localhost
export DB_PORT=5432
export DB_NAME=axenta_crm
export DB_USER=postgres
export DB_PASSWORD=your_password

# Запустите скрипт
./apply_contracts_migration.sh
```

### Способ 3: Вручную через SQL

```sql
-- Для каждой tenant-схемы выполните:
SET search_path TO tenant_XXX;
ALTER TABLE contracts ALTER COLUMN start_date DROP NOT NULL;
ALTER TABLE contracts ALTER COLUMN end_date DROP NOT NULL;
```

Где `tenant_XXX` - название вашей tenant-схемы.

## Проверка результата

После применения миграции проверьте результат:

```sql
-- Для конкретной схемы
SET search_path TO tenant_XXX;
SELECT column_name, is_nullable, data_type 
FROM information_schema.columns 
WHERE table_name = 'contracts' 
AND column_name IN ('start_date', 'end_date');
```

Ожидаемый результат:
```
 column_name | is_nullable | data_type 
-------------+-------------+-----------
 end_date    | YES         | date
 start_date  | YES         | date
```

## Влияние на работу системы

- ✅ Исправлена проблема создания договоров на продакшене
- ✅ Структура БД приведена в соответствие с моделью GORM
- ✅ Старые договоры не затронуты
- ✅ Новые договоры создаются с дефолтными датами из frontend
- ✅ Возможность создания договоров без дат (через API напрямую)

## Откат миграции

Если потребуется откатить миграцию:

```sql
-- Для каждой tenant-схемы:
SET search_path TO tenant_XXX;

-- Сначала обновите NULL значения на дефолтные
UPDATE contracts SET start_date = CURRENT_DATE WHERE start_date IS NULL;
UPDATE contracts SET end_date = start_date + INTERVAL '1 year' WHERE end_date IS NULL;

-- Затем верните NOT NULL ограничение
ALTER TABLE contracts ALTER COLUMN start_date SET NOT NULL;
ALTER TABLE contracts ALTER COLUMN end_date SET NOT NULL;
```

## Дата создания

18 ноября 2025

## Автор

Backend Axenta Team

