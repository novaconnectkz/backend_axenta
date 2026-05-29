-- Фильтр «Наши родители» на /users требует знать аккаунт-владельца Axenta-юзера.
-- Axenta API отдаёт accountId/accountName per user, но snapshot их не хранил.
-- Добавляем колонки во ВСЕ тенантные axenta_user_snapshots (паттерн 0065/0067).
-- GORM AutoMigrate для тенантных таблиц новые колонки на рестарте НЕ добавляет —
-- поэтому явный цикл по pg_tables.

DO $$
DECLARE
  s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'axenta_user_snapshots'
  LOOP
    EXECUTE format('ALTER TABLE %I.axenta_user_snapshots ADD COLUMN IF NOT EXISTS user_account_id BIGINT', s);
    EXECUTE format('ALTER TABLE %I.axenta_user_snapshots ADD COLUMN IF NOT EXISTS owner_account_name VARCHAR(200)', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_aus_user_account_id ON %I.axenta_user_snapshots (user_account_id)', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_aus_owner_account_name ON %I.axenta_user_snapshots (owner_account_name)', s);
  END LOOP;
END $$;
