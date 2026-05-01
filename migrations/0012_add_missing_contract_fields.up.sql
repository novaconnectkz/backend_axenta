-- Migration: 012_add_missing_contract_fields.sql
-- Description: Adds missing client_type and client_website columns to contracts table
-- Date: 2025-11-22

-- Add client_type column to public.contracts
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = 'public' 
        AND table_name = 'contracts' 
        AND column_name = 'client_type'
    ) THEN
        ALTER TABLE public.contracts 
        ADD COLUMN client_type VARCHAR(20);
        
        COMMENT ON COLUMN public.contracts.client_type IS 'Тип клиента: individual или organization';
    END IF;
END $$;

-- Add client_website column to public.contracts
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = 'public' 
        AND table_name = 'contracts' 
        AND column_name = 'client_website'
    ) THEN
        ALTER TABLE public.contracts 
        ADD COLUMN client_website VARCHAR(200);
        
        COMMENT ON COLUMN public.contracts.client_website IS 'Веб-сайт клиента';
    END IF;
END $$;

-- Update existing records (set organization as default if client_inn is present)
UPDATE public.contracts 
SET client_type = 'organization' 
WHERE client_type IS NULL 
AND client_inn IS NOT NULL 
AND client_inn != '';

-- Set individual for records without INN
UPDATE public.contracts 
SET client_type = 'individual' 
WHERE client_type IS NULL;

-- Now apply the same changes to all tenant schemas
DO $$
DECLARE
    tenant_schema TEXT;
BEGIN
    FOR tenant_schema IN 
        SELECT schema_name 
        FROM information_schema.schemata 
        WHERE schema_name LIKE 'tenant_%'
    LOOP
        -- Add client_type column
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns 
            WHERE table_schema = tenant_schema 
            AND table_name = 'contracts' 
            AND column_name = 'client_type'
        ) THEN
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN client_type VARCHAR(20)', tenant_schema);
            EXECUTE format('COMMENT ON COLUMN %I.contracts.client_type IS ''Тип клиента: individual или organization''', tenant_schema);
        END IF;

        -- Add client_website column
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns 
            WHERE table_schema = tenant_schema 
            AND table_name = 'contracts' 
            AND column_name = 'client_website'
        ) THEN
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN client_website VARCHAR(200)', tenant_schema);
            EXECUTE format('COMMENT ON COLUMN %I.contracts.client_website IS ''Веб-сайт клиента''', tenant_schema);
        END IF;

        -- Update existing records in tenant schema
        EXECUTE format('UPDATE %I.contracts SET client_type = ''organization'' WHERE client_type IS NULL AND client_inn IS NOT NULL AND client_inn != ''''', tenant_schema);
        EXECUTE format('UPDATE %I.contracts SET client_type = ''individual'' WHERE client_type IS NULL', tenant_schema);
        
        RAISE NOTICE 'Added missing columns to %.contracts', tenant_schema;
    END LOOP;
END $$;

-- Verify the migration
DO $$
DECLARE
    public_count INTEGER;
    tenant_count INTEGER;
    total_tenants INTEGER;
BEGIN
    -- Check public schema
    SELECT COUNT(*) INTO public_count
    FROM information_schema.columns
    WHERE table_schema = 'public'
    AND table_name = 'contracts'
    AND column_name IN ('client_type', 'client_website');
    
    -- Check tenant schemas
    SELECT COUNT(DISTINCT table_schema) INTO total_tenants
    FROM information_schema.columns
    WHERE table_schema LIKE 'tenant_%'
    AND table_name = 'contracts';
    
    SELECT COUNT(DISTINCT table_schema) INTO tenant_count
    FROM information_schema.columns
    WHERE table_schema LIKE 'tenant_%'
    AND table_name = 'contracts'
    AND column_name IN ('client_type', 'client_website')
    GROUP BY table_schema
    HAVING COUNT(*) = 2;
    
    RAISE NOTICE '✅ Migration 012 completed:';
    RAISE NOTICE '   - Public schema: % columns added', public_count;
    RAISE NOTICE '   - Tenant schemas: %/% updated', tenant_count, total_tenants;
END $$;

