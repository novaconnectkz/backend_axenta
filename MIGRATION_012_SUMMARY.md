# ✅ Миграция 012: Добавление недостающих полей в contracts

**Дата:** 22 ноября 2025  
**Статус:** Успешно применена

---

## 🎯 Проблема

При создании договора на продакшене возникала ошибка:

```
ERROR: column "client_type" of relation "contracts" does not exist (SQLSTATE 42703)
```

Фронтенд отправлял поля `client_type` и `client_website`, которые присутствуют в модели Go, но отсутствовали в базе данных.

---

## 🔧 Решение

Создана и применена миграция `012_add_missing_contract_fields.sql`, которая добавляет:

1. **`client_type`** (VARCHAR 20) - тип клиента:
   - `organization` - для юридических лиц (если есть ИНН)
   - `individual` - для физических лиц

2. **`client_website`** (VARCHAR 200) - веб-сайт клиента

---

## 📋 Применение миграции

### Локально (macOS)

```bash
psql -U postgres -d axenta_db -f database/migrations/012_add_missing_contract_fields.sql
```

**Результат:**
```
✅ Public schema: 2 columns added
✅ Tenant schemas: 4/4 updated
   - tenant_default
   - tenant_1803
   - tenant_186
   - tenant_2110
```

### Продакшен (Debian)

```bash
# Public schema
sudo -u postgres psql -d axenta_db << 'SQL'
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_type VARCHAR(20);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_website VARCHAR(200);
UPDATE public.contracts SET client_type = 'organization' 
WHERE client_type IS NULL AND client_inn IS NOT NULL AND client_inn != '';
UPDATE public.contracts SET client_type = 'individual' WHERE client_type IS NULL;
SQL

# Tenant schemas
sudo -u postgres psql -d axenta_db -f /tmp/fix_tenants.sql
```

**Результат:**
```
✅ Updated tenant_default.contracts
✅ Updated tenant_186.contracts
⚠️ Skipped tenant_yyqqqqqq (no contracts table)
⚠️ Skipped tenant_newacrm (no contracts table)
```

---

## ✅ Проверка

### Локально

```sql
SELECT column_name, data_type, character_maximum_length 
FROM information_schema.columns 
WHERE table_schema = 'public' 
AND table_name = 'contracts' 
AND column_name IN ('client_type', 'client_website', 'client_short_name') 
ORDER BY column_name;
```

**Результат:**
```
    column_name    |     data_type     | character_maximum_length 
-------------------+-------------------+--------------------------
 client_short_name | character varying |                      200
 client_type       | character varying |                       20
 client_website    | character varying |                      200
```

### Продакшен

```sql
-- Аналогичный запрос через SSH
✅ client_type: добавлена
✅ client_website: добавлена
```

---

## 🎉 Результат

Теперь создание договоров работает корректно на продакшене:

- ✅ Фронтенд отправляет `client_type` и `client_website`
- ✅ Backend принимает и сохраняет эти поля
- ✅ Существующие записи получили значения по умолчанию
- ✅ Все tenant схемы синхронизированы

---

## 📝 Файлы

- Миграция: `database/migrations/012_add_missing_contract_fields.sql`
- История: `database/migrations/MIGRATION_HISTORY.md`
- Отчет: `MIGRATION_STATUS_REPORT.md`

