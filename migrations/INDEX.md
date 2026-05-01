# Migrations — Inventory

Этот файл — единая точка входа в SQL-миграции ACRM. Все актуальные миграции живут в `migrations/`. Папка `database/migrations/` упразднена.

## Как применяются миграции (текущее состояние)

| Слой | Что делает |
|---|---|
| **GORM AutoMigrate** | Запускается при старте сервера. Создаёт/обновляет колонки из Go-структур моделей. Это основной механизм поддержания схемы. |
| **`cmd/migrate/main.go`** | CLI-обёртка над GORM AutoMigrate. SQL-файлы НЕ применяет. |
| **`apply_*.sh`** в этой папке | Прямой `psql -f file.sql`. Запускаются вручную для миграций, требующих UPDATE/INSERT данных или специфичных индексов. |
| **`database/materialized_views.go`** | Применяет `002_create_materialized_views.sql` через Go при старте. |

**Будущее (Phase B)**: переход на `golang-migrate` с таблицей `schema_migrations`. См. документ `acrm_migrations_baseline_2026-05-01.md` в Brain.

## Список миграций

### По номерам (`NNN_*`)

| Файл | Описание |
|---|---|
| `000010_make_contract_dates_nullable.sql` | Контрактные даты nullable |
| `000022_create_audit_logs.up.sql` | Создание audit_logs (с down-миграцией) |
| `000022_create_audit_logs.down.sql` | Откат audit_logs |
| `002_create_materialized_views.sql` | Materialized views для биллинга. **Применяется через Go** (`database/materialized_views.go`) |
| `003_add_vat_rate_fields.sql` | НДС поля (старая версия) |
| `004_create_system_settings.sql` | Таблица system_settings (TIMESTAMPTZ, public schema) |
| `005_fix_subscriptions_columns.sql` | Фикс колонок subscriptions |
| `006_add_sync_columns.sql` | Колонки синхронизации |
| `007_add_sync_columns_to_tenants.sql` | Sync-колонки для tenant-схем |
| `008_add_max_messenger_columns.sql` | Колонки MAX мессенджера (`max_bot_token` etc) |
| `009_add_sequential_numbers.sql` | Последовательные номера документов |
| `010_add_invoice_notification_columns.sql` | Колонки уведомлений по счетам |
| `011_fix_invoice_notification_column_types.sql` | Фикс типов колонок уведомлений |
| `012_add_missing_contract_fields.sql` | Добавление недостающих полей contracts |
| `013_add_all_missing_contract_fields.sql` | Расширение полей contracts |
| `014_add_missing_objects_fields.sql` | Поля объектов |
| `015_sync_production_schema.sql` | **Большой sync с прод-схемой** (~20KB) |
| `016_add_autopilot_enabled.sql` | Поле autopilot_enabled (default true, с UPDATE existing) |
| `017_add_vat_rate_fields.sql` | НДС поля v2 |
| `017_create_default_contract_numerators.sql` | Дефолтные нумераторы контрактов |
| `018_add_contract_client_fields.sql` | Поля клиента в contracts |
| `019_create_discounts_table.sql` | Таблица скидок |
| `020_add_subscription_id_to_contract_objects.sql` | subscription_id в contract_objects (hardcoded tenants) |
| `021_add_tax_fields_to_system_settings.sql` | Tax-поля в system_settings (минимальная версия) |
| `023_add_contract_type_fields.sql` | Тип контракта |
| `024_add_company_type.sql` | Тип компании |
| `024_increase_client_type_size.sql` | Размер типа клиента |
| `025_add_partner_daily_snapshots.sql` | Партнёрские дневные снимки |
| `026_update_partner_snapshots_precision.sql` | Точность снимков |
| `027_create_axenta_snapshots_tables.sql` | Таблицы Axenta snapshots |
| `028_create_snapshot_jobs_table.sql` | Таблица snapshot jobs |
| `029_fix_partner_snapshot_unique_index.sql` | Уникальный индекс снимков |
| `030_create_public_sync_tables.sql` | Public sync-таблицы (~12KB) |

### По датам (`YYYYMMDD_*`)

| Файл | Описание |
|---|---|
| `20251108_add_integration_sync_columns.sql` | Sync-колонки интеграций |
| `20251108_add_sync_metadata_entities.sql` | Sync metadata (~10KB) |
| `20251108_adjust_object_constraints.sql` | Ограничения объектов |
| `20251108_create_integration_events.sql` | Таблица integration_events |
| `20251108_fix_sync_metadata.sql` | Фикс sync metadata |
| `20251108_seed_tenant_companies.sql` | Seed default company в tenant-схемах |
| `20251117_add_contract_auto_renew_fields.sql` | Auto-renew для контрактов |
| `20251117_make_end_date_nullable.sql` | end_date nullable |
| `20251117_make_objects_contract_id_nullable.sql` | objects.contract_id nullable |
| `20251117_make_start_date_nullable.sql` | start_date nullable |
| `20251117_make_tariff_plan_id_nullable.sql` | tariff_plan_id nullable |
| `20251118_add_contract_id_to_subscriptions.sql` | subscriptions.contract_id |
| `20251118_make_contracts_dates_nullable.sql` | contracts.dates nullable |
| `20251121_add_invoice_send_channels.sql` | Каналы отправки счетов |
| `20251123_add_autopilot_enabled.sql` | Autopilot (default false, без UPDATE existing) |
| `20251124_add_tax_fields_to_system_settings.sql` | Tax-поля + UPDATE existing |
| `20251124_add_tax_settings_to_companies.sql` | Tax-настройки компаний |
| `20251124_create_system_settings_table.sql` | Старая версия system_settings (TIMESTAMP без timezone) |
| `20251125_add_subscription_id_to_contract_objects.sql` | subscription_id (динамический по tenant_*) |
| `20251125_cleanup_invalid_subscription_ids.sql` | Очистка невалидных subscription_id |
| `20251125_fix_existing_subscriptions_objects.sql` | Фикс существующих привязок |
| `20251125_recalculate_contract_totals.sql` | Пересчёт итогов контрактов |
| `20251125_update_existing_contract_objects.sql` | Обновление существующих contract_objects |

### Без префикса (исторические)

| Файл | Описание |
|---|---|
| `add_axenta_user_fields.sql` | Поля пользователя Axenta |
| `add_client_short_name_to_contracts.sql` | Короткое имя клиента |
| `add_discount_fixed_columns.sql` | Колонки фиксированных скидок |
| `add_partner_objects_history.sql` | История объектов партнёров |
| `add_snapshot_jobs_table.sql` | Таблица snapshot jobs (дубль `028_*`?) |
| `delete_all_contracts.sql` | **Опасный**: удаление всех контрактов |
| `fix_billing_plans_unique_index.sql` | Фикс unique-индекса тарифных планов |
| `update_contracts_account_id.sql` | Обновление account_id в контрактах |

## Известные дубли

| Файл (date-prefix) | Файл (NNN-prefix) | Различие |
|---|---|---|
| `20251123_add_autopilot_enabled.sql` | `016_add_autopilot_enabled.sql` | default `false` vs `true` + UPDATE existing |
| `20251124_create_system_settings_table.sql` | `004_create_system_settings.sql` | TIMESTAMP vs TIMESTAMPTZ |
| `20251124_add_tax_fields_to_system_settings.sql` | `021_add_tax_fields_to_system_settings.sql` | UPDATE existing vs только ALTER |
| `20251125_add_subscription_id_to_contract_objects.sql` | `020_add_subscription_id_to_contract_objects.sql` | dynamic по `tenant_*` vs hardcoded 4 tenant |

Все используют `IF NOT EXISTS` — идемпотентны, повторное применение безопасно. Разрешение дублей отложено до Phase B (golang-migrate).

## Известные конфликты номеров

| Номер | Файл 1 | Файл 2 |
|---|---|---|
| `017` | `add_vat_rate_fields.sql` | `create_default_contract_numerators.sql` |
| `024` | `add_company_type.sql` | `increase_client_type_size.sql` |

## Применение

### Локально (dev)

```bash
# GORM AutoMigrate (структура из Go-моделей) — автоматом при старте сервера
make run

# CLI миграции (тоже AutoMigrate)
go run cmd/migrate/main.go

# Конкретный SQL вручную
psql -h localhost -U postgres -d axenta_db -f migrations/023_add_contract_type_fields.sql

# Готовые apply-скрипты
bash migrations/apply_all_tax_migrations.sh
bash migrations/apply_client_short_name_migration.sh
bash migrations/apply_contracts_migration.sh
bash migrations/apply_subscription_id_migration.sh
bash migrations/apply_tax_settings_migration.sh
```

### Продакшен

```bash
# Полный набор миграций
bash run_production_migrations.sh

# Принудительно (если упирается)
bash run_production_migrations_force.sh

# Проверка состояния
bash verify_migrations.sh
```

## Как добавлять новые миграции (текущая практика)

1. Создать SQL-файл в `migrations/` с именем `YYYYMMDD_<описание>.sql` (или `NNN_<описание>.sql`).
2. Использовать `IF NOT EXISTS`/`IF EXISTS` для идемпотентности.
3. Если меняется Go-модель — обновить структуру в `models/`, GORM AutoMigrate подхватит при старте.
4. Если требуется UPDATE/INSERT данных — использовать `apply_*.sh` либо запускать `psql -f` вручную на проде.
5. Закоммитить SQL **в git** (теперь они трекаются благодаря whitelist в `.gitignore`).

## После Phase B (`golang-migrate`)

После перехода на `golang-migrate` правила изменятся:
- Все SQL переименуются в `NNN_<name>.up.sql` + `NNN_<name>.down.sql`
- Применяться будут автоматически при старте сервера через `migrate.Up()`
- Учёт состояния — в таблице `schema_migrations`
- AutoMigrate — отключится

Текущий список (этот файл) станет историческим срезом.

## Связанные документы

- `MIGRATION_HISTORY.md` — исторические записи о применённых миграциях
- `MIGRATION_REPORT_2025-11-30.md` — отчёт от 30.11.2025
- `README.md` — общий обзор папки
- В Brain: `sessions/acrm_migrations_baseline_2026-05-01.md` — полный baseline перед Phase A+B
