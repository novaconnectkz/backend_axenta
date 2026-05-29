-- Постоплата (П2): режим биллинга договора + кредит-лимит.
-- contracts — тенантная таблица: AutoMigrate новые колонки на рестарте НЕ добавляет,
-- поэтому явный цикл по pg_tables (только tenant_% схемы, паттерн 0073).

DO $$
DECLARE
  s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables
           WHERE tablename = 'contracts'
             AND schemaname LIKE 'tenant_%'
  LOOP
    EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS billing_mode VARCHAR(20) DEFAULT ''prepaid''', s);
    EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS credit_limit DECIMAL(15,2) DEFAULT 0', s);
  END LOOP;
END $$;
