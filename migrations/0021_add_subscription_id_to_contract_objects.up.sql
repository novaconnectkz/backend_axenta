-- Добавление subscription_id в contract_objects
-- Дата: 2025-11-26
-- Описание: Добавляет колонку subscription_id для привязки объектов к конкретным подпискам

-- Для tenant_186
DO $$
BEGIN
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'tenant_186' AND table_name = 'contract_objects') THEN
        ALTER TABLE tenant_186.contract_objects 
        ADD COLUMN IF NOT EXISTS subscription_id INTEGER;
        
        CREATE INDEX IF NOT EXISTS idx_contract_objects_subscription_id 
        ON tenant_186.contract_objects(subscription_id);
        
        RAISE NOTICE 'tenant_186.contract_objects: колонка subscription_id добавлена';
    END IF;
END $$;

-- Для tenant_default
DO $$
BEGIN
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'tenant_default' AND table_name = 'contract_objects') THEN
        ALTER TABLE tenant_default.contract_objects 
        ADD COLUMN IF NOT EXISTS subscription_id INTEGER;
        
        CREATE INDEX IF NOT EXISTS idx_contract_objects_subscription_id 
        ON tenant_default.contract_objects(subscription_id);
        
        RAISE NOTICE 'tenant_default.contract_objects: колонка subscription_id добавлена';
    END IF;
END $$;

-- Для tenant_newacrm
DO $$
BEGIN
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'tenant_newacrm' AND table_name = 'contract_objects') THEN
        ALTER TABLE tenant_newacrm.contract_objects 
        ADD COLUMN IF NOT EXISTS subscription_id INTEGER;
        
        CREATE INDEX IF NOT EXISTS idx_contract_objects_subscription_id 
        ON tenant_newacrm.contract_objects(subscription_id);
        
        RAISE NOTICE 'tenant_newacrm.contract_objects: колонка subscription_id добавлена';
    END IF;
END $$;

-- Для tenant_yyqqqqqq
DO $$
BEGIN
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'tenant_yyqqqqqq' AND table_name = 'contract_objects') THEN
        ALTER TABLE tenant_yyqqqqqq.contract_objects 
        ADD COLUMN IF NOT EXISTS subscription_id INTEGER;
        
        CREATE INDEX IF NOT EXISTS idx_contract_objects_subscription_id 
        ON tenant_yyqqqqqq.contract_objects(subscription_id);
        
        RAISE NOTICE 'tenant_yyqqqqqq.contract_objects: колонка subscription_id добавлена';
    END IF;
END $$;

