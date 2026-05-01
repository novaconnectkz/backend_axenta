-- Миграция 002: Создание материализованных представлений для биллинга
-- Цель: оптимизация расчетов биллинга через предрасчет активных дней и компонентов

-- ============================================================================
-- 1. Материализованное представление: mv_object_days
-- Показывает активные биллинговые дни для каждого объекта с учетом заморозок
--
-- ВАЖНО О МУЛЬТИТЕНАНТНОСТИ:
-- - assignments, subscriptions находятся в схеме public (глобальные)
-- - objects, contracts находятся в тенантных схемах (per-company)
-- - contract_id установлен как NULL, т.к. contracts недоступны из public схемы
-- - Для получения contract_id используйте связь через objects в Go-коде
-- ============================================================================

-- Удаляем представления если существуют (для повторного запуска)
DROP MATERIALIZED VIEW IF EXISTS mv_object_day_components CASCADE;
DROP MATERIALIZED VIEW IF EXISTS mv_object_days CASCADE;
DROP FUNCTION IF EXISTS refresh_billing_materialized_views() CASCADE;

CREATE MATERIALIZED VIEW mv_object_days AS
WITH date_series AS (
    -- Генерируем серию дат от начала текущего года до конца следующего
    SELECT generate_series(
        date_trunc('year', CURRENT_DATE)::date,
        (date_trunc('year', CURRENT_DATE) + interval '2 years' - interval '1 day')::date,
        '1 day'::interval
    )::date AS usage_date
),
assignment_periods AS (
    -- Получаем периоды действия привязок объектов к подпискам
    -- ВАЖНО: contracts находятся в тенантных схемах, поэтому contract_id получаем через objects
    -- если objects тоже тенантные, то contract_id будет NULL (можно получить через функцию/триггер)
    SELECT 
        a.id AS assignment_id,
        a.object_id,
        a.subscription_id,
        a.tariff_plan_id,
        s.company_id,
        NULL::integer AS contract_id,  -- Будет заполнено через отдельную логику или функции
        GREATEST(a.start_date, date_trunc('year', CURRENT_DATE)::date) AS period_start,
        LEAST(
            COALESCE(a.end_date, (date_trunc('year', CURRENT_DATE) + interval '2 years' - interval '1 day')::date),
            (date_trunc('year', CURRENT_DATE) + interval '2 years' - interval '1 day')::date
        ) AS period_end,
        a.status AS assignment_status,
        a.is_active AS assignment_is_active
    FROM assignments a
    INNER JOIN subscriptions s ON s.id = a.subscription_id
    WHERE a.deleted_at IS NULL
      AND s.deleted_at IS NULL
      AND a.status = 'active'
      AND a.is_active = true
      AND s.status = 'active'
),
freeze_periods AS (
    -- Получаем периоды заморозок, которые исключают дни из биллинга
    SELECT 
        f.object_id,
        f.assignment_id,
        GREATEST(f.start_date, date_trunc('year', CURRENT_DATE)::date) AS freeze_start,
        LEAST(f.end_date, (date_trunc('year', CURRENT_DATE) + interval '2 years' - interval '1 day')::date) AS freeze_end
    FROM freezes f
    WHERE f.deleted_at IS NULL
      AND f.is_active = true
      AND f.billable = false  -- Только небиллинговые заморозки
),
object_days_raw AS (
    -- Комбинируем привязки с датами
    SELECT 
        ds.usage_date,
        ap.assignment_id,
        ap.object_id,
        ap.subscription_id,
        ap.tariff_plan_id,
        ap.company_id,
        ap.contract_id
    FROM date_series ds
    CROSS JOIN assignment_periods ap
    WHERE ds.usage_date >= ap.period_start
      AND ds.usage_date <= ap.period_end
),
object_days_excluding_freezes AS (
    -- Исключаем дни заморозок
    SELECT 
        odr.usage_date,
        odr.assignment_id,
        odr.object_id,
        odr.subscription_id,
        odr.tariff_plan_id,
        odr.company_id,
        odr.contract_id
    FROM object_days_raw odr
    WHERE NOT EXISTS (
        SELECT 1
        FROM freeze_periods fp
        WHERE fp.object_id = odr.object_id
          AND (fp.assignment_id IS NULL OR fp.assignment_id = odr.assignment_id)
          AND odr.usage_date >= fp.freeze_start
          AND odr.usage_date <= fp.freeze_end
    )
)
SELECT 
    usage_date,
    assignment_id,
    object_id,
    subscription_id,
    tariff_plan_id,
    company_id,
    contract_id,
    COUNT(*) OVER (PARTITION BY assignment_id, date_trunc('month', usage_date)) AS days_in_month,
    COUNT(*) OVER (PARTITION BY assignment_id) AS total_assignment_days
FROM object_days_excluding_freezes;

-- Создаем индексы для быстрого поиска
CREATE UNIQUE INDEX IF NOT EXISTS idx_mv_object_days_unique 
    ON mv_object_days (usage_date, assignment_id, object_id);

CREATE INDEX IF NOT EXISTS idx_mv_object_days_date 
    ON mv_object_days (usage_date);

CREATE INDEX IF NOT EXISTS idx_mv_object_days_assignment 
    ON mv_object_days (assignment_id);

CREATE INDEX IF NOT EXISTS idx_mv_object_days_object 
    ON mv_object_days (object_id);

CREATE INDEX IF NOT EXISTS idx_mv_object_days_company 
    ON mv_object_days (company_id);

CREATE INDEX IF NOT EXISTS idx_mv_object_days_contract 
    ON mv_object_days (contract_id);

CREATE INDEX IF NOT EXISTS idx_mv_object_days_subscription 
    ON mv_object_days (subscription_id);

-- ============================================================================
-- 2. Материализованное представление: mv_object_day_components
-- Разворот по тарифным компонентам для каждого дня
-- ============================================================================

CREATE MATERIALIZED VIEW mv_object_day_components AS
WITH base_days AS (
    -- Базовые дни из mv_object_days
    SELECT 
        usage_date,
        assignment_id,
        object_id,
        subscription_id,
        tariff_plan_id,
        company_id,
        contract_id
    FROM mv_object_days
),
recurring_components AS (
    -- Recurring компоненты (абонплата)
    SELECT 
        bd.usage_date,
        bd.assignment_id,
        bd.object_id,
        bd.subscription_id,
        bd.tariff_plan_id,
        bd.company_id,
        bd.contract_id,
        tc.id AS component_id,
        tc.type AS component_type,
        tc.name AS component_name,
        tc.price AS unit_price,
        CASE 
            WHEN tc.billing_period = 'monthly' THEN 
                (tc.price / DATE_PART('days', DATE_TRUNC('month', bd.usage_date) + INTERVAL '1 month' - INTERVAL '1 day'))::numeric
            WHEN tc.billing_period = 'yearly' THEN 
                (tc.price / 365)::numeric
            ELSE tc.price
        END AS daily_price,
        1.0 AS quantity,
        'recurring' AS charge_type
    FROM base_days bd
    INNER JOIN tariff_components tc ON tc.tariff_plan_id = bd.tariff_plan_id
    WHERE tc.type = 'recurring'
      AND tc.is_active = true
      AND tc.deleted_at IS NULL
),
usage_components AS (
    -- Per-usage компоненты (за использование)
    SELECT 
        bd.usage_date,
        bd.assignment_id,
        bd.object_id,
        bd.subscription_id,
        bd.tariff_plan_id,
        bd.company_id,
        bd.contract_id,
        tc.id AS component_id,
        tc.type AS component_type,
        tc.name AS component_name,
        tc.price AS unit_price,
        tc.price AS daily_price,
        COALESCE(u.quantity, 0) AS quantity,
        'per_usage' AS charge_type
    FROM base_days bd
    INNER JOIN tariff_components tc ON tc.tariff_plan_id = bd.tariff_plan_id
    LEFT JOIN usages u ON u.object_id = bd.object_id 
        AND (u.assignment_id = bd.assignment_id OR u.assignment_id IS NULL)
        AND DATE(u.usage_date) = bd.usage_date
        AND u.deleted_at IS NULL
    WHERE tc.type = 'per_usage'
      AND tc.is_active = true
      AND tc.deleted_at IS NULL
)
SELECT 
    usage_date,
    assignment_id,
    object_id,
    subscription_id,
    tariff_plan_id,
    company_id,
    contract_id,
    component_id,
    component_type,
    component_name,
    unit_price,
    daily_price,
    quantity,
    (daily_price * quantity)::numeric(15,2) AS amount,
    charge_type
FROM (
    SELECT * FROM recurring_components
    UNION ALL
    SELECT * FROM usage_components
) combined
WHERE quantity > 0;

-- Создаем индексы для быстрого поиска
CREATE UNIQUE INDEX IF NOT EXISTS idx_mv_object_day_components_unique 
    ON mv_object_day_components (usage_date, assignment_id, component_id);

CREATE INDEX IF NOT EXISTS idx_mv_object_day_components_date 
    ON mv_object_day_components (usage_date);

CREATE INDEX IF NOT EXISTS idx_mv_object_day_components_assignment 
    ON mv_object_day_components (assignment_id);

CREATE INDEX IF NOT EXISTS idx_mv_object_day_components_object 
    ON mv_object_day_components (object_id);

CREATE INDEX IF NOT EXISTS idx_mv_object_day_components_company 
    ON mv_object_day_components (company_id);

CREATE INDEX IF NOT EXISTS idx_mv_object_day_components_contract 
    ON mv_object_day_components (contract_id);

CREATE INDEX IF NOT EXISTS idx_mv_object_day_components_component 
    ON mv_object_day_components (component_id);

CREATE INDEX IF NOT EXISTS idx_mv_object_day_components_type 
    ON mv_object_day_components (component_type);

-- ============================================================================
-- Комментарии для документации
-- ============================================================================

COMMENT ON MATERIALIZED VIEW mv_object_days IS 
    'Активные биллинговые дни для объектов с учетом заморозок. Обновляется вручную через REFRESH MATERIALIZED VIEW.';

COMMENT ON MATERIALIZED VIEW mv_object_day_components IS 
    'Разворот по тарифным компонентам для каждого дня. Зависит от mv_object_days. Обновляется после обновления mv_object_days.';

-- ============================================================================
-- Функция для обновления материализованных представлений
-- ============================================================================

CREATE OR REPLACE FUNCTION refresh_billing_materialized_views()
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    -- Обновляем в правильном порядке (сначала базовое, затем зависимое)
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_object_days;
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_object_day_components;
    
    RAISE NOTICE 'Материализованные представления биллинга успешно обновлены';
END;
$$;

COMMENT ON FUNCTION refresh_billing_materialized_views() IS 
    'Обновляет все материализованные представления для биллинга в правильном порядке';

