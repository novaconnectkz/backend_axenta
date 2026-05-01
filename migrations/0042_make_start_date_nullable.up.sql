-- Миграция для изменения поля start_date на nullable
-- Период договора будет устанавливаться через подписку

BEGIN;

-- Изменяем поле start_date для всех tenant схем
DO $$
DECLARE
    tenant_schema TEXT;
BEGIN
    -- Для каждой tenant схемы
    FOR tenant_schema IN 
        SELECT database_schema 
        FROM public.companies 
        WHERE database_schema IS NOT NULL AND database_schema != ''
    LOOP
        -- Проверяем, существует ли таблица contracts в этой схеме
        IF EXISTS (
            SELECT 1 
            FROM information_schema.tables 
            WHERE table_schema = tenant_schema 
            AND table_name = 'contracts'
        ) THEN
            -- Изменяем поле start_date на nullable
            EXECUTE format('ALTER TABLE %I.contracts ALTER COLUMN start_date DROP NOT NULL;', tenant_schema);
            RAISE NOTICE 'Updated start_date column in schema: %', tenant_schema;
        END IF;
    END LOOP;
END $$;

COMMIT;

