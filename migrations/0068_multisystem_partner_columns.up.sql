-- Ф0 мульти-системный partner billing: колонки источника партнёра + нормализация
-- уникального индекса partner_daily_snapshots по ВСЕМ схемам.
--
-- Зачем DO $$ цикл по pg_tables: golang-migrate ходит по public search_path, а
-- таблицы тенантные (в каждой схеме). Историческое расхождение индекса
-- (tenant_186=(partner_company_id,snapshot_date) vs public/прочие=(snapshot_date)
-- vs 0027 (contract_id,snapshot_date)) нормализуем здесь принудительно для всех.
--
-- Новый составной unique = (partner_source, connection_id, partner_external_id,
-- contract_id, snapshot_date). NON-partial — соответствует GORM-тегу модели
-- (GORM не умеет WHERE deleted_at); данные чистые (0 soft-deleted дублей).
--
-- Self-contained (ADD COLUMN IF NOT EXISTS): покрывает и НЕактивные тенант-схемы,
-- которые AutoMigrate пропускает (RunAllMigrations: companies WHERE is_active).
-- Backfill external_id ДО создания индекса — иначе orphan-строки (contract_id=0)
-- с external_id='' схлопнулись бы по составному ключу.

DO $$
DECLARE s TEXT;
BEGIN
  -- partner_daily_snapshots во всех схемах
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'partner_daily_snapshots'
  LOOP
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots ADD COLUMN IF NOT EXISTS partner_source varchar(20) NOT NULL DEFAULT ''axenta''', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots ADD COLUMN IF NOT EXISTS connection_id bigint NOT NULL DEFAULT 0', s);
    EXECUTE format('ALTER TABLE %I.partner_daily_snapshots ADD COLUMN IF NOT EXISTS partner_external_id varchar(128) NOT NULL DEFAULT ''''', s);

    -- Backfill: существующие снимки = Axenta, external_id = partner_company_id
    EXECUTE format('UPDATE %I.partner_daily_snapshots SET partner_external_id = partner_company_id::text WHERE partner_external_id = '''' OR partner_external_id IS NULL', s);
    EXECUTE format('UPDATE %I.partner_daily_snapshots SET partner_source = ''axenta'' WHERE partner_source IS NULL OR partner_source = ''''', s);

    -- Нормализация уникального индекса
    EXECUTE format('DROP INDEX IF EXISTS %I.idx_partner_snapshot_unique', s);
    EXECUTE format('CREATE UNIQUE INDEX idx_partner_snapshot_unique ON %I.partner_daily_snapshots (partner_source, connection_id, partner_external_id, contract_id, snapshot_date)', s);
    RAISE NOTICE 'partner_daily_snapshots normalized in schema: %', s;
  END LOOP;

  -- contracts во всех схемах. Фильтр по information_schema.columns (а не pg_tables):
  -- существует стрэй public.contracts БЕЗ колонки contract_type, который иначе валит
  -- UPDATE ниже и откатывает весь DO-блок.
  FOR s IN SELECT table_schema FROM information_schema.columns
           WHERE table_name = 'contracts' AND column_name = 'contract_type'
  LOOP
    EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS partner_source varchar(20) NOT NULL DEFAULT ''axenta''', s);
    EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS partner_connection_id bigint NOT NULL DEFAULT 0', s);
    EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS partner_external_id varchar(128) NOT NULL DEFAULT ''''', s);

    -- Backfill: существующие партнёрские договоры = Axenta, external_id = partner_company_id
    EXECUTE format('UPDATE %I.contracts SET partner_external_id = partner_company_id::text WHERE contract_type = ''partner'' AND partner_company_id IS NOT NULL AND (partner_external_id = '''' OR partner_external_id IS NULL)', s);
    RAISE NOTICE 'contracts extended in schema: %', s;
  END LOOP;
END $$;
