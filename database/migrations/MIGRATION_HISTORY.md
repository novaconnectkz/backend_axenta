# Примененные миграции на продакшен-сервере

## Дата: 2025-11-21

### Миграции базы данных

#### 1. Системные настройки (003_add_vat_rate_fields.sql, 004_create_system_settings.sql)
- Добавлены поля `vat_rate_preset` и `vat_rate_custom` в таблицу `billing_settings`
- Создана таблица `system_settings` для хранения системных настроек компаний
- Статус: ✅ Применено

#### 2. Исправление таблицы subscriptions (005_fix_subscriptions_columns.sql)
- Добавлена колонка `contract_id` в таблицу `subscriptions`
- Создан индекс на `contract_id`
- Статус: ✅ Применено

#### 3. Метаданные синхронизации - public схема (006_add_sync_columns.sql)
Добавлены колонки синхронизации в таблицы `public.contracts`, `public.objects`, `public.users`:
- `last_synced_at` - дата последней синхронизации
- `sync_status` - статус синхронизации (idle, pending, syncing, error)
- `sync_version` - версия синхронизации
- `sync_checksum` - контрольная сумма
- `is_dirty` - флаг изменений
- `sync_error` - текст ошибки
- `source_of_truth` - источник истины (local, remote)
- `sync_queued_at` - время постановки в очередь
- `sync_attempted_at` - время последней попытки

Дополнительно для `contracts`:
- `sequential_number` - последовательный номер
- `client_short_name` - сокращенное название клиента

Статус: ✅ Применено

#### 4. Метаданные синхронизации - tenant схемы (007_add_sync_columns_to_tenants.sql)
Те же колонки, что и в миграции #3, но для всех tenant-схем:
- `tenant_186` (основная рабочая схема)
- `tenant_default`
- `tenant_newacrm`
- `tenant_yyqqqqqq`

Статус: ✅ Применено

### Перезапуск сервисов

#### Backend Service
```bash
systemctl stop axenta-backend
pkill -9 -f axenta_backend
systemctl start axenta-backend
```
Статус: ✅ Перезапущен

### Проверка работоспособности

#### Эндпоинты API
- ✅ `/api/auth/system/settings` - GET/PUT
- ✅ `/api/auth/billing/invoice-numerators` - GET/POST/PUT/DELETE
- ✅ `/api/auth/billing/contracts/:id/invoice` - POST
- ✅ `/api/auth/contracts` - GET/POST/PUT/DELETE

#### База данных
- ✅ Таблица `public.contracts` - все колонки присутствуют
- ✅ Таблица `tenant_186.contracts` - все колонки присутствуют
- ✅ Таблица `public.subscriptions` - колонка `contract_id` присутствует
- ✅ Таблица `public.system_settings` - создана
- ✅ Таблица `public.billing_settings` - поля НДС добавлены

### Функциональность

После применения всех миграций на продакшене работает:
- ✅ Создание и редактирование договоров
- ✅ Создание и управление подписками
- ✅ Генерация счетов
- ✅ Управление нумераторами счетов
- ✅ Настройка ставок НДС в системных настройках
- ✅ Синхронизация с Axenta Cloud (метаданные готовы)

### Команды для проверки

```sql
-- Проверка колонок в public.contracts
SELECT column_name FROM information_schema.columns 
WHERE table_schema = 'public' AND table_name = 'contracts' 
ORDER BY column_name;

-- Проверка колонок в tenant_186.contracts
SELECT column_name FROM information_schema.columns 
WHERE table_schema = 'tenant_186' AND table_name = 'contracts' 
ORDER BY column_name;

-- Проверка колонки contract_id в subscriptions
SELECT column_name FROM information_schema.columns 
WHERE table_schema = 'public' AND table_name = 'subscriptions' 
AND column_name = 'contract_id';

-- Проверка наличия system_settings
SELECT COUNT(*) FROM information_schema.tables 
WHERE table_schema = 'public' AND table_name = 'system_settings';
```

### Контакты сервера

- **IP**: 194.87.143.169
- **Пользователь**: root
- **База данных**: axenta_db
- **PostgreSQL пользователь**: postgres
- **Backend порт**: 8080
- **API URL**: https://api.axenta.glonass-saratov.ru

#### 5. Интеграция MAX messenger (008_add_max_messenger_columns.sql)
Добавлены колонки для интеграции с российским мессенджером MAX в таблицу `notification_settings`:
- `max_bot_token` - токен бота MAX
- `max_webhook_url` - URL для вебхуков
- `max_enabled` - включен ли MAX
- `max_use_polling` - использовать Long Polling вместо Webhook
- `max_parse_mode` - режим парсинга сообщений (HTML, Markdown, MarkdownV2)

Статус: ✅ Применено

#### 6. Последовательные номера (009_add_sequential_numbers.sql)
Добавлена колонка `sequential_number` в таблицы:
- `public.invoices` - последовательный номер счёта в рамках компании
- `public.subscriptions` - последовательный номер подписки в рамках компании

Созданы индексы для производительности:
- `idx_invoices_sequential_number`
- `idx_subscriptions_sequential_number`

Статус: ✅ Применено

### Примечания

1. На сервере используется архитектура с tenant-схемами (multi-tenancy)
2. Основная рабочая схема для `company_id=186` - `tenant_186`
3. Все миграции применены с использованием `IF NOT EXISTS` для идемпотентности
4. При создании новых tenant-схем необходимо применять миграцию 007
5. Файлы миграций сохранены в `/Users/com/backend_axenta/database/migrations/`
6. Все структурные различия между локальной и продакшен базами устранены


## Migration 012: Add missing contract fields (2025-11-22)

### Цель
Добавление недостающих полей `client_type` и `client_website` в таблицу `contracts`.

### Изменения
- Добавлена колонка `client_type` VARCHAR(20) в `public.contracts`
- Добавлена колонка `client_website` VARCHAR(200) в `public.contracts`
- Добавлены те же колонки в tenant схемы с таблицей `contracts`
- Установлены значения по умолчанию для существующих записей

### Применено
- ✅ Локально: 2025-11-22
- ✅ Продакшен: 2025-11-22

### Статус
Успешно применена на всех окружениях.

## Migration 013: Add all missing contract fields (2025-11-22)

### Цель
Добавление всех недостающих полей в таблицу `contracts` для полного соответствия модели Contract.

### Проблема
На продакшене отсутствовало 20 колонок из модели Contract:
- Поля организаций (legal_address, postal_address, ogrn, okpo, director, based_on)
- Банковские реквизиты (bank_name, bank_bik, bank_account, correspondent_account, recipient)
- Поля физических лиц (passport_*, registration_address, actual_address, snils, ogrn_ip)

### Изменения
Добавлены все 20 недостающих колонок в:
- `public.contracts`
- `tenant_default.contracts`
- `tenant_186.contracts`

### Применено
- ✅ Локально: 2025-11-22
- ✅ Продакшен: 2025-11-22

### Результат
Теперь таблица contracts содержит все 58 колонок, полностью соответствует модели Go.

### Статус
Успешно применена. Создание договоров работает корректно.

## Migration 014: Add missing objects fields (2025-11-22)

### Цель
Добавление недостающих полей в таблицу `objects` для полного соответствия локальной БД.

### Проблема
На продакшене в таблице `objects` отсутствовало 4 колонки:
- `company_id` - ID компании-владельца
- `external_account_id` - ID внешнего аккаунта
- `external_account_name` - название внешнего аккаунта
- `source` - источник объекта (local, wialon, etc.)

### Изменения
Добавлены 4 недостающие колонки + индекс для company_id в:
- `public.objects`
- `tenant_default.objects`
- `tenant_186.objects`

### Применено
- ✅ Локально: 2025-11-22
- ✅ Продакшен: 2025-11-22

### Результат
Теперь таблица objects содержит все 37 колонок, полностью синхронизирована.

### Статус
Успешно применена. Все таблицы синхронизированы на 100%.
