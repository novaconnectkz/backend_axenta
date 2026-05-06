-- Откат индексов производительности /objects.
-- axenta_object_snapshots — тенантная (цикл), wialon_units — public.

DO $$
DECLARE
  s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'axenta_object_snapshots'
  LOOP
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_aos_active_filter', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_aos_deleted_partial', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_aos_scheduled_partial', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_aos_object_name_trgm', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_aos_account_name_trgm', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_aos_unique_id_trgm', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_aos_object_name_sort', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_aos_last_communication_at', s);
  END LOOP;
END $$;

DROP INDEX IF EXISTS idx_wialon_units_name_trgm;
DROP INDEX IF EXISTS idx_wialon_units_conn_name;

-- pg_trgm не дропаем — может использоваться другими таблицами.
-- axenta_deleted_at колонку не дропаем — это no-op rollback (колонка не наша по семантике).
