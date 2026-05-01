CREATE TABLE IF NOT EXISTS integration_events (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    integration_type VARCHAR(50) NOT NULL,
    company_id BIGINT NOT NULL,
    level VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    details JSONB,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_integration_events_type ON integration_events(integration_type);
CREATE INDEX IF NOT EXISTS idx_integration_events_company ON integration_events(company_id);
CREATE INDEX IF NOT EXISTS idx_integration_events_occurred_at ON integration_events(occurred_at DESC);

