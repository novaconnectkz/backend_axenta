-- Migration: Fix subscriptions table columns
-- Created: 2025-11-19
-- Description: Добавляет отсутствующие колонки в таблицу subscriptions

-- Добавляем contract_id, если отсутствует
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_schema = 'public' 
        AND table_name = 'subscriptions' 
        AND column_name = 'contract_id'
    ) THEN
        ALTER TABLE public.subscriptions 
        ADD COLUMN contract_id INTEGER;
        
        CREATE INDEX IF NOT EXISTS idx_subscriptions_contract_id 
        ON public.subscriptions(contract_id);
        
        RAISE NOTICE 'Добавлена колонка contract_id в таблицу subscriptions';
    END IF;
END $$;

-- Комментарии
COMMENT ON COLUMN public.subscriptions.contract_id IS 'ID договора, к которому привязана подписка';

