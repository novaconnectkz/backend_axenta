-- Откат Ф0 мульти-системных колонок партнёра. Best-effort: восстанавливаем
-- индекс к (partner_company_id, snapshot_date) и убираем новые колонки.

DO $$
DECLARE s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'partner_daily_snapshots'
  LOOP
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_partner_snapshot_unique', s);
    EXECUTE format('CREATE UNIQUE INDEX idx_partner_snapshot_unique ON %I.partner_daily_snapshots (partner_company_id, snapshot_date)', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots DROP COLUMN IF EXISTS partner_source', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots DROP COLUMN IF EXISTS connection_id', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots DROP COLUMN IF EXISTS partner_external_id', s);
  END LOOP;

  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'contracts'
  LOOP
    EXECUTE format('ALTER TABLE %I.contracts DROP COLUMN IF EXISTS partner_source', s);
    EXECUTE format('ALTER TABLE %I.contracts DROP COLUMN IF EXISTS partner_connection_id', s);
    EXECUTE format('ALTER TABLE %I.contracts DROP COLUMN IF EXISTS partner_external_id', s);
  END LOOP;
END $$;
