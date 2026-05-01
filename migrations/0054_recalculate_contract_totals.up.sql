-- Миграция для пересчета total_amount всех договоров на основе их подписок
-- Дата: 2025-11-25
-- Описание: Исправляет неправильный расчет суммы договора, который использовал tariff_plan_id
--           вместо суммирования стоимости всех подписок

DO $$
DECLARE
    r RECORD;
    tenant_schema TEXT;
    contract_rec RECORD;
    subscription_rec RECORD;
    billing_plan_rec RECORD;
    total_amount DECIMAL(10,2);
    subscription_amount DECIMAL(10,2);
    objects_count INTEGER;
    months INTEGER;
    days INTEGER;
BEGIN
    -- Проходим по всем tenant схемам
    FOR r IN (SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tenant_%')
    LOOP
        tenant_schema := r.schema_name;
        EXECUTE 'SET search_path TO ' || quote_ident(tenant_schema) || ', public';
        
        RAISE NOTICE '📊 Обработка схемы: %', tenant_schema;
        
        -- Проходим по всем договорам в схеме
        FOR contract_rec IN 
            EXECUTE 'SELECT id, number, start_date, end_date FROM ' || quote_ident(tenant_schema) || '.contracts'
        LOOP
            total_amount := 0;
            
            -- Рассчитываем количество месяцев в договоре
            IF contract_rec.start_date IS NOT NULL AND contract_rec.end_date IS NOT NULL THEN
                days := EXTRACT(DAY FROM (contract_rec.end_date - contract_rec.start_date));
                months := GREATEST(1, days / 30);
            ELSE
                months := 1;
            END IF;
            
            -- Проходим по всем активным подпискам договора
            FOR subscription_rec IN 
                SELECT s.id, s.billing_plan_id
                FROM public.subscriptions s
                WHERE s.contract_id = contract_rec.id
                  AND s.deleted_at IS NULL
                  AND s.status NOT IN ('cancelled', 'expired')
            LOOP
                -- Получаем тарифный план подписки
                SELECT price, billing_period INTO billing_plan_rec
                FROM public.billing_plans
                WHERE id = subscription_rec.billing_plan_id;
                
                -- Подсчитываем количество объектов в подписке
                EXECUTE format(
                    'SELECT COUNT(*) FROM %I.contract_objects WHERE contract_id = $1 AND subscription_id = $2 AND status = ''active''',
                    tenant_schema
                ) INTO objects_count USING contract_rec.id, subscription_rec.id;
                
                -- Рассчитываем стоимость подписки
                IF billing_plan_rec.billing_period = 'yearly' THEN
                    -- Годовой тариф: (цена / 12) × количество месяцев × количество объектов
                    subscription_amount := (billing_plan_rec.price / 12) * months * objects_count;
                ELSIF billing_plan_rec.billing_period = 'weekly' THEN
                    -- Недельный тариф: цена × количество недель × количество объектов
                    subscription_amount := billing_plan_rec.price * ((months * 30) / 7) * objects_count;
                ELSIF billing_plan_rec.billing_period = 'daily' THEN
                    -- Дневной тариф: цена × количество дней × количество объектов
                    subscription_amount := billing_plan_rec.price * (months * 30) * objects_count;
                ELSIF billing_plan_rec.billing_period = 'hourly' THEN
                    -- Часовой тариф: цена × количество часов × количество объектов
                    subscription_amount := billing_plan_rec.price * (months * 30 * 24) * objects_count;
                ELSE
                    -- Месячный тариф с пролонгацией: цена × количество объектов
                    subscription_amount := billing_plan_rec.price * objects_count;
                END IF;
                
                total_amount := total_amount + subscription_amount;
                
                RAISE NOTICE '  - Подписка #%: % объектов × % (%) = %',
                    subscription_rec.id,
                    objects_count,
                    billing_plan_rec.price,
                    billing_plan_rec.billing_period,
                    subscription_amount;
            END LOOP;
            
            -- Обновляем total_amount договора
            IF total_amount > 0 THEN
                EXECUTE format(
                    'UPDATE %I.contracts SET total_amount = $1 WHERE id = $2',
                    tenant_schema
                ) USING total_amount, contract_rec.id;
                
                RAISE NOTICE '✅ Договор % (ID %): обновлена сумма на %',
                    contract_rec.number, contract_rec.id, total_amount;
            END IF;
        END LOOP;
    END LOOP;
    
    RAISE NOTICE '✅ Миграция 20251125_recalculate_contract_totals завершена успешно';
END;
$$;

