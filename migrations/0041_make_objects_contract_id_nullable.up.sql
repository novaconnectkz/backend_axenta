-- Миграция для изменения колонки contract_id в таблице objects
-- Делаем contract_id опциональным (NULL) и очищаем у объектов без активных договоров

-- Применяем изменения для всех tenant схем
DO $$
DECLARE
    schema_name TEXT;
    updated_count INTEGER;
BEGIN
    -- Обрабатываем каждую tenant схему
    FOR schema_name IN 
        SELECT nspname 
        FROM pg_namespace 
        WHERE nspname LIKE 'tenant_%'
    LOOP
        -- Проверяем, существует ли таблица objects в этой схеме
        IF EXISTS (
            SELECT 1 
            FROM information_schema.tables 
            WHERE table_schema = schema_name 
            AND table_name = 'objects'
        ) THEN
            -- Делаем contract_id nullable
            EXECUTE format('
                ALTER TABLE %I.objects
                ALTER COLUMN contract_id DROP NOT NULL;
            ', schema_name);
            
            RAISE NOTICE 'Сделали contract_id nullable в схеме: %', schema_name;
            
            -- Обнуляем contract_id у объектов, где договор не существует
            EXECUTE format('
                UPDATE %I.objects
                SET contract_id = NULL
                WHERE contract_id IS NOT NULL
                  AND NOT EXISTS (
                    SELECT 1 FROM %I.contracts WHERE id = objects.contract_id
                  );
            ', schema_name, schema_name);
            
            GET DIAGNOSTICS updated_count = ROW_COUNT;
            
            IF updated_count > 0 THEN
                RAISE NOTICE 'Очищено contract_id у % объектов в схеме: %', updated_count, schema_name;
            END IF;
        END IF;
    END LOOP;
    
    RAISE NOTICE '✅ Миграция завершена успешно';
END $$;

