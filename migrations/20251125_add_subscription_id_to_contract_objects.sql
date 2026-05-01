-- Миграция: Добавление поля subscription_id в таблицу contract_objects
-- Дата: 2025-11-25
-- Описание: Добавляем поле subscription_id для привязки объектов к конкретной подписке

-- Для каждой tenant-схемы выполняем добавление поля subscription_id
DO $$
DECLARE
    schema_name TEXT;
BEGIN
    -- Получаем все схемы tenant_*
    FOR schema_name IN 
        SELECT nspname 
        FROM pg_namespace 
        WHERE nspname LIKE 'tenant_%'
    LOOP
        -- Проверяем, существует ли таблица contract_objects в схеме
        IF EXISTS (
            SELECT 1 
            FROM information_schema.tables 
            WHERE table_schema = schema_name 
            AND table_name = 'contract_objects'
        ) THEN
            -- Проверяем, существует ли уже поле subscription_id
            IF NOT EXISTS (
                SELECT 1 
                FROM information_schema.columns 
                WHERE table_schema = schema_name 
                AND table_name = 'contract_objects' 
                AND column_name = 'subscription_id'
            ) THEN
                -- Добавляем поле subscription_id
                EXECUTE format('ALTER TABLE %I.contract_objects ADD COLUMN subscription_id INTEGER', schema_name);
                
                -- Создаем индекс для улучшения производительности запросов
                EXECUTE format('CREATE INDEX idx_contract_objects_subscription_id ON %I.contract_objects(subscription_id)', schema_name);
                
                RAISE NOTICE 'Добавлено поле subscription_id в схему %', schema_name;
            ELSE
                RAISE NOTICE 'Поле subscription_id уже существует в схеме %', schema_name;
            END IF;
        ELSE
            RAISE NOTICE 'Таблица contract_objects не найдена в схеме %', schema_name;
        END IF;
    END LOOP;
END $$;

-- Логируем завершение миграции
DO $$
BEGIN
    RAISE NOTICE '✅ Миграция 20251125_add_subscription_id_to_contract_objects завершена успешно';
END $$;

