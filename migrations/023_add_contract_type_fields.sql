-- Миграция для добавления типов договоров (client и partner)
-- Создано: 2024-11-28

-- Функция для применения миграции к tenant схеме
CREATE OR REPLACE FUNCTION add_contract_type_fields_to_schema(schema_name text)
RETURNS void AS $$
BEGIN
    -- Проверяем, существует ли таблица contracts
    IF EXISTS (
        SELECT 1 
        FROM information_schema.tables 
        WHERE table_schema = schema_name 
        AND table_name = 'contracts'
    ) THEN
        -- Добавляем поле contract_type (client или partner)
        IF NOT EXISTS (
            SELECT 1 
            FROM information_schema.columns 
            WHERE table_schema = schema_name 
            AND table_name = 'contracts' 
            AND column_name = 'contract_type'
        ) THEN
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN contract_type VARCHAR(20) DEFAULT ''client''', schema_name);
            RAISE NOTICE 'Добавлено поле contract_type в схему %', schema_name;
        ELSE
            RAISE NOTICE 'Поле contract_type уже существует в схеме %', schema_name;
        END IF;

        -- Добавляем поле partner_company_id для партнерских договоров
        IF NOT EXISTS (
            SELECT 1 
            FROM information_schema.columns 
            WHERE table_schema = schema_name 
            AND table_name = 'contracts' 
            AND column_name = 'partner_company_id'
        ) THEN
            EXECUTE format('ALTER TABLE %I.contracts ADD COLUMN partner_company_id INTEGER', schema_name);
            RAISE NOTICE 'Добавлено поле partner_company_id в схему %', schema_name;
        ELSE
            RAISE NOTICE 'Поле partner_company_id уже существует в схеме %', schema_name;
        END IF;

        -- Создаем индекс для partner_company_id
        IF NOT EXISTS (
            SELECT 1 
            FROM pg_indexes 
            WHERE schemaname = schema_name 
            AND tablename = 'contracts' 
            AND indexname = 'idx_contracts_partner_company_id'
        ) THEN
            EXECUTE format('CREATE INDEX idx_contracts_partner_company_id ON %I.contracts (partner_company_id)', schema_name);
            RAISE NOTICE 'Создан индекс idx_contracts_partner_company_id в схеме %', schema_name;
        ELSE
            RAISE NOTICE 'Индекс idx_contracts_partner_company_id уже существует в схеме %', schema_name;
        END IF;

        -- Обновляем существующие договоры, устанавливая contract_type = 'client' по умолчанию
        EXECUTE format('UPDATE %I.contracts SET contract_type = ''client'' WHERE contract_type IS NULL', schema_name);
        RAISE NOTICE 'Обновлены существующие договоры в схеме % (установлен contract_type = client)', schema_name;
    ELSE
        RAISE NOTICE 'Таблица contracts не найдена в схеме %', schema_name;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Применяем миграцию ко всем tenant схемам
DO $$
DECLARE
    schema_record RECORD;
BEGIN
    FOR schema_record IN 
        SELECT schema_name 
        FROM information_schema.schemata 
        WHERE schema_name LIKE 'tenant_%'
    LOOP
        PERFORM add_contract_type_fields_to_schema(schema_record.schema_name);
    END LOOP;
END $$;

-- Удаляем вспомогательную функцию
DROP FUNCTION IF EXISTS add_contract_type_fields_to_schema(text);

