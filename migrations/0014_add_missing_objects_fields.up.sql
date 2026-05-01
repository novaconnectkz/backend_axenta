-- Migration: 014_add_missing_objects_fields.sql
-- Description: Adds missing fields to objects table
-- Date: 2025-11-22

-- Add missing columns to public.objects
ALTER TABLE public.objects ADD COLUMN IF NOT EXISTS company_id BIGINT;
ALTER TABLE public.objects ADD COLUMN IF NOT EXISTS external_account_id BIGINT;
ALTER TABLE public.objects ADD COLUMN IF NOT EXISTS external_account_name VARCHAR(200);
ALTER TABLE public.objects ADD COLUMN IF NOT EXISTS source VARCHAR(50);

-- Add index for company_id if not exists
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE tablename = 'objects' 
        AND schemaname = 'public' 
        AND indexname = 'idx_objects_company_id'
    ) THEN
        CREATE INDEX idx_objects_company_id ON public.objects(company_id);
    END IF;
END $$;

-- Add comments
COMMENT ON COLUMN public.objects.company_id IS 'ID компании-владельца объекта';
COMMENT ON COLUMN public.objects.external_account_id IS 'ID внешнего аккаунта';
COMMENT ON COLUMN public.objects.external_account_name IS 'Название внешнего аккаунта';
COMMENT ON COLUMN public.objects.source IS 'Источник объекта (local, wialon, etc.)';

-- Apply to all tenant schemas
DO $$
DECLARE
    tenant_schema TEXT;
    table_exists BOOLEAN;
BEGIN
    FOR tenant_schema IN
        SELECT schema_name 
        FROM information_schema.schemata 
        WHERE schema_name LIKE 'tenant_%'
    LOOP
        -- Check if objects table exists in tenant schema
        SELECT EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_schema = tenant_schema AND table_name = 'objects'
        ) INTO table_exists;
        
        IF table_exists THEN
            -- Add missing columns
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS company_id BIGINT', tenant_schema);
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS external_account_id BIGINT', tenant_schema);
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS external_account_name VARCHAR(200)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS source VARCHAR(50)', tenant_schema);
            
            -- Add index for company_id
            EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%s_objects_company_id ON %I.objects(company_id)', 
                          replace(tenant_schema, 'tenant_', ''), tenant_schema);
            
            RAISE NOTICE 'Added missing columns to %.objects', tenant_schema;
        ELSE
            RAISE NOTICE 'Skipped % (no objects table)', tenant_schema;
        END IF;
    END LOOP;
END $$;

-- Verify migration
DO $$
DECLARE
    public_count INTEGER;
    expected_columns TEXT[] := ARRAY['company_id', 'external_account_id', 'external_account_name', 'source'];
BEGIN
    SELECT COUNT(*) INTO public_count
    FROM information_schema.columns
    WHERE table_schema = 'public'
    AND table_name = 'objects'
    AND column_name = ANY(expected_columns);
    
    RAISE NOTICE '✅ Migration 014 completed:';
    RAISE NOTICE '   - Added %/% expected columns to public.objects', public_count, array_length(expected_columns, 1);
    
    -- Show total column count
    SELECT COUNT(*) INTO public_count
    FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'objects';
    
    RAISE NOTICE '   - Total columns in public.objects: %', public_count;
END $$;

