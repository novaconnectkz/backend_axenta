-- Создаем таблицу discounts в схеме public для управления скидками

CREATE TABLE IF NOT EXISTS discounts (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    -- Уровень применения скидки
    level VARCHAR(50) NOT NULL,  -- object, tariff, subscription, appendix, contract
    entity_id BIGINT NOT NULL,   -- ID сущности на соответствующем уровне
    
    -- Тип и размер скидки
    type VARCHAR(20) NOT NULL,   -- fixed (фиксированная сумма) или percent (%)
    value NUMERIC(15,2) NOT NULL, -- Значение скидки
    
    -- Период действия
    start_date TIMESTAMP WITH TIME ZONE NOT NULL,
    end_date TIMESTAMP WITH TIME ZONE,  -- NULL = бессрочно
    
    -- Статус
    is_active BOOLEAN DEFAULT true,
    reason TEXT  -- Причина скидки
);

-- Создаем индексы
CREATE INDEX IF NOT EXISTS idx_discounts_deleted_at ON discounts(deleted_at);
CREATE INDEX IF NOT EXISTS idx_discounts_entity ON discounts(level, entity_id);
CREATE INDEX IF NOT EXISTS idx_discounts_active ON discounts(is_active);
CREATE INDEX IF NOT EXISTS idx_discounts_dates ON discounts(start_date, end_date);

