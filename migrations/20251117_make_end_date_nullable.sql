-- Миграция для изменения колонки end_date в таблицах contracts и contract_objects
-- Делаем end_date опциональным (NULL), так как период будет устанавливаться через подписку

-- Применяем изменения для всех tenant схем
DO $$
DECLARE
    schema_name TEXT;
BEGIN
    -- Обрабатываем каждую tenant схему
    FOR schema_name IN 
        SELECT nspname 
        FROM pg_namespace 
        WHERE nspname LIKE 'tenant_%'
    LOOP
        -- Изменяем таблицу contracts
        EXECUTE format('
            ALTER TABLE %I.contracts
                ALTER COLUMN end_date DROP NOT NULL,
                ALTER COLUMN end_date SET DEFAULT NULL;
        ', schema_name);
        
        -- Изменяем таблицу contract_objects
        EXECUTE format('
            ALTER TABLE %I.contract_objects
                ALTER COLUMN end_date DROP NOT NULL,
                ALTER COLUMN end_date SET DEFAULT NULL;
        ', schema_name);
        
        RAISE NOTICE 'Обновлена схема: %', schema_name;
    END LOOP;
END $$;

