-- Миграция для создания таблицы system_settings
-- Дата: 2025-11-24

-- Создаём таблицу system_settings
CREATE TABLE IF NOT EXISTS system_settings (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    
    -- Связь с компанией
    admin_account_id INTEGER NOT NULL,
    company_id INTEGER NOT NULL,
    
    -- Настройки компании
    company_name VARCHAR(255),
    company_logo TEXT,
    
    -- Региональные настройки
    timezone VARCHAR(50) DEFAULT 'Europe/Moscow',
    date_format VARCHAR(20) DEFAULT 'DD.MM.YYYY',
    currency VARCHAR(3) DEFAULT 'RUB',
    language VARCHAR(10) DEFAULT 'ru',
    theme VARCHAR(10) DEFAULT 'light',
    
    -- Настройки безопасности
    session_timeout INTEGER DEFAULT 480,
    password_min_length INTEGER DEFAULT 8,
    password_require_special BOOLEAN DEFAULT true,
    max_login_attempts INTEGER DEFAULT 5,
    
    -- Настройки уведомлений
    email_notifications_enabled BOOLEAN DEFAULT true,
    sms_notifications_enabled BOOLEAN DEFAULT false,
    telegram_notifications_enabled BOOLEAN DEFAULT true,
    
    -- Налоговые настройки
    vat_rate_preset VARCHAR(20) DEFAULT 'russia',
    vat_rate_custom DECIMAL(5,2) DEFAULT 20,
    default_tax_rate DECIMAL(5,2) DEFAULT 20,
    tax_included BOOLEAN DEFAULT false,
    
    -- Настройки резервного копирования
    backup_enabled BOOLEAN DEFAULT true,
    backup_schedule VARCHAR(50) DEFAULT '0 2 * * *',
    backup_retention_days INTEGER DEFAULT 30,
    
    UNIQUE(company_id)
);

-- Индексы
CREATE INDEX IF NOT EXISTS idx_system_settings_admin_account_id ON system_settings(admin_account_id);
CREATE INDEX IF NOT EXISTS idx_system_settings_company_id ON system_settings(company_id);
CREATE INDEX IF NOT EXISTS idx_system_settings_deleted_at ON system_settings(deleted_at);

-- Комментарии
COMMENT ON TABLE system_settings IS 'Системные настройки для каждой компании';
COMMENT ON COLUMN system_settings.default_tax_rate IS 'Ставка НДС по умолчанию (%)';
COMMENT ON COLUMN system_settings.tax_included IS 'НДС включен в цену';

