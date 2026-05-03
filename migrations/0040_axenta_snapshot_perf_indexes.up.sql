-- Индексы для ускорения чтения axenta_account_snapshots в read-path /api/auth/accounts.
-- Применяется во ВСЕХ схемах где есть таблица (public, tenant_*).
-- Без этих индексов ORDER BY created_at DESC + LIMIT/OFFSET даёт seq-scan на 5K+ записей (~1s+).

DO $$
DECLARE
  s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'axenta_account_snapshots'
  LOOP
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_axenta_snap_created_at_desc ON %I.axenta_account_snapshots (created_at DESC) WHERE deleted_at IS NULL', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_axenta_snap_account_type ON %I.axenta_account_snapshots (account_type) WHERE deleted_at IS NULL', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_axenta_snap_account_name_lower ON %I.axenta_account_snapshots (LOWER(account_name)) WHERE deleted_at IS NULL', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_axenta_snap_active_created ON %I.axenta_account_snapshots (is_active, created_at DESC) WHERE deleted_at IS NULL', s);
    RAISE NOTICE 'indexes created in schema: %', s;
  END LOOP;
END $$;
