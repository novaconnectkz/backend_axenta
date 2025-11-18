# СРОЧНАЯ МИГРАЦИЯ: Исправление ошибки создания договоров

## Проблема
```
ERROR: null value in column "start_date" of relation "contracts" violates not-null constraint
```

## Причина
База данных на продакшене требует обязательное заполнение `start_date`, но новая логика предполагает, что даты устанавливаются через подписку.

## Быстрое решение (для продакшена)

### Выполните эти SQL команды на продакшене:

```sql
-- Сделать start_date nullable
ALTER TABLE contracts ALTER COLUMN start_date DROP NOT NULL;

-- Сделать end_date nullable  
ALTER TABLE contracts ALTER COLUMN end_date DROP NOT NULL;
```

### Проверка:
```sql
-- Проверьте, что изменения применились
\d contracts

-- Должно показать что start_date и end_date теперь nullable
```

### Проверка работы:
После применения миграции попробуйте создать договор через интерфейс.

## Файлы для справки
- Миграция: `migrations/000010_make_contract_dates_nullable.sql`
- Документация: `MIGRATION_CONTRACT_DATES.md`

## Время выполнения
~5 секунд (миграция очень быстрая, не требует блокировки таблицы)

## Риски
✅ Минимальные - только изменяем ограничение NOT NULL на NULL
✅ Не влияет на су��ествующие данные
✅ Обратная совместимость сохранена

