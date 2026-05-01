-- Миграция: Увеличение точности для стоимостей в partner_daily_snapshots
-- Дата: 2025-11-29
-- Описание: Изменение типа daily_price и daily_cost для более точных расчетов

-- Изменяем тип колонки daily_price с decimal(10,2) на decimal(12,6)
ALTER TABLE public.partner_daily_snapshots 
    ALTER COLUMN daily_price TYPE DECIMAL(12,6);

-- Изменяем тип колонки daily_cost с decimal(10,2) на decimal(12,4)
ALTER TABLE public.partner_daily_snapshots 
    ALTER COLUMN daily_cost TYPE DECIMAL(12,4);

-- Комментарии к изменениям
COMMENT ON COLUMN public.partner_daily_snapshots.daily_price IS 'Дневная цена с точностью до 6 знаков для корректных расчетов';
COMMENT ON COLUMN public.partner_daily_snapshots.daily_cost IS 'Стоимость за день с точностью до 4 знаков для прозрачности расчетов';

