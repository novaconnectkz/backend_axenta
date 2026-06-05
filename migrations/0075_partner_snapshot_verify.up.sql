-- Ф1 billing-gate: двойная сверка партнёрских снимков.
-- partner_daily_snapshots зарегистрирована IsGlobal:false → реальные данные живут в
-- tenant_*-схемах (public-копия исторически протухла, 2 строки 2025-11). Колонки нужны
-- во ВСЕХ схемах где таблица существует (public + tenant_*), поэтому цикл по pg_tables.
-- GORM AutoMigrate для тенантных таблиц новые колонки на рестарте НЕ добавляет.
--
-- Семантика verify_status (forward-only, как и весь billing):
--   verified        — прошёл continuity-guard + cross-source сверку (или нет baseline)
--   needs_review    — расхождение/обнуление/скачок > порога ИЛИ источник дал warning
--   estimated       — backfill прошлого дня текущим состоянием (нет истории у провайдера)
--   manual_approved — оператор подтвердил needs_review/estimated вручную (audit trail)
-- DEFAULT 'verified' — grandfather существующих строк (они = текущий источник истины),
-- guard понижает статус ТОЛЬКО при детекте проблемы. Живого авто-charge партнёров нет
-- (0 инвойсов/проводок), поэтому fail-open безопасен; гейт превентивный.

DO $$
DECLARE
  s TEXT;
BEGIN
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'partner_daily_snapshots'
  LOOP
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots ADD COLUMN IF NOT EXISTS verify_status VARCHAR(20) NOT NULL DEFAULT ''verified''', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots ADD COLUMN IF NOT EXISTS verify_secondary_count INTEGER NOT NULL DEFAULT -1', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots ADD COLUMN IF NOT EXISTS prev_active_count INTEGER NOT NULL DEFAULT -1', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots ADD COLUMN IF NOT EXISTS delta_pct NUMERIC(7,2) NOT NULL DEFAULT 0', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots ADD COLUMN IF NOT EXISTS amount_at_risk NUMERIC(14,2) NOT NULL DEFAULT 0', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots ADD COLUMN IF NOT EXISTS is_estimated BOOLEAN NOT NULL DEFAULT false', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots ADD COLUMN IF NOT EXISTS verify_notes TEXT', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots ADD COLUMN IF NOT EXISTS verified_at TIMESTAMP WITH TIME ZONE', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots ADD COLUMN IF NOT EXISTS approved_by INTEGER', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_partner_snapshots_verify ON %I.partner_daily_snapshots (verify_status)', s);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_partner_snapshots_verify_date ON %I.partner_daily_snapshots (verify_status, snapshot_date)', s);
  END LOOP;
END $$;
