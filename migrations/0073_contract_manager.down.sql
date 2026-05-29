DO $$
DECLARE
  s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables
           WHERE tablename = 'contracts'
             AND schemaname LIKE 'tenant_%'
  LOOP
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_contracts_manager_id', s);
    EXECUTE format('ALTER TABLE %I.contracts DROP COLUMN IF EXISTS manager_id', s);
    EXECUTE format('ALTER TABLE %I.contracts DROP COLUMN IF EXISTS manager_name', s);
  END LOOP;
END $$;
