DO $$
DECLARE
  s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables
           WHERE tablename = 'contracts'
             AND schemaname LIKE 'tenant_%'
  LOOP
    EXECUTE format('ALTER TABLE %I.contracts DROP COLUMN IF EXISTS billing_mode', s);
    EXECUTE format('ALTER TABLE %I.contracts DROP COLUMN IF EXISTS credit_limit', s);
  END LOOP;
END $$;
