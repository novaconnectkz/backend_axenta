-- Миграция: Добавление полей для автоматической пролонгации договоров
-- Дата: 2025-11-17
-- Описание: Добавляет поля is_auto_renew и contract_period_months в таблицу contracts

-- Добавляем поле is_auto_renew (автоматическая пролонгация)
-- По умолчанию true для обратной совместимости
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'contracts' 
        AND column_name = 'is_auto_renew'
    ) THEN
        ALTER TABLE contracts 
        ADD COLUMN is_auto_renew BOOLEAN NOT NULL DEFAULT true;
        
        COMMENT ON COLUMN contracts.is_auto_renew IS 'Автоматическая пролонгация договора';
    END IF;
END $$;

-- Добавляем поле contract_period_months (период договора в месяцах)
-- NULL означает, что используется период из тарифа
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'contracts' 
        AND column_name = 'contract_period_months'
    ) THEN
        ALTER TABLE contracts 
        ADD COLUMN contract_period_months INTEGER;
        
        COMMENT ON COLUMN contracts.contract_period_months IS 'Период договора в месяцах (если NULL, используется период из тарифа)';
    END IF;
END $$;

-- Создаем индекс для быстрого поиска договоров с включенной автопролонгацией
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE indexname = 'idx_contracts_is_auto_renew'
    ) THEN
        CREATE INDEX idx_contracts_is_auto_renew ON contracts(is_auto_renew) 
        WHERE is_auto_renew = true AND status = 'active';
    END IF;
END $$;

