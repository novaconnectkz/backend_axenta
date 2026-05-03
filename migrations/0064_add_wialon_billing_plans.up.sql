-- Кэш тарифных планов Wialon. Раз в час фоновый scheduler синхронизирует с Wialon API
-- (account/get_billing_plans). Endpoint /api/wialon/connections/:id/billing-plans отдаёт
-- из этой таблицы мгновенно. ?force_refresh=true триггерит синхронный sync.

CREATE TABLE IF NOT EXISTS wialon_billing_plans (
    id BIGSERIAL PRIMARY KEY,

    connection_id INTEGER NOT NULL REFERENCES wialon_connections(id) ON DELETE CASCADE,
    plan_name     VARCHAR(255) NOT NULL,
    raw_payload   JSONB,

    last_synced_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (connection_id, plan_name)
);

CREATE INDEX IF NOT EXISTS idx_wialon_billing_plans_connection ON wialon_billing_plans(connection_id);
CREATE INDEX IF NOT EXISTS idx_wialon_billing_plans_synced     ON wialon_billing_plans(last_synced_at);

COMMENT ON TABLE  wialon_billing_plans IS 'Кэш тарифных планов Wialon (фоновый sync раз в час)';
COMMENT ON COLUMN wialon_billing_plans.raw_payload IS 'Полный объект плана из Wialon (parent, services, denyBalance, etc.)';
