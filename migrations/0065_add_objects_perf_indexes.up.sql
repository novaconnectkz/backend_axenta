-- Индексы для быстрых read-path запросов /objects, /objects/stats, /unified/objects.
-- Цель: ускорить COUNT FILTER и поиск по snapshot/units.

-- ============================================================
-- axenta_object_snapshots
-- ============================================================

-- Композитный индекс для COUNT FILTER в /objects/stats:
-- WHERE deleted_at IS NULL AND scheduled_delete_at IS NULL AND axenta_deleted_at IS NULL AND is_active = ...
CREATE INDEX IF NOT EXISTS idx_aos_active_filter
    ON axenta_object_snapshots (is_active)
    WHERE deleted_at IS NULL AND scheduled_delete_at IS NULL AND axenta_deleted_at IS NULL;

-- Корзина: axenta_deleted_at IS NOT NULL
CREATE INDEX IF NOT EXISTS idx_aos_deleted_partial
    ON axenta_object_snapshots (axenta_deleted_at)
    WHERE axenta_deleted_at IS NOT NULL;

-- К удалению: scheduled_delete_at IS NOT NULL
CREATE INDEX IF NOT EXISTS idx_aos_scheduled_partial
    ON axenta_object_snapshots (scheduled_delete_at)
    WHERE scheduled_delete_at IS NOT NULL;

-- Поиск по object_name (LIKE %...%) — подойдёт trigram
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS idx_aos_object_name_trgm
    ON axenta_object_snapshots USING GIN (LOWER(object_name) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_aos_account_name_trgm
    ON axenta_object_snapshots USING GIN (LOWER(account_name) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_aos_unique_id_trgm
    ON axenta_object_snapshots USING GIN (LOWER(unique_id) gin_trgm_ops);

-- Сортировка по object_name (без поиска)
CREATE INDEX IF NOT EXISTS idx_aos_object_name_sort
    ON axenta_object_snapshots (object_name)
    WHERE deleted_at IS NULL AND scheduled_delete_at IS NULL AND axenta_deleted_at IS NULL;

-- Сортировка по последнему сообщению
CREATE INDEX IF NOT EXISTS idx_aos_last_communication_at
    ON axenta_object_snapshots (last_communication_at DESC NULLS LAST);

-- ============================================================
-- wialon_units
-- ============================================================

-- Поиск по name (для /unified/objects, search)
CREATE INDEX IF NOT EXISTS idx_wialon_units_name_trgm
    ON wialon_units USING GIN (LOWER(name) gin_trgm_ops);

-- Сортировка по name внутри подключения (используется в /wialon/all-units)
CREATE INDEX IF NOT EXISTS idx_wialon_units_conn_name
    ON wialon_units (connection_id, name);
