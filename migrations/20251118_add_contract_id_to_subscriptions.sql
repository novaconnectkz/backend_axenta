-- Миграция для добавления contract_id в таблицу subscriptions
-- Добавляем связь подписки с договором

-- Добавляем колонку contract_id в таблицу subscriptions (если её ещё нет)
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
            ADD COLUMN contract_id INTEGER NULL;
        
        -- Создаем индекс для быстрого поиска подписок по договору
        CREATE INDEX IF NOT EXISTS idx_subscriptions_contract_id ON public.subscriptions(contract_id);
        
        RAISE NOTICE '✅ Добавлена колонка contract_id в таблицу subscriptions';
    ELSE
        RAISE NOTICE 'ℹ️ Колонка contract_id уже существует в таблице subscriptions';
    END IF;
END $$;

