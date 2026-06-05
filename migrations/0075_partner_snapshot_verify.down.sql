-- Откат Ф1 billing-gate колонок партнёрских снимков во всех схемах.
DO $$
DECLARE
  s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'partner_daily_snapshots'
  LOOP
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_partner_snapshots_verify_date', s);
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_partner_snapshots_verify', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots DROP COLUMN IF EXISTS approved_by', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots DROP COLUMN IF EXISTS verified_at', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots DROP COLUMN IF EXISTS verify_notes', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots DROP COLUMN IF EXISTS is_estimated', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots DROP COLUMN IF EXISTS amount_at_risk', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots DROP COLUMN IF EXISTS delta_pct', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots DROP COLUMN IF EXISTS prev_active_count', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots DROP COLUMN IF EXISTS verify_secondary_count', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots DROP COLUMN IF EXISTS verify_status', s);
  END LOOP;
END $$;
