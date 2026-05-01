-- ================================================================
-- Migration 017: Создание дефолтных нумераторов договоров
-- Для компаний, у которых method = 'numerator', но нет нумераторов
-- ================================================================

-- Создаем нумераторы для компаний, где используется метод 'numerator'
-- но нумераторы еще не созданы

DO $$ 
DECLARE
    company_record RECORD;
    new_numerator_id BIGINT;
BEGIN
    -- Для каждой компании с методом 'numerator'
    FOR company_record IN 
        SELECT id, company_id 
        FROM billing_settings 
        WHERE contract_numbering_method = 'numerator'
    LOOP
        -- Проверяем, есть ли уже нумератор для этой компании
        IF NOT EXISTS (
            SELECT 1 FROM contract_numerators 
            WHERE company_id = company_record.company_id
        ) THEN
            -- Создаем дефолтный нумератор
            INSERT INTO contract_numerators (
                created_at,
                updated_at,
                company_id,
                name,
                prefix,
                template,
                description,
                counter_value,
                is_default,
                is_active
            ) VALUES (
                NOW(),
                NOW(),
                company_record.company_id,
                'Стандартный нумератор договоров',
                'Т-',
                'Т-{YYMMDD}/{NNN}',
                'Автоматически созданный нумератор для договоров',
                1,
                true,
                true
            ) RETURNING id INTO new_numerator_id;
            
            -- Обновляем billing_settings, устанавливая этот нумератор как default
            UPDATE billing_settings 
            SET contract_default_numerator_id = new_numerator_id,
                updated_at = NOW()
            WHERE id = company_record.id;
            
            RAISE NOTICE 'Создан нумератор ID=% для компании %', new_numerator_id, company_record.company_id;
        ELSE
            RAISE NOTICE 'У компании % уже есть нумератор', company_record.company_id;
        END IF;
    END LOOP;
    
    -- Подсчитываем результат
    RAISE NOTICE 'Миграция завершена. Всего нумераторов: %', 
        (SELECT COUNT(*) FROM contract_numerators);
END $$;

-- Комментарии
COMMENT ON COLUMN contract_numerators.template IS 'Шаблон номера: {YYYY}=год, {MM}=месяц, {DD}=день, {NNN}=счетчик';
COMMENT ON COLUMN contract_numerators.counter_value IS 'Текущее значение счетчика';
COMMENT ON COLUMN contract_numerators.is_default IS 'Используется по умолчанию для новых договоров';
