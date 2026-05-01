-- Migration: Add sync metadata columns to tenant schemas
-- Created: 2025-11-21
-- Description: Adds synchronization metadata columns for contracts, objects, and users in all tenant schemas

DO $$
DECLARE
    tenant_schema TEXT;
BEGIN
    FOR tenant_schema IN
        SELECT schema_name
        FROM information_schema.schemata
        WHERE schema_name LIKE 'tenant_%'
        ORDER BY schema_name
    LOOP
        RAISE NOTICE 'Migrating schema: %', tenant_schema;
        
        -- Contracts
        EXECUTE format('ALTER TABLE IF EXISTS %I.contracts ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.contracts ADD COLUMN IF NOT EXISTS sync_status VARCHAR(20) DEFAULT %L', tenant_schema, 'idle');
        EXECUTE format('ALTER TABLE IF EXISTS %I.contracts ADD COLUMN IF NOT EXISTS sync_version BIGINT DEFAULT 0', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.contracts ADD COLUMN IF NOT EXISTS sync_checksum TEXT', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.contracts ADD COLUMN IF NOT EXISTS is_dirty BOOLEAN DEFAULT FALSE', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.contracts ADD COLUMN IF NOT EXISTS sync_error TEXT', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.contracts ADD COLUMN IF NOT EXISTS source_of_truth VARCHAR(20) DEFAULT %L', tenant_schema, 'local');
        EXECUTE format('ALTER TABLE IF EXISTS %I.contracts ADD COLUMN IF NOT EXISTS sync_queued_at TIMESTAMPTZ', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.contracts ADD COLUMN IF NOT EXISTS sync_attempted_at TIMESTAMPTZ', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.contracts ADD COLUMN IF NOT EXISTS sequential_number INTEGER DEFAULT 0', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.contracts ADD COLUMN IF NOT EXISTS client_short_name VARCHAR(200) DEFAULT NULL', tenant_schema);
        
        -- Objects
        EXECUTE format('ALTER TABLE IF EXISTS %I.objects ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.objects ADD COLUMN IF NOT EXISTS sync_status VARCHAR(20) DEFAULT %L', tenant_schema, 'idle');
        EXECUTE format('ALTER TABLE IF EXISTS %I.objects ADD COLUMN IF NOT EXISTS sync_version BIGINT DEFAULT 0', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.objects ADD COLUMN IF NOT EXISTS sync_checksum TEXT', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.objects ADD COLUMN IF NOT EXISTS is_dirty BOOLEAN DEFAULT FALSE', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.objects ADD COLUMN IF NOT EXISTS sync_error TEXT', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.objects ADD COLUMN IF NOT EXISTS source_of_truth VARCHAR(20) DEFAULT %L', tenant_schema, 'local');
        EXECUTE format('ALTER TABLE IF EXISTS %I.objects ADD COLUMN IF NOT EXISTS sync_queued_at TIMESTAMPTZ', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.objects ADD COLUMN IF NOT EXISTS sync_attempted_at TIMESTAMPTZ', tenant_schema);
        
        -- Users
        EXECUTE format('ALTER TABLE IF EXISTS %I.users ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.users ADD COLUMN IF NOT EXISTS sync_status VARCHAR(20) DEFAULT %L', tenant_schema, 'idle');
        EXECUTE format('ALTER TABLE IF EXISTS %I.users ADD COLUMN IF NOT EXISTS sync_version BIGINT DEFAULT 0', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.users ADD COLUMN IF NOT EXISTS sync_checksum TEXT', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.users ADD COLUMN IF NOT EXISTS is_dirty BOOLEAN DEFAULT FALSE', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.users ADD COLUMN IF NOT EXISTS sync_error TEXT', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.users ADD COLUMN IF NOT EXISTS source_of_truth VARCHAR(20) DEFAULT %L', tenant_schema, 'local');
        EXECUTE format('ALTER TABLE IF EXISTS %I.users ADD COLUMN IF NOT EXISTS sync_queued_at TIMESTAMPTZ', tenant_schema);
        EXECUTE format('ALTER TABLE IF EXISTS %I.users ADD COLUMN IF NOT EXISTS sync_attempted_at TIMESTAMPTZ', tenant_schema);
        
        RAISE NOTICE 'Completed migration for schema: %', tenant_schema;
    END LOOP;
END$$;

