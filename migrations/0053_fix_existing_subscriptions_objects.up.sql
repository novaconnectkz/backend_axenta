-- Миграция: Исправление привязки существующих объектов к подпискам
-- Дата: 2025-11-25
-- Описание: Для объектов, которые были добавлены в подписку но не получили subscription_id,
--           обновляем subscription_id на основе последней активной подписки договора

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
            
            -- Для каждого договора находим последнюю активную подписку
            -- и обновляем все объекты этого договора, у которых subscription_id IS NULL
            EXECUTE format('
                WITH latest_subscription AS (
                    SELECT DISTINCT ON (contract_id)
                        contract_id,
                        id as subscription_id
                    FROM public.subscriptions
                    WHERE contract_id IS NOT NULL
                      AND status IN (''active'', ''scheduled'')
                    ORDER BY contract_id, created_at DESC
                )
                UPDATE %I.contract_objects co
                SET subscription_id = ls.subscription_id
                FROM latest_subscription ls
                WHERE co.contract_id = ls.contract_id
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
    RAISE NOTICE '✅ Исправление завершено. Всего обновлено записей: %', total_updated;
    RAISE NOTICE '';
    RAISE NOTICE '📝 Примечание:';
    RAISE NOTICE '   - Объекты привязаны к последней активной подписке договора';
    RAISE NOTICE '   - При создании новых подписок объекты будут корректно привязываться';
END $$;

