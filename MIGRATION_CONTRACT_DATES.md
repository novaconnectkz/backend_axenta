# Миграция: Сделать даты договора необязательными

## Проблема

При создании договора возникает ошибка:
```
ERROR: null value in column "start_date" of relation "contracts" violates not-null constraint (SQLSTATE 23502)
```

## Причина

На продакшене колонка `start_date` имеет ограничение `NOT NULL`, но согласно новой бизнес-логике:
- Договор создается БЕЗ дат начала и окончания
- Даты устанавливаются позже через создание подписки
- Это позволяет создавать договоры-черновики без привязки к подписке

## Решение

Применить миграцию `000010_make_contract_dates_nullable.sql` для изменения структуры таблицы contracts.

## Применение миграции на продакшене

### Вариант 1: Через psql

```bash
# Подключаемся к базе данных
psql -h <host> -U <user> -d <database>

# Применяем миграцию
\i migrations/000010_make_contract_dates_nullable.sql
```

### Вариант 2: Прямое выполнение SQL

```sql
-- Сделать start_date nullable
ALTER TABLE contracts ALTER COLUMN start_date DROP NOT NULL;

-- Сделать end_date nullable
ALTER TABLE contracts ALTER COLUMN end_date DROP NOT NULL;

-- Добавить комментарии
COMMENT ON COLUMN contracts.start_date IS 'Дата начала действия договора. Устанавливается через подписку.';
COMMENT ON COLUMN contracts.end_date IS 'Дата окончания действия договора. Устанавливается через подписку.';
```

### Вариант 3: Через миграционный инструмент

Если используется golang-migrate или другой миграционный инструмент:

```bash
cd backend_axenta
migrate -path ./migrations -database "postgres://user:password@host:port/database?sslmode=require" up
```

## Проверка

После применения миграции проверьте структуру таблицы:

```sql
\d contracts
```

Колонки `start_date` и `end_date` должны иметь `nullable: yes`.

## Откат (при необходимости)

Если нужно откатить изменения:

```sql
-- Сначала установите значения по умолчанию для существующих NULL
UPDATE contracts 
SET start_date = CURRENT_DATE 
WHERE start_date IS NULL;

UPDATE contracts 
SET end_date = start_date + INTERVAL '1 year'
WHERE end_date IS NULL AND start_date IS NOT NULL;

-- Затем верните ограничение NOT NULL
ALTER TABLE contracts ALTER COLUMN start_date SET NOT NULL;
-- end_date можно оставить nullable, т.к. это было и раньше
```

## После применения

1. ✅ Договоры можно создавать без указания дат
2. ✅ Даты устанавливаются через подписку
3. ✅ Существующие договоры с датами продолжают работать
4. ✅ Новая бизнес-логика: Договор → Подписка → Даты

