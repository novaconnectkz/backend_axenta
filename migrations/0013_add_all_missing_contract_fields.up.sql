-- Migration: 013_add_all_missing_contract_fields.sql
-- Description: Adds all missing contract fields to match the Contract model
-- Date: 2025-11-22

-- Add all missing columns to public.contracts
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_legal_address TEXT;
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_postal_address TEXT;
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_ogrn VARCHAR(20);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_okpo VARCHAR(20);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_director VARCHAR(200);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_based_on VARCHAR(200);

-- Bank fields
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_bank_name VARCHAR(200);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_bank_bik VARCHAR(20);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_bank_correspondent_account VARCHAR(20);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_bank_account VARCHAR(20);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_bank_recipient VARCHAR(200);

-- Individual person fields
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_passport_series VARCHAR(10);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_passport_number VARCHAR(20);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_passport_issued_by TEXT;
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_passport_issue_date VARCHAR(20);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_passport_department_code VARCHAR(10);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_registration_address TEXT;
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_actual_address TEXT;
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_snils VARCHAR(20);
ALTER TABLE public.contracts ADD COLUMN IF NOT EXISTS client_ogrn_ip VARCHAR(20);

-- Add comments
COMMENT ON COLUMN public.contracts.client_legal_address IS 'Юридический адрес';
COMMENT ON COLUMN public.contracts.client_postal_address IS 'Почтовый адрес';
COMMENT ON COLUMN public.contracts.client_ogrn IS 'ОГРН организации';
COMMENT ON COLUMN public.contracts.client_okpo IS 'ОКПО организации';
COMMENT ON COLUMN public.contracts.client_director IS 'Руководитель организации';
COMMENT ON COLUMN public.contracts.client_based_on IS 'Действует на основании';
COMMENT ON COLUMN public.contracts.client_bank_name IS 'Название банка';
COMMENT ON COLUMN public.contracts.client_bank_bik IS 'БИК банка';
COMMENT ON COLUMN public.contracts.client_bank_correspondent_account IS 'Корреспондентский счет';
COMMENT ON COLUMN public.contracts.client_bank_account IS 'Расчетный счет';
COMMENT ON COLUMN public.contracts.client_bank_recipient IS 'Получатель';
COMMENT ON COLUMN public.contracts.client_passport_series IS 'Серия паспорта (ФЛ)';
COMMENT ON COLUMN public.contracts.client_passport_number IS 'Номер паспорта (ФЛ)';
COMMENT ON COLUMN public.contracts.client_passport_issued_by IS 'Кем выдан паспорт (ФЛ)';
COMMENT ON COLUMN public.contracts.client_passport_issue_date IS 'Дата выдачи паспорта (ФЛ)';
COMMENT ON COLUMN public.contracts.client_passport_department_code IS 'Код подразделения (ФЛ)';
COMMENT ON COLUMN public.contracts.client_registration_address IS 'Адрес регистрации (ФЛ)';
COMMENT ON COLUMN public.contracts.client_actual_address IS 'Фактический адрес (ФЛ)';
COMMENT ON COLUMN public.contracts.client_snils IS 'СНИЛС (ФЛ)';
COMMENT ON COLUMN public.contracts.client_ogrn_ip IS 'ОГРНИП (для ИП)';

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
        -- Check if contracts table exists in tenant schema
        SELECT EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_schema = tenant_schema AND table_name = 'contracts'
        ) INTO table_exists;
        
        IF table_exists THEN
            -- Organization fields
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_legal_address TEXT', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_postal_address TEXT', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_ogrn VARCHAR(20)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_okpo VARCHAR(20)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_director VARCHAR(200)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_based_on VARCHAR(200)', tenant_schema);
            
            -- Bank fields
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_bank_name VARCHAR(200)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_bank_bik VARCHAR(20)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_bank_correspondent_account VARCHAR(20)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_bank_account VARCHAR(20)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_bank_recipient VARCHAR(200)', tenant_schema);
            
            -- Individual person fields
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_passport_series VARCHAR(10)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_passport_number VARCHAR(20)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_passport_issued_by TEXT', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_passport_issue_date VARCHAR(20)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_passport_department_code VARCHAR(10)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_registration_address TEXT', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_actual_address TEXT', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_snils VARCHAR(20)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN IF NOT EXISTS client_ogrn_ip VARCHAR(20)', tenant_schema);
            
            RAISE NOTICE 'Added missing columns to %.contracts', tenant_schema;
        ELSE
            RAISE NOTICE 'Skipped % (no contracts table)', tenant_schema;
        END IF;
    END LOOP;
END $$;

-- Verify migration
DO $$
DECLARE
    public_count INTEGER;
    expected_columns TEXT[] := ARRAY[
        'client_legal_address', 'client_postal_address', 'client_ogrn', 'client_okpo',
        'client_director', 'client_based_on', 'client_bank_name', 'client_bank_bik',
        'client_bank_correspondent_account', 'client_bank_account', 'client_bank_recipient',
        'client_passport_series', 'client_passport_number', 'client_passport_issued_by',
        'client_passport_issue_date', 'client_passport_department_code',
        'client_registration_address', 'client_actual_address', 'client_snils', 'client_ogrn_ip'
    ];
BEGIN
    SELECT COUNT(*) INTO public_count
    FROM information_schema.columns
    WHERE table_schema = 'public'
    AND table_name = 'contracts'
    AND column_name = ANY(expected_columns);
    
    RAISE NOTICE '✅ Migration 013 completed:';
    RAISE NOTICE '   - Added %/% expected columns to public.contracts', public_count, array_length(expected_columns, 1);
END $$;

