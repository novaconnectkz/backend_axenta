-- Откат индексов производительности /objects.

DROP INDEX IF EXISTS idx_aos_active_filter;
DROP INDEX IF EXISTS idx_aos_deleted_partial;
DROP INDEX IF EXISTS idx_aos_scheduled_partial;
DROP INDEX IF EXISTS idx_aos_object_name_trgm;
DROP INDEX IF EXISTS idx_aos_account_name_trgm;
DROP INDEX IF EXISTS idx_aos_unique_id_trgm;
DROP INDEX IF EXISTS idx_aos_object_name_sort;
DROP INDEX IF EXISTS idx_aos_last_communication_at;
DROP INDEX IF EXISTS idx_wialon_units_name_trgm;
DROP INDEX IF EXISTS idx_wialon_units_conn_name;

-- pg_trgm не дропаем — может использоваться другими таблицами.
