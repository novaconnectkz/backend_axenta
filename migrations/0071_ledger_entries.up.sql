-- Лицевой счёт: неизменяемые проводки (ledger_entries) в public.
-- Баланс договора = SUM(amount). payment(+)/charge(−). Источник правды — эта таблица.

CREATE TABLE IF NOT EXISTS public.ledger_entries (
    id               BIGSERIAL PRIMARY KEY,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at       TIMESTAMPTZ,
    admin_account_id BIGINT NOT NULL,
    company_id       BIGINT NOT NULL,
    contract_id      BIGINT NOT NULL,
    subscription_id  BIGINT,
    object_id        BIGINT,
    entry_type       VARCHAR(30) NOT NULL,            -- charge|payment|adjustment|reversal|migration_balance
    amount           DECIMAL(15,2) NOT NULL,          -- знаковый: payment(+), charge(−)
    currency         VARCHAR(3) NOT NULL DEFAULT 'RUB',
    source           VARCHAR(30) NOT NULL,            -- manual|excel|1c|payment_system|bank|migration|auto_charge
    external_id      VARCHAR(128),
    description      TEXT,
    entry_date       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by       VARCHAR(100),
    reversal_of_id   BIGINT,
    metadata         JSONB
);

CREATE INDEX IF NOT EXISTS idx_ledger_contract     ON public.ledger_entries (contract_id, entry_date) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ledger_company      ON public.ledger_entries (company_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ledger_type         ON public.ledger_entries (entry_type);

-- Идемпотентность импорта (Codex): по (admin, company, source, external_id), а не голый external_id.
-- Банковские fingerprint-дубли (пустой external_id) отсекаются на уровне импортёра.
CREATE UNIQUE INDEX IF NOT EXISTS uq_ledger_import_extid
    ON public.ledger_entries (admin_account_id, company_id, source, external_id)
    WHERE external_id IS NOT NULL AND external_id <> '' AND deleted_at IS NULL;
