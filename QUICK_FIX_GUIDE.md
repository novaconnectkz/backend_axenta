# 🔧 QUICK FIX: Создание договоров на продакшене

## ⚡ Немедленное решение (уже работает)

Исправлен код frontend - теперь при создании договора автоматически устанавливаются:
- **start_date** → текущая дата
- **end_date** → текущая дата + 1 год

**Файл:** `frontend_axenta/src/views/CreateContract.vue` (строки 1179-1203)

## 🚀 Применение миграции на продакшене

Выберите один из способов:

### Способ 1️⃣: Go-скрипт (рекомендуется) ⭐

```bash
cd /Users/com/backend_axenta
go run cmd/migrate_contracts_dates/main.go
```

**Что делает:**
- ✅ Автоматически находит все tenant-схемы
- ✅ Проверяет существование таблицы
- ✅ Показывает текущее состояние
- ✅ Применяет изменения только там где нужно
- ✅ Подробный отчет

**Вывод:**
```
🚀 Применение миграции для колонок contracts.start_date и contracts.end_date
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ Подключение к базе данных установлено
✅ Найдено схем: 5

🔄 Обработка схемы: tenant_186
  📋 Текущее состояние колонок:
    start_date: nullable=NO
    end_date: nullable=NO
  🔄 Применение миграции...
  ✅ Миграция применена успешно

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ Миграция завершена
📊 Обработано схем: 5
✅ Успешно: 5
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### Способ 2️⃣: Bash-скрипт

```bash
cd /Users/com/backend_axenta/migrations

# Установите переменные окружения
export DB_HOST=localhost
export DB_PORT=5432
export DB_NAME=axenta_crm
export DB_USER=postgres
export DB_PASSWORD=your_password

# Запустите скрипт
./apply_contracts_migration.sh
```

### Способ 3️⃣: Вручную через SQL

Для каждой tenant-схемы выполните:
```sql
SET search_path TO tenant_186;  -- замените на вашу схему
ALTER TABLE contracts ALTER COLUMN start_date DROP NOT NULL;
ALTER TABLE contracts ALTER COLUMN end_date DROP NOT NULL;
```

## ✅ Проверка результата

```sql
-- Проверить для конкретной схемы
SET search_path TO tenant_186;
SELECT column_name, is_nullable 
FROM information_schema.columns 
WHERE table_name = 'contracts' 
AND column_name IN ('start_date', 'end_date');
```

**Ожидаемый результат:**
```
 column_name | is_nullable 
-------------+-------------
 end_date    | YES
 start_date  | YES
```

## 📊 Что изменилось

| До | После |
|----|-------|
| `start_date DATE NOT NULL` | `start_date DATE` |
| `end_date DATE NOT NULL` | `end_date DATE` |
| ❌ Ошибка при создании | ✅ Работает |

## 🛡️ Безопасность

- ✅ Не изменяет существующие данные
- ✅ Только убирает NOT NULL ограничение
- ✅ Откат возможен (см. README.md)
- ✅ Нулевой downtime

## 📝 Дополнительная информация

- 📖 Полная документация: `backend_axenta/migrations/README.md`
- 📄 SQL миграция: `backend_axenta/migrations/20251118_make_contracts_dates_nullable.sql`
- 🔧 Bash скрипт: `backend_axenta/migrations/apply_contracts_migration.sh`
- 💻 Go скрипт: `backend_axenta/cmd/migrate_contracts_dates/main.go`

---

**Дата:** 18 ноября 2025  
**Статус:** ✅ Frontend исправлен, ⚠️ миграция БД требует применения

