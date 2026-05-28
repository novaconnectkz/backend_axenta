-- WCRM→ACRM Axenta-договоры: инфраструктура идемпотентности миграции.
--
-- Создаёт:
--   1) partial UNIQUE indexes на external_id для tenant_186.contracts/contract_appendices
--      (ключ 'wcrm:contract:<id>' / 'wcrm:b.attachments:<id>') — защита от дублей при
--      повторном approve. Partial (WHERE LIKE 'wcrm:%' AND deleted_at IS NULL) чтобы не
--      конфликтовать с существующими external_id из 1С/Битрикс и не ломать soft-delete.
--   2) partial UNIQUE index на public.invoices.external_id (ключ 'wcrm:debt:<company>').
--   3) expression UNIQUE index на public.billing_history по metadata->>'wcrm_balance_company'
--      (у BillingHistory нет колонки external_id — идемпотентность через metadata).
--   4) tenant_186.wcrm_migration_state — аудит/состояние per-company approve.
--
-- ВНИМАНИЕ CONCURRENTLY: golang-migrate оборачивает каждую миграцию в транзакцию,
-- где CREATE INDEX CONCURRENTLY запрещён. Поэтому используем обычный CREATE UNIQUE INDEX
-- внутри DO-цикла. Таблицы маленькие (contracts ≤ сотни строк на тенант), краткая
-- блокировка приемлема. Дубли external_id перед созданием индекса не ожидаются
-- (ключи 'wcrm:%' новые), но IF NOT EXISTS защищает повторный прогон.
--
-- Scope схем: применяем к ВСЕМ tenant_%-схемам где есть таблица (не только tenant_186),
-- чтобы миграция была переносима и покрывала staging/неактивные тенанты. Целевой импорт
-- идёт в tenant_186, но индексы безвредны везде.

DO $$
DECLARE s TEXT;
BEGIN
  -- contracts.external_id partial unique во всех схемах с таблицей
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'contracts'
  LOOP
    EXECUTE format(
      'CREATE UNIQUE INDEX IF NOT EXISTS uq_contracts_wcrm_extid ON %I.contracts (external_id) WHERE external_id LIKE ''wcrm:contract:%%'' AND deleted_at IS NULL',
      s);
  END LOOP;

  -- contract_appendices.external_id partial unique
  FOR s IN SELECT schemaname FROM pg_tables WHERE tablename = 'contract_appendices'
  LOOP
    EXECUTE format(
      'CREATE UNIQUE INDEX IF NOT EXISTS uq_appendices_wcrm_extid ON %I.contract_appendices (external_id) WHERE external_id LIKE ''wcrm:b.attachments:%%'' AND deleted_at IS NULL',
      s);
  END LOOP;
END $$;

-- public.invoices.external_id partial unique (балансовые долги)
CREATE UNIQUE INDEX IF NOT EXISTS uq_invoices_wcrm_extid
  ON public.invoices (external_id)
  WHERE external_id LIKE 'wcrm:debt:%' AND deleted_at IS NULL;

-- public.billing_history идемпотентность по metadata (нет колонки external_id).
-- Ключ: предоплата per WCRM-компания. metadata->>'source'='wcrm_balance'.
CREATE UNIQUE INDEX IF NOT EXISTS uq_billhist_wcrm_balance
  ON public.billing_history ((metadata::jsonb ->> 'wcrm_balance_company'))
  WHERE (metadata::jsonb ->> 'source') = 'wcrm_balance' AND deleted_at IS NULL;

-- Состояние миграции per-company (в tenant_186 и любых tenant_%-схемах).
DO $$
DECLARE s TEXT;
BEGIN
  FOR s IN SELECT schema_name FROM information_schema.schemata
           WHERE schema_name LIKE 'tenant_%' AND schema_name ~ '^tenant_[a-zA-Z0-9_]+$'
  LOOP
    EXECUTE format($f$
      CREATE TABLE IF NOT EXISTS %I.wcrm_migration_state (
        wcrm_company_id      BIGINT PRIMARY KEY,
        status               VARCHAR(20) NOT NULL DEFAULT 'pending',
        snapshot_sha256      VARCHAR(64) NOT NULL DEFAULT '',
        client_type_override VARCHAR(40),
        approved_by          VARCHAR(100),
        approved_at          TIMESTAMPTZ,
        created_contract_ids JSONB,
        created_appendix_ids JSONB,
        created_billing_ids  JSONB,
        created_invoice_ids  JSONB,
        error                TEXT,
        created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
        updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
      )$f$, s);
    -- Таблица создаётся под postgres (CREATE INDEX требует ownership), но
    -- приложение ходит под axenta_user — выдаём права явно.
    EXECUTE format('GRANT ALL ON TABLE %I.wcrm_migration_state TO axenta_user', s);
  END LOOP;
END $$;
