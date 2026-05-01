-- Миграция: Обновление существующих записей contract_objects для привязки к подпискам
-- Дата: 2025-11-25
-- Описание: Обновляем существующие записи contract_objects, устанавливая subscription_id
--           на основе первой активной подписки договора

DO $$
DECLARE
    schema_name TEXT;
    updated_count INTEGER;
    total_updated INTEGER := 0;
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
            RAISE NOTICE 'Обработка схемы: %', schema_name;
            
            -- Обновляем записи contract_objects, у которых subscription_id = NULL
            -- Привязываем их к первой активной подписке договора из таблицы public.subscriptions
            EXECUTE format('
                WITH subscription_map AS (
                    SELECT DISTINCT ON (contract_id)
                        contract_id,
                        id as subscription_id
                    FROM public.subscriptions
                    WHERE contract_id IS NOT NULL
                      AND status IN (''active'', ''scheduled'')
                    ORDER BY contract_id, created_at ASC
                )
                UPDATE %I.contract_objects co
                SET subscription_id = sm.subscription_id
                FROM subscription_map sm
                WHERE co.contract_id = sm.contract_id
                  AND co.subscription_id IS NULL
                  AND co.status = ''active''
            ', schema_name);
            
            GET DIAGNOSTICS updated_count = ROW_COUNT;
            total_updated := total_updated + updated_count;
            
            IF updated_count > 0 THEN
                RAISE NOTICE '  ✅ Обновлено записей: %', updated_count;
            ELSE
                RAISE NOTICE '  ℹ️  Нет записей для обновления';
            END IF;
        ELSE
            RAISE NOTICE 'Таблица contract_objects не найдена в схеме %', schema_name;
        END IF;
    END LOOP;
    
    RAISE NOTICE '';
    RAISE NOTICE '✅ Миграция завершена. Всего обновлено записей: %', total_updated;
    RAISE NOTICE '';
    RAISE NOTICE '⚠️  Примечание:';
    RAISE NOTICE '   - Объекты без активных подписок остались с subscription_id = NULL';
    RAISE NOTICE '   - Такие объекты не будут отображаться в подписках';
    RAISE NOTICE '   - Это нормально для объектов, добавленных вне подписок';
END $$;

