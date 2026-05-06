-- Индексы для быстрых read-path запросов /objects, /objects/stats, /unified/objects.
-- Цель: ускорить COUNT FILTER и поиск по snapshot/units.
--
-- ВАЖНО: axenta_object_snapshots — тенантная таблица (живёт в tenant_* схемах, не в public).
-- Применяем индексы во ВСЕХ схемах через цикл по pg_tables (паттерн 0067).
-- Колонка axenta_deleted_at добавлена через GORM AutoMigrate, на проде до этой миграции
-- её может не быть — добавляем ALTER TABLE ADD COLUMN IF NOT EXISTS перед индексами.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ============================================================
-- axenta_object_snapshots (тенантная — цикл по схемам)
-- ============================================================

DO $$
DECLARE
  s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'axenta_object_snapshots'
  LOOP
    -- Гарантируем наличие колонки axenta_deleted_at (добавлена позже через GORM)
    EXECUTE format('ALTER TABLE %I.axenta_object_snapshots ADD COLUMN IF NOT EXISTS axenta_deleted_at TIMESTAMP', s);

    -- Композитный индекс для COUNT FILTER в /objects/stats
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_aos_active_filter ON %I.axenta_object_snapshots (is_active) WHERE deleted_at IS NULL AND scheduled_delete_at IS NULL AND axenta_deleted_at IS NULL', s);

    -- Корзина: axenta_deleted_at IS NOT NULL
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_aos_deleted_partial ON %I.axenta_object_snapshots (axenta_deleted_at) WHERE axenta_deleted_at IS NOT NULL', s);

    -- К удалению: scheduled_delete_at IS NOT NULL
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_aos_scheduled_partial ON %I.axenta_object_snapshots (scheduled_delete_at) WHERE scheduled_delete_at IS NOT NULL', s);

    -- Trigram-поиск (LIKE %...%)
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_aos_object_name_trgm ON %I.axenta_object_snapshots USING GIN (LOWER(object_name) gin_trgm_ops)', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_aos_account_name_trgm ON %I.axenta_object_snapshots USING GIN (LOWER(account_name) gin_trgm_ops)', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_aos_unique_id_trgm ON %I.axenta_object_snapshots USING GIN (LOWER(unique_id) gin_trgm_ops)', s);

    -- Сортировка по object_name (без поиска)
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_aos_object_name_sort ON %I.axenta_object_snapshots (object_name) WHERE deleted_at IS NULL AND scheduled_delete_at IS NULL AND axenta_deleted_at IS NULL', s);

    -- Сортировка по последнему сообщению
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_aos_last_communication_at ON %I.axenta_object_snapshots (last_communication_at DESC NULLS LAST)', s);

    RAISE NOTICE 'objects perf indexes created in schema: %', s;
  END LOOP;
END $$;

-- ============================================================
-- wialon_units (глобальная, public-схема)
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_wialon_units_name_trgm
    ON wialon_units USING GIN (LOWER(name) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_wialon_units_conn_name
    ON wialon_units (connection_id, name);
