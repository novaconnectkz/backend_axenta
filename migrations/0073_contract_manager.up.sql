-- Привязка договора к обслуживающему менеджеру (local_users.id) + денорм-имя.
-- Менеджер видит только свои договоры (scoping), admin назначает.
-- contracts — тенантная таблица: GORM AutoMigrate новые колонки на рестарте НЕ
-- добавляет, поэтому явный цикл по pg_tables (паттерн 0072).

DO $$
DECLARE
  s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables
           WHERE tablename = 'contracts'
             AND schemaname LIKE 'tenant_%'
  LOOP
    EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS manager_id BIGINT', s);
    EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS manager_name VARCHAR(200)', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_contracts_manager_id ON %I.contracts (manager_id)', s);
  END LOOP;
END $$;
