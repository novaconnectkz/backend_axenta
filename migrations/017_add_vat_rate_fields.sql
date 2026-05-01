-- Добавляем поля для управления ставками НДС
ALTER TABLE billing_settings
ADD COLUMN IF NOT EXISTS vat_rate_preset VARCHAR(20) DEFAULT 'russia',
ADD COLUMN IF NOT EXISTS vat_rate_custom NUMERIC(5,2) DEFAULT 20;

-- Обновляем существующие записи
UPDATE billing_settings
SET vat_rate_preset = 'russia',
    vat_rate_custom = 20
WHERE vat_rate_preset IS NULL OR vat_rate_custom IS NULL;

