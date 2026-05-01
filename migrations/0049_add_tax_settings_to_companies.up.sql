-- Миграция для добавления настроек налогов в таблицу companies
-- Дата: 2025-11-24

-- Добавляем поля налогов в таблицу companies
ALTER TABLE companies 
ADD COLUMN IF NOT EXISTS default_tax_rate DECIMAL(5,2) DEFAULT 20,
ADD COLUMN IF NOT EXISTS tax_included BOOLEAN DEFAULT false;

-- Комментарии к полям
COMMENT ON COLUMN companies.default_tax_rate IS 'Ставка НДС по умолчанию (%)';
COMMENT ON COLUMN companies.tax_included IS 'НДС включен в цену';

