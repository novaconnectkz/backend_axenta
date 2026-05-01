DO $$
DECLARE
    tenant RECORD;
    has_users BOOLEAN;
    has_objects BOOLEAN;
    has_contracts BOOLEAN;
BEGIN
    FOR tenant IN
        SELECT schema_name
        FROM information_schema.schemata
        WHERE schema_name LIKE 'tenant_%'
    LOOP
        SELECT EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_schema = tenant.schema_name AND table_name = 'users'
        ) INTO has_users;

        IF has_users THEN
            EXECUTE format('ALTER TABLE %I.users ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.users ADD COLUMN IF NOT EXISTS sync_status VARCHAR(20) DEFAULT %L', tenant.schema_name, 'idle');
            EXECUTE format('ALTER TABLE %I.users ADD COLUMN IF NOT EXISTS sync_version BIGINT DEFAULT 0', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.users ADD COLUMN IF NOT EXISTS sync_checksum TEXT', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.users ADD COLUMN IF NOT EXISTS is_dirty BOOLEAN DEFAULT FALSE', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.users ADD COLUMN IF NOT EXISTS sync_error TEXT', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.users ADD COLUMN IF NOT EXISTS source_of_truth VARCHAR(20) DEFAULT %L', tenant.schema_name, 'local');
            EXECUTE format('ALTER TABLE %I.users ADD COLUMN IF NOT EXISTS sync_queued_at TIMESTAMPTZ', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.users ADD COLUMN IF NOT EXISTS sync_attempted_at TIMESTAMPTZ', tenant.schema_name);
        END IF;

        SELECT EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_schema = tenant.schema_name AND table_name = 'objects'
        ) INTO has_objects;

        IF has_objects THEN
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS sync_status VARCHAR(20) DEFAULT %L', tenant.schema_name, 'idle');
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS sync_version BIGINT DEFAULT 0', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS sync_checksum TEXT', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS is_dirty BOOLEAN DEFAULT FALSE', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS sync_error TEXT', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS source_of_truth VARCHAR(20) DEFAULT %L', tenant.schema_name, 'local');
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS sync_queued_at TIMESTAMPTZ', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS sync_attempted_at TIMESTAMPTZ', tenant.schema_name);
        END IF;

        SELECT EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_schema = tenant.schema_name AND table_name = 'contracts'
        ) INTO has_contracts;

        IF has_contracts THEN
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS sync_status VARCHAR(20) DEFAULT %L', tenant.schema_name, 'idle');
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS sync_version BIGINT DEFAULT 0', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS sync_checksum TEXT', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS is_dirty BOOLEAN DEFAULT FALSE', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS sync_error TEXT', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS source_of_truth VARCHAR(20) DEFAULT %L', tenant.schema_name, 'local');
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS sync_queued_at TIMESTAMPTZ', tenant.schema_name);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS sync_attempted_at TIMESTAMPTZ', tenant.schema_name);
        END IF;

        EXECUTE format('CREATE TABLE IF NOT EXISTS %I.integration_mappings (
            id BIGSERIAL PRIMARY KEY,
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            deleted_at TIMESTAMPTZ,
            company_id BIGINT NOT NULL,
            entity_type VARCHAR(50) NOT NULL,
            local_id BIGINT NOT NULL,
            remote_id VARCHAR(100) NOT NULL,
            remote_hash TEXT,
            payload_hash TEXT,
            metadata JSONB,
            last_synced_at TIMESTAMPTZ,
            sync_status VARCHAR(20) DEFAULT %L,
            sync_error TEXT,
            source_of_truth VARCHAR(20) DEFAULT %L,
            sync_queued_at TIMESTAMPTZ,
            sync_attempted_at TIMESTAMPTZ
        )', tenant.schema_name, 'idle', 'local');

        EXECUTE format('CREATE UNIQUE INDEX IF NOT EXISTS %I ON %I.integration_mappings (company_id, entity_type, local_id)', tenant.schema_name || '_integration_mappings_local_udx', tenant.schema_name);
        EXECUTE format('CREATE UNIQUE INDEX IF NOT EXISTS %I ON %I.integration_mappings (company_id, entity_type, remote_id)', tenant.schema_name || '_integration_mappings_remote_udx', tenant.schema_name);
        EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I.integration_mappings (sync_status, last_synced_at)', tenant.schema_name || '_integration_mappings_status_idx', tenant.schema_name);
    END LOOP;
END $$;
