-- Миграция для добавления полей налогов в таблицу system_settings
-- Дата: 2025-11-24

-- Добавляем поля налогов в таблицу system_settings
ALTER TABLE system_settings 
ADD COLUMN IF NOT EXISTS default_tax_rate DECIMAL(5,2) DEFAULT 20,
ADD COLUMN IF NOT EXISTS tax_included BOOLEAN DEFAULT false;

-- Комментарии к полям
COMMENT ON COLUMN system_settings.default_tax_rate IS 'Ставка НДС по умолчанию (%)';
COMMENT ON COLUMN system_settings.tax_included IS 'НДС включен в цену';

-- Обновляем существующие записи: синхронизируем default_tax_rate с vat_rate_custom
UPDATE system_settings 
SET default_tax_rate = vat_rate_custom 
WHERE default_tax_rate IS NULL OR default_tax_rate = 0;

