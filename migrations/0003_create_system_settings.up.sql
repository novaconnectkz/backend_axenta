-- Migration: Create system_settings table
-- Created: 2025-11-19

CREATE TABLE IF NOT EXISTS public.system_settings (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
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
    password_require_special BOOLEAN DEFAULT TRUE,
    max_login_attempts INTEGER DEFAULT 5,
    
    -- Настройки уведомлений
    email_notifications_enabled BOOLEAN DEFAULT TRUE,
    sms_notifications_enabled BOOLEAN DEFAULT FALSE,
    telegram_notifications_enabled BOOLEAN DEFAULT TRUE,
    
    -- Налоговые настройки
    vat_rate_preset VARCHAR(20) DEFAULT 'russia',
    vat_rate_custom DECIMAL(5,2) DEFAULT 20,
    
    -- Настройки резервного копирования
    backup_enabled BOOLEAN DEFAULT TRUE,
    backup_schedule VARCHAR(50) DEFAULT '0 2 * * *',
    backup_retention_days INTEGER DEFAULT 30,
    
    CONSTRAINT idx_system_settings_company UNIQUE (company_id)
);

-- Индексы
CREATE INDEX IF NOT EXISTS idx_system_settings_admin_account ON public.system_settings(admin_account_id);
CREATE INDEX IF NOT EXISTS idx_system_settings_deleted_at ON public.system_settings(deleted_at);

-- Комментарии
COMMENT ON TABLE public.system_settings IS 'System-wide settings for each company';
COMMENT ON COLUMN public.system_settings.vat_rate_preset IS 'VAT rate preset: russia (20%), kazakhstan (12%), none (0%), custom';
COMMENT ON COLUMN public.system_settings.vat_rate_custom IS 'Custom VAT rate percentage (used when vat_rate_preset = custom)';
