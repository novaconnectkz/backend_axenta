DO $$
DECLARE
  s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'axenta_account_snapshots'
  LOOP
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_axenta_snap_created_at_desc', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_axenta_snap_account_type', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_axenta_snap_account_name_lower', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_axenta_snap_active_created', s);
  END LOOP;
END $$;
