-- Миграция: Создание таблицы для ежедневных снимков партнерских объектов
-- Дата: 2025-11-29
-- Описание: Таблица для хранения ежедневных снимков количества объектов партнеров для тарификации

-- Создаем таблицу в схеме public (глобальная таблица)
CREATE TABLE IF NOT EXISTS public.partner_daily_snapshots (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    -- Привязка
    admin_account_id INTEGER NOT NULL,
    company_id INTEGER NOT NULL,
    contract_id INTEGER NOT NULL,
    
    -- Дата снимка (начало дня в UTC)
    snapshot_date TIMESTAMP WITH TIME ZONE NOT NULL,
    
    -- ID учетной записи партнера
    partner_company_id INTEGER NOT NULL,
    
    -- Тарифный план на момент снимка
    tariff_plan_id INTEGER NOT NULL,
    
    -- Цены (сохраняем для истории)
    monthly_price DECIMAL(10,2) NOT NULL,
    daily_price DECIMAL(10,2) NOT NULL,
    
    -- Количество объектов
    total_objects_count INTEGER NOT NULL DEFAULT 0,
    active_objects_count INTEGER NOT NULL DEFAULT 0,
    
    -- Стоимость за день
    daily_cost DECIMAL(10,2) NOT NULL,
    
    -- Статус снимка
    status VARCHAR(20) DEFAULT 'completed',
    
    -- Дополнительная информация
    notes TEXT
);

-- Индексы для оптимизации запросов
CREATE INDEX IF NOT EXISTS idx_partner_snapshots_admin ON public.partner_daily_snapshots(admin_account_id);
CREATE INDEX IF NOT EXISTS idx_partner_snapshots_company ON public.partner_daily_snapshots(company_id);
CREATE INDEX IF NOT EXISTS idx_partner_snapshots_contract ON public.partner_daily_snapshots(contract_id);
CREATE INDEX IF NOT EXISTS idx_partner_snapshots_date ON public.partner_daily_snapshots(snapshot_date);
CREATE INDEX IF NOT EXISTS idx_partner_snapshots_partner ON public.partner_daily_snapshots(partner_company_id);
CREATE INDEX IF NOT EXISTS idx_partner_snapshots_deleted ON public.partner_daily_snapshots(deleted_at);

-- Уникальный индекс: один снимок на договор в день
CREATE UNIQUE INDEX IF NOT EXISTS idx_partner_snapshot_unique 
    ON public.partner_daily_snapshots(contract_id, snapshot_date) 
    WHERE deleted_at IS NULL;

-- Комментарии к таблице и колонкам
COMMENT ON TABLE public.partner_daily_snapshots IS 'Ежедневные снимки объектов партнеров для тарификации';
COMMENT ON COLUMN public.partner_daily_snapshots.snapshot_date IS 'Дата снимка (начало дня в UTC 00:00)';
COMMENT ON COLUMN public.partner_daily_snapshots.active_objects_count IS 'Количество активных объектов (для тарификации)';
COMMENT ON COLUMN public.partner_daily_snapshots.daily_price IS 'Дневная цена = monthly_price / 30';
COMMENT ON COLUMN public.partner_daily_snapshots.daily_cost IS 'Стоимость за день = daily_price * active_objects_count';

