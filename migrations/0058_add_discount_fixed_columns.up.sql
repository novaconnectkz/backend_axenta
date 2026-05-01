-- Добавление поддержки фиксированной скидки для договоров и снимков
-- Дата: 2025-12-01

-- Добавляем колонку manual_discount_fixed в таблицу contracts (tenant схемы)
-- Эта колонка будет добавлена через AutoMigrate, но добавляем для явности
ALTER TABLE contracts ADD COLUMN IF NOT EXISTS manual_discount_fixed DECIMAL(12,2) DEFAULT 0;

-- Добавляем колонку discount_fixed в таблицу partner_daily_snapshots (tenant схемы)  
ALTER TABLE partner_daily_snapshots ADD COLUMN IF NOT EXISTS discount_fixed DECIMAL(12,2) DEFAULT 0;

-- Обновляем комментарии для clarity
COMMENT ON COLUMN contracts.discount_type IS 'Тип скидки: none, manual_percent, manual_fixed, auto';
COMMENT ON COLUMN contracts.manual_discount_percent IS 'Процентная скидка (0-100%)';
COMMENT ON COLUMN contracts.manual_discount_fixed IS 'Фиксированная скидка в рублях';

COMMENT ON COLUMN partner_daily_snapshots.discount_type IS 'Тип скидки: none, manual_percent, manual_fixed, auto';
COMMENT ON COLUMN partner_daily_snapshots.discount_percent IS 'Процентная скидка (0-100%)';
COMMENT ON COLUMN partner_daily_snapshots.discount_fixed IS 'Фиксированная скидка в рублях';
