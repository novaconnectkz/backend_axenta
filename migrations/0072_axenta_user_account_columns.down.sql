DO $$
DECLARE
  s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'axenta_user_snapshots'
  LOOP
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_aus_user_account_id', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_aus_owner_account_name', s);
    EXECUTE format('ALTER TABLE %I.axenta_user_snapshots DROP COLUMN IF EXISTS user_account_id', s);
    EXECUTE format('ALTER TABLE %I.axenta_user_snapshots DROP COLUMN IF EXISTS owner_account_name', s);
  END LOOP;
END $$;
