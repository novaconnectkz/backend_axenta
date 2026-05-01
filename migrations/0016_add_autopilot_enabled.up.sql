-- ================================================================
-- Migration 016: Добавление поля autopilot_enabled
-- Добавляет поле для включения/отключения автопилота биллинга
-- ================================================================

-- Добавляем столбец autopilot_enabled в billing_settings
ALTER TABLE billing_settings 
ADD COLUMN IF NOT EXISTS autopilot_enabled BOOLEAN DEFAULT true;

COMMENT ON COLUMN billing_settings.autopilot_enabled IS 'Включен ли автопилот для автоматического создания подписок и счетов';

-- Обновляем существующие записи (устанавливаем true по умолчанию)
UPDATE billing_settings 
SET autopilot_enabled = true 
WHERE autopilot_enabled IS NULL;

-- Логируем результат
DO $$ 
DECLARE
    updated_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO updated_count FROM billing_settings WHERE autopilot_enabled = true;
    RAISE NOTICE 'Автопилот включен для % компаний', updated_count;
END $$;
