-- Откат инфраструктуры идемпотентности WCRM-миграции.

DROP INDEX IF EXISTS public.uq_invoices_wcrm_extid;
DROP INDEX IF EXISTS public.uq_billhist_wcrm_balance;

DO $$
DECLARE s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'contracts'
  LOOP
    EXECUTE format('DROP INDEX IF EXISTS %I.uq_contracts_wcrm_extid', s);
  END LOOP;
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'contract_appendices'
  LOOP
    EXECUTE format('DROP INDEX IF EXISTS %I.uq_appendices_wcrm_extid', s);
  END LOOP;
  FOR s IN SELECT schema_name FROM information_schema.schemata
           WHERE schema_name LIKE 'tenant_%' AND schema_name ~ '^tenant_[a-zA-Z0-9_]+$'
  LOOP
    EXECUTE format('DROP TABLE IF EXISTS %I.wcrm_migration_state', s);
  END LOOP;
END $$;
