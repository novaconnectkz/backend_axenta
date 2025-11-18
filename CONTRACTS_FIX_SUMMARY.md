# ✅ ИСПРАВЛЕНА ОШИБКА СОЗДАНИЯ ДОГОВОРОВ НА ПРОДАКШЕНЕ

## Проблема
При создании договора на продакшене возникала ошибка:
```
ERROR: null value in column "start_date" of relation "contracts" violates not-null constraint
```

## Что было сделано

### 1. ✅ Frontend (уже работает)
**Файл:** `frontend_axenta/src/views/CreateContract.vue`

Добавлена автоматическая установка дат при создании договора:
- `start_date` → текущая дата (по умолчанию)
- `end_date` → текущая дата + 1 год (по умолчанию)

**Эффект:** Создание договоров теперь работает без ошибок.

### 2. ✅ Backend - исправление старой миграции
**Файл:** `backend_axenta/cmd/create_missing_tables/main.go`

Изменена структура таблицы contracts:
- `start_date DATE NOT NULL` → `start_date DATE`
- `end_date DATE NOT NULL` → `end_date DATE`

**Эффект:** Новые окружения будут создаваться с правильной структурой.

### 3. ⚠️ Миграция для продакшена (требуется применить)

Созданы файлы:
- `backend_axenta/migrations/20251118_make_contracts_dates_nullable.sql` - SQL миграция
- `backend_axenta/migrations/apply_contracts_migration.sh` - Bash скрипт
- `backend_axenta/cmd/migrate_contracts_dates/main.go` - Go скрипт
- `backend_axenta/migrations/README.md` - Полная документация

## Применение миграции на продакшене

### Вариант 1: Go-скрипт (рекомендуется)
```bash
cd /Users/com/backend_axenta
go run cmd/migrate_contracts_dates/main.go
```

### Вариант 2: Bash-скрипт
```bash
cd /Users/com/backend_axenta/migrations
export DB_PASSWORD=your_password
./apply_contracts_migration.sh
```

### Вариант 3: Вручную через SQL
Для каждой tenant-схемы:
```sql
SET search_path TO tenant_XXX;
ALTER TABLE contracts ALTER COLUMN start_date DROP NOT NULL;
ALTER TABLE contracts ALTER COLUMN end_date DROP NOT NULL;
```

## Проверка

После применения миграции создание договоров должно работать без ошибок.

Проверить можно так:
```sql
SELECT column_name, is_nullable 
FROM information_schema.columns 
WHERE table_name = 'contracts' 
AND column_name IN ('start_date', 'end_date');
```

Должно быть: `is_nullable = YES`

## Статус

- ✅ Frontend исправлен - работает немедленно
- ✅ Backend исправлен для новых окружений
- ⚠️ Миграция для продакшена - требует ручного запуска

## Безопасность

- Миграция НЕ изменяет существующие данные
- Миграция только убирает NOT NULL ограничение
- Откат миграции возможен (см. README.md)
- После frontend исправления ошибка не повторится, даже если миграция не применена

---

📝 Полная документация: `backend_axenta/migrations/README.md`

