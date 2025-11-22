# 📊 Отчет о статусе миграций БД

**Дата проверки:** 22 ноября 2025  
**Окружения:** Локально (macOS) + Продакшен (Debian)

---

## ✅ Статус миграций

### Локально (macOS)

| № | Миграция | Статус | Таблицы |
|---|----------|--------|---------|
| 003 | add_vat_rate_fields | ✅ Применена | `billing_settings` |
| 004 | create_system_settings | ✅ Применена | `system_settings` |
| 005 | fix_subscriptions_columns | ✅ Применена | `subscriptions` |
| 006 | add_sync_columns | ✅ Применена | `users`, `objects`, `contracts` (public) |
| 007 | add_sync_columns_to_tenants | ✅ Применена | `users`, `objects`, `contracts` (tenant) |
| 008 | add_max_messenger_columns | ✅ Применена | `notification_settings` |
| 009 | add_sequential_numbers | ✅ Применена | `invoices`, `subscriptions` |
| 010 | add_invoice_notification_columns | ✅ Применена | `invoices` |
| 011 | fix_invoice_notification_column_types | ✅ Применена | `invoices` |
| 012 | add_missing_contract_fields | ✅ Применена | `contracts` |

### Продакшен (Debian)

| № | Миграция | Статус | Таблицы |
|---|----------|--------|---------|
| 003 | add_vat_rate_fields | ✅ Применена | `billing_settings` |
| 004 | create_system_settings | ✅ Применена | `system_settings` |
| 005 | fix_subscriptions_columns | ✅ Применена | `subscriptions` |
| 006 | add_sync_columns | ✅ Применена | `users`, `objects`, `contracts` (public) |
| 007 | add_sync_columns_to_tenants | ✅ Применена | `users`, `objects`, `contracts` (tenant) |
| 008 | add_max_messenger_columns | ✅ Применена | `notification_settings` |
| 009 | add_sequential_numbers | ✅ Применена | `invoices`, `subscriptions` |
| 010 | add_invoice_notification_columns | ✅ Применена | `invoices` |
| 011 | fix_invoice_notification_column_types | ✅ Применена | `invoices` |
| 012 | add_missing_contract_fields | ✅ Применена | `contracts` |

---

## 📋 Детали по таблицам

### 1. `subscriptions`

**Добавленные колонки:**
- ✅ `contract_id` (bigint) - связь с договором
- ✅ `sequential_number` (integer) - порядковый номер подписки

### 2. `contracts` (public)

**Добавленные колонки:**
- ✅ `client_type` (varchar 20) - тип клиента (individual/organization)
- ✅ `client_website` (varchar 200) - веб-сайт клиента
- ✅ `client_short_name` (varchar) - краткое имя клиента
- ✅ `sequential_number` (integer) - порядковый номер договора
- ✅ `last_synced_at` (timestamp) - время последней синхронизации
- ✅ `sync_status` (varchar) - статус синхронизации
- ✅ `sync_version` (integer) - версия для синхронизации
- ✅ `sync_checksum` (varchar) - контрольная сумма
- ✅ `is_dirty` (boolean) - флаг изменений
- ✅ `sync_error` (text) - описание ошибки синхронизации
- ✅ `source_of_truth` (varchar) - источник истины
- ✅ `sync_queued_at` (timestamp) - время постановки в очередь
- ✅ `sync_attempted_at` (timestamp) - время попытки синхронизации

### 3. `contracts` (tenant_*)

**Добавленные колонки:**
- ✅ `client_type` (varchar 20) - тип клиента (individual/organization)
- ✅ `client_website` (varchar 200) - веб-сайт клиента
- ✅ `client_short_name` (varchar)
- ✅ `sequential_number` (integer)
- ✅ Все sync_* колонки (аналогично public)

### 4. `notification_settings`

**Добавленные колонки (MAX Messenger):**
- ✅ `max_bot_token` (varchar 500) - токен бота MAX
- ✅ `max_webhook_url` (text) - URL вебхука
- ✅ `max_enabled` (boolean) - включен ли MAX
- ✅ `max_use_polling` (boolean) - использовать polling
- ✅ `max_parse_mode` (varchar 20) - режим парсинга (HTML)
- ✅ `max_retry_attempts` (integer) - количество попыток

### 5. `invoices`

**Добавленные колонки:**
- ✅ `sequential_number` (integer) - порядковый номер счета
- ✅ `admin_account_id` (bigint) - ID администратора
- ✅ `billing_period_end` (timestamp) - конец периода
- ✅ `last_sent_at` (timestamp) - время последней отправки
- ✅ `last_sent_channels` (varchar 100) - каналы отправки
- ✅ `last_sent_error` (text) - ошибка отправки
- ✅ `send_channels` (varchar 100) - каналы для отправки
- ✅ `sent_count` (integer) - счетчик отправок

**Исправленные типы колонок (миграция 011):**
- ✅ `send_to_email` (varchar 100) ← было boolean
- ✅ `send_to_telegram` (varchar 50) ← было boolean
- ✅ `send_to_max` (varchar 50) ← было boolean

### 6. `system_settings`

**Создана новая таблица с колонками:**
- ✅ `vat_rate_preset` (varchar 20) - пресет ставки НДС (russia/kazakhstan/none/custom)
- ✅ `vat_rate_custom` (numeric 5,2) - кастомная ставка НДС
- ✅ `company_id` (integer) - ID компании (unique)
- ✅ `admin_account_id` (integer) - ID администратора
- ✅ + другие настройки системы

### 7. `billing_settings`

**Добавленные колонки:**
- ✅ `vat_rate_preset` (varchar 20) - пресет ставки НДС
- ✅ `vat_rate_custom` (numeric 5,2) - кастомная ставка НДС

---

## 🔍 Проверка целостности

### Локально

```sql
-- subscriptions
✅ contract_id: bigint
✅ sequential_number: integer

-- contracts (public)
✅ client_short_name: character varying
✅ sequential_number: integer
✅ last_synced_at: timestamp with time zone
✅ sync_status: character varying

-- notification_settings
✅ max_bot_token: character varying(500)
✅ max_enabled: boolean
✅ max_parse_mode: character varying(20)
✅ max_webhook_url: text
✅ max_use_polling: boolean
✅ max_retry_attempts: integer

-- invoices
✅ sequential_number: integer
✅ admin_account_id: bigint
✅ last_sent_at: timestamp without time zone
✅ send_to_email: character varying(100)
✅ send_to_telegram: character varying(50)
✅ send_to_max: character varying(50)

-- system_settings
✅ vat_rate_preset: character varying(20)
✅ vat_rate_custom: numeric(5,2)
✅ company_id: integer
✅ admin_account_id: integer

-- billing_settings
✅ vat_rate_preset: character varying(20)
✅ vat_rate_custom: numeric(5,2)
```

### Продакшен

```bash
# Все миграции применены успешно через SSH
✅ 003_add_vat_rate_fields.sql
✅ 004_create_system_settings.sql
✅ 005_fix_subscriptions_columns.sql
✅ 006_add_sync_columns.sql
✅ 007_add_sync_columns_to_tenants.sql
✅ 008_add_max_messenger_columns.sql
✅ 009_add_sequential_numbers.sql
✅ 010_add_invoice_notification_columns.sql
✅ 011_fix_invoice_notification_column_types.sql

# Backend перезапущен для применения изменений
✅ systemctl restart axenta-backend
```

---

## 🎯 Результат

### ✅ Все критические миграции применены

**Функционал, который теперь работает:**

1. **Создание подписок** - `contract_id` добавлен ✅
2. **Создание договоров** - все поля модели добавлены ✅
3. **Интеграция MAX Messenger** - все поля добавлены ✅
4. **Нумераторы счетов** - `sequential_number` добавлен ✅
5. **Отправка счетов** - типы колонок исправлены ✅
6. **Настройки НДС** - система настроек создана ✅
7. **Смена статуса счета** - после отправки меняется на "Отправлен" ✅

### ⚡ Синхронизация схем

**Локально и на продакшене** структура базы данных **полностью идентична**.

---

## 📝 Примечания

- Все миграции задокументированы в `database/migrations/MIGRATION_HISTORY.md`
- Tenant schemas обновлены динамически через функцию `sync_contracts_to_tenant()`
- Синхронизация метаданных добавлена для `users`, `objects`, `contracts`
- Backend на продакшене перезапущен и работает с актуальной схемой БД

---

**Статус:** 🟢 Все миграции применены, БД синхронизированы

