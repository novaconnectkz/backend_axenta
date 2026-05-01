-- Миграция: Очистка невалидных subscription_id в contract_objects
-- Дата: 2025-11-25
-- Описание: Обнуляем subscription_id у объектов, которые привязаны к:
--           1. Несуществующим подпискам (удаленным)
--           2. Отмененным подпискам (status = 'cancelled')
--           3. Истекшим подпискам (status = 'expired')

DO $$
DECLARE
    schema_name TEXT;
    cleaned_count INTEGER;
    total_cleaned INTEGER := 0;
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
            
            -- Обнуляем subscription_id у объектов, привязанных к несуществующим подпискам
            EXECUTE format('
                UPDATE %I.contract_objects co
                SET subscription_id = NULL
                WHERE subscription_id IS NOT NULL
                  AND NOT EXISTS (
                      SELECT 1 
                      FROM public.subscriptions s 
                      WHERE s.id = co.subscription_id
                        AND s.deleted_at IS NULL
                  )
            ', schema_name);
            
            GET DIAGNOSTICS cleaned_count = ROW_COUNT;
            
            IF cleaned_count > 0 THEN
                RAISE NOTICE '  ✅ Обнулено subscription_id у % объектов (несуществующие подписки)', cleaned_count;
                total_cleaned := total_cleaned + cleaned_count;
            END IF;
            
            -- Обнуляем subscription_id у объектов, привязанных к отмененным/истекшим подпискам
            EXECUTE format('
                UPDATE %I.contract_objects co
                SET subscription_id = NULL
                WHERE subscription_id IS NOT NULL
                  AND EXISTS (
                      SELECT 1 
                      FROM public.subscriptions s 
                      WHERE s.id = co.subscription_id
                        AND s.deleted_at IS NULL
                        AND s.status IN (''cancelled'', ''expired'')
                  )
            ', schema_name);
            
            GET DIAGNOSTICS cleaned_count = ROW_COUNT;
            
            IF cleaned_count > 0 THEN
                RAISE NOTICE '  ✅ Обнулено subscription_id у % объектов (отмененные/истекшие подписки)', cleaned_count;
                total_cleaned := total_cleaned + cleaned_count;
            END IF;
            
            IF cleaned_count = 0 THEN
                RAISE NOTICE '  ℹ️  Нет невалидных subscription_id';
            END IF;
        ELSE
            RAISE NOTICE 'Таблица contract_objects не найдена в схеме %', schema_name;
        END IF;
    END LOOP;
    
    RAISE NOTICE '';
    RAISE NOTICE '✅ Очистка завершена. Всего обнулено записей: %', total_cleaned;
    RAISE NOTICE '';
    RAISE NOTICE '📝 Примечание:';
    RAISE NOTICE '   - Объекты теперь доступны для привязки к новым подпискам';
    RAISE NOTICE '   - При создании новой подписки они автоматически получат subscription_id';
END $$;

