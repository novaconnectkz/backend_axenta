-- Миграция: Добавление поля autopilot_enabled в billing_settings
-- Дата: 2025-11-23
-- Описание: Добавляет настройку автопилота для автоматизации создания договора -> подписки -> счета -> отправки

-- Добавляем поле autopilot_enabled в таблицу billing_settings
ALTER TABLE billing_settings
ADD COLUMN IF NOT EXISTS autopilot_enabled BOOLEAN DEFAULT FALSE;

-- Комментарий к полю
COMMENT ON COLUMN billing_settings.autopilot_enabled IS 'Включение режима "Автопилот" для автоматизации: после создания договора предлагается создать подписку, после подписки - счет, после счета - отправка клиенту';

