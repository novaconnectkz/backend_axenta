-- ================================================================
-- Migration 015: Синхронизация схемы с продакшеном
-- Добавление всех отсутствующих таблиц и столбцов
-- ================================================================

-- 1. Создание отсутствующих таблиц для equipment management
-- ================================================================

-- Equipment Categories
CREATE TABLE IF NOT EXISTS equipment_categories (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    code VARCHAR(20),
    min_stock_level BIGINT DEFAULT 5,
    is_active BOOLEAN DEFAULT true
);

CREATE INDEX IF NOT EXISTS idx_equipment_categories_deleted ON equipment_categories(deleted_at);

-- Equipment
CREATE TABLE IF NOT EXISTS equipment (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    category_id BIGINT REFERENCES equipment_categories(id),
    type VARCHAR(100) NOT NULL,
    model VARCHAR(100),
    serial_number VARCHAR(100),
    imei VARCHAR(20),
    status VARCHAR(50) DEFAULT 'in_stock',
    purchase_date TIMESTAMPTZ,
    purchase_price NUMERIC,
    warranty_until TIMESTAMPTZ,
    supplier VARCHAR(200),
    current_location VARCHAR(100),
    quantity BIGINT DEFAULT 1,
    min_stock_level BIGINT DEFAULT 5,
    notes TEXT,
    last_maintenance_at TIMESTAMPTZ,
    company_id BIGINT
);

CREATE INDEX IF NOT EXISTS idx_equipment_deleted ON equipment(deleted_at);
CREATE INDEX IF NOT EXISTS idx_equipment_company ON equipment(company_id);

-- 2. Создание отсутствующих таблиц для installations
-- ================================================================

-- Locations
CREATE TABLE IF NOT EXISTS locations (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    city VARCHAR(100) NOT NULL,
    region VARCHAR(100),
    country VARCHAR(100) DEFAULT 'Russia',
    latitude NUMERIC,
    longitude NUMERIC,
    timezone VARCHAR(50) DEFAULT 'Europe/Moscow',
    is_active BOOLEAN DEFAULT true,
    notes TEXT
);

CREATE INDEX IF NOT EXISTS idx_locations_deleted ON locations(deleted_at);

-- Installers
CREATE TABLE IF NOT EXISTS installers (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    first_name VARCHAR(50) NOT NULL,
    last_name VARCHAR(50) NOT NULL,
    middle_name VARCHAR(50),
    type VARCHAR(20) NOT NULL,
    phone VARCHAR(20) NOT NULL,
    email VARCHAR(100),
    telegram_id VARCHAR(50),
    specialization TEXT[],
    skill_level VARCHAR(20) DEFAULT 'junior',
    experience BIGINT,
    location_ids BIGINT[],
    max_daily_installations BIGINT DEFAULT 3,
    working_hours_start TEXT DEFAULT '09:00',
    working_hours_end TEXT DEFAULT '18:00',
    working_days BIGINT[],
    hourly_rate NUMERIC,
    is_active BOOLEAN DEFAULT true,
    status VARCHAR(20) DEFAULT 'available',
    last_worked_at TIMESTAMPTZ,
    rating NUMERIC DEFAULT 5,
    completed_jobs BIGINT DEFAULT 0,
    notes TEXT
);

CREATE INDEX IF NOT EXISTS idx_installers_deleted ON installers(deleted_at);

-- Installations
CREATE TABLE IF NOT EXISTS installations (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    type VARCHAR(50) NOT NULL,
    status VARCHAR(50) DEFAULT 'planned',
    priority VARCHAR(20) DEFAULT 'normal',
    description TEXT,
    scheduled_at TIMESTAMPTZ NOT NULL,
    estimated_duration BIGINT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    object_id BIGINT NOT NULL,
    installer_id BIGINT NOT NULL,
    location_id BIGINT,
    client_contact VARCHAR(100),
    address TEXT,
    notes TEXT,
    result TEXT,
    created_by_user_id BIGINT,
    reminder_sent BOOLEAN DEFAULT false,
    reminder_sent_at TIMESTAMPTZ,
    notification_sent BOOLEAN DEFAULT false,
    actual_duration BIGINT,
    travel_time BIGINT,
    materials_cost NUMERIC,
    labor_cost NUMERIC,
    quality_rating NUMERIC,
    client_feedback TEXT,
    issues TEXT,
    photos TEXT[],
    cost NUMERIC,
    is_billable BOOLEAN DEFAULT true,
    company_id BIGINT
);

CREATE INDEX IF NOT EXISTS idx_installations_deleted ON installations(deleted_at);
CREATE INDEX IF NOT EXISTS idx_installations_company ON installations(company_id);

-- Installation Equipment (junction table)
CREATE TABLE IF NOT EXISTS installation_equipment (
    equipment_id BIGINT NOT NULL,
    installation_id BIGINT NOT NULL,
    PRIMARY KEY (equipment_id, installation_id)
);

-- Installer Locations (junction table)
CREATE TABLE IF NOT EXISTS installer_locations (
    installer_id BIGINT NOT NULL,
    location_id BIGINT NOT NULL,
    PRIMARY KEY (installer_id, location_id)
);

-- 3. Создание таблиц для уведомлений
-- ================================================================

-- Notification Logs
CREATE TABLE IF NOT EXISTS notification_logs (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    type TEXT NOT NULL,
    channel TEXT NOT NULL,
    recipient TEXT NOT NULL,
    subject TEXT,
    message TEXT NOT NULL,
    status TEXT DEFAULT 'pending',
    error_message TEXT,
    sent_at TIMESTAMPTZ,
    related_id BIGINT,
    related_type TEXT,
    user_id BIGINT,
    template_id BIGINT,
    attempt_count BIGINT DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    external_id TEXT,
    company_id BIGINT
);

CREATE INDEX IF NOT EXISTS idx_notification_logs_deleted ON notification_logs(deleted_at);
CREATE INDEX IF NOT EXISTS idx_notification_logs_company ON notification_logs(company_id);

-- Notification Templates
CREATE TABLE IF NOT EXISTS notification_templates (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    channel TEXT NOT NULL,
    subject TEXT,
    template TEXT NOT NULL,
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    language TEXT DEFAULT 'ru',
    priority TEXT DEFAULT 'normal',
    retry_attempts BIGINT DEFAULT 3,
    delay_seconds BIGINT DEFAULT 0,
    company_id BIGINT
);

CREATE INDEX IF NOT EXISTS idx_notification_templates_deleted ON notification_templates(deleted_at);

-- User Notification Preferences
CREATE TABLE IF NOT EXISTS user_notification_preferences (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    user_id BIGINT NOT NULL,
    telegram_enabled BOOLEAN DEFAULT true,
    email_enabled BOOLEAN DEFAULT true,
    sms_enabled BOOLEAN DEFAULT false,
    installation_reminders BOOLEAN DEFAULT true,
    installation_updates BOOLEAN DEFAULT true,
    billing_alerts BOOLEAN DEFAULT true,
    warehouse_alerts BOOLEAN DEFAULT true,
    system_notifications BOOLEAN DEFAULT true,
    quiet_hours_start TEXT DEFAULT '22:00',
    quiet_hours_end TEXT DEFAULT '08:00',
    timezone TEXT DEFAULT 'Europe/Moscow',
    company_id BIGINT
);

-- 4. Создание таблиц для отчетов
-- ================================================================

-- Report Templates
CREATE TABLE IF NOT EXISTS report_templates (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    type VARCHAR(50) NOT NULL,
    config JSONB,
    sql_query TEXT,
    parameters JSONB,
    headers JSONB,
    formatting JSONB,
    is_active BOOLEAN DEFAULT true,
    is_public BOOLEAN DEFAULT false,
    created_by_id BIGINT NOT NULL,
    company_id BIGINT
);

-- Reports
CREATE TABLE IF NOT EXISTS reports (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    type VARCHAR(50) NOT NULL,
    parameters JSONB,
    date_from TIMESTAMPTZ,
    date_to TIMESTAMPTZ,
    status VARCHAR(20) DEFAULT 'pending',
    error_msg TEXT,
    file_path VARCHAR(500),
    file_size BIGINT,
    record_count BIGINT,
    format VARCHAR(20) NOT NULL,
    created_by_id BIGINT NOT NULL,
    company_id BIGINT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration BIGINT
);

-- Report Schedules
CREATE TABLE IF NOT EXISTS report_schedules (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    type VARCHAR(20) NOT NULL,
    template_id BIGINT NOT NULL,
    cron_expression VARCHAR(100),
    time_of_day VARCHAR(10),
    day_of_week BIGINT,
    day_of_month BIGINT,
    parameters JSONB,
    format VARCHAR(20) NOT NULL,
    recipients JSONB,
    is_active BOOLEAN DEFAULT true,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    last_report_id BIGINT,
    run_count BIGINT DEFAULT 0,
    fail_count BIGINT DEFAULT 0,
    created_by_id BIGINT NOT NULL,
    company_id BIGINT
);

-- Report Executions
CREATE TABLE IF NOT EXISTS report_executions (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    schedule_id BIGINT NOT NULL,
    report_id BIGINT,
    status VARCHAR(20) DEFAULT 'pending',
    error_msg TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration BIGINT,
    emails_sent BIGINT,
    emails_failures BIGINT,
    delivery_log TEXT,
    company_id BIGINT
);

-- 5. Создание таблиц для склада
-- ================================================================

-- Stock Alerts
CREATE TABLE IF NOT EXISTS stock_alerts (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    type VARCHAR(50) NOT NULL,
    title VARCHAR(200) NOT NULL,
    description TEXT,
    severity VARCHAR(20) DEFAULT 'medium',
    equipment_id BIGINT,
    equipment_category_id BIGINT,
    status VARCHAR(20) DEFAULT 'active',
    read_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    assigned_user_id BIGINT,
    metadata JSONB,
    company_id BIGINT
);

-- Warehouse Operations
CREATE TABLE IF NOT EXISTS warehouse_operations (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    type VARCHAR(50) NOT NULL,
    description TEXT,
    status VARCHAR(20) DEFAULT 'completed',
    equipment_id BIGINT NOT NULL,
    quantity BIGINT DEFAULT 1,
    from_location VARCHAR(100),
    to_location VARCHAR(100),
    user_id BIGINT,
    document_number VARCHAR(50),
    notes TEXT,
    installation_id BIGINT,
    company_id BIGINT
);

-- 6. Создание таблиц для локальной авторизации
-- ================================================================

-- Local Users
CREATE TABLE IF NOT EXISTS local_users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    company_id VARCHAR(36) NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'user',
    email VARCHAR(255),
    name VARCHAR(255),
    is_active BOOLEAN DEFAULT true,
    last_login TIMESTAMP,
    login_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_local_users_company ON local_users(company_id);

-- Refresh Tokens
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    is_revoked BOOLEAN DEFAULT false
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);

-- User Tokens
CREATE TABLE IF NOT EXISTS user_tokens (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    user_id INTEGER NOT NULL,
    username VARCHAR(100) NOT NULL,
    token TEXT NOT NULL,
    expires_at TIMESTAMP,
    is_active BOOLEAN DEFAULT true,
    last_used_at TIMESTAMP,
    user_agent TEXT,
    ip_address VARCHAR(45)
);

-- User Accesses
CREATE TABLE IF NOT EXISTS user_accesses (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    user_id INTEGER NOT NULL,
    scope VARCHAR(100) NOT NULL,
    perms TEXT NOT NULL DEFAULT '{}',
    is_active BOOLEAN DEFAULT true
);

-- User Tabs
CREATE TABLE IF NOT EXISTS user_tabs (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    user_id INTEGER NOT NULL,
    name VARCHAR(100) NOT NULL,
    is_active BOOLEAN DEFAULT true
);

-- 7. Добавление отсутствующих столбцов в public.invoices
-- ================================================================

DO $$ BEGIN
    ALTER TABLE public.invoices ADD COLUMN IF NOT EXISTS sequential_number INTEGER DEFAULT 0;
    ALTER TABLE public.invoices ADD COLUMN IF NOT EXISTS last_sent_at TIMESTAMPTZ;
    ALTER TABLE public.invoices ADD COLUMN IF NOT EXISTS last_sent_channels TEXT;
    ALTER TABLE public.invoices ADD COLUMN IF NOT EXISTS last_sent_error TEXT;
    ALTER TABLE public.invoices ADD COLUMN IF NOT EXISTS send_channels TEXT DEFAULT 'email';
    ALTER TABLE public.invoices ADD COLUMN IF NOT EXISTS sent_count INTEGER DEFAULT 0;
    ALTER TABLE public.invoices ADD COLUMN IF NOT EXISTS send_to_email VARCHAR(100);
    ALTER TABLE public.invoices ADD COLUMN IF NOT EXISTS send_to_telegram VARCHAR(50);
    ALTER TABLE public.invoices ADD COLUMN IF NOT EXISTS send_to_max VARCHAR(50);
    RAISE NOTICE 'Added missing columns to public.invoices';
EXCEPTION
    WHEN duplicate_column THEN
        RAISE NOTICE 'Column already exists in public.invoices';
END $$;

-- 8. Добавление отсутствующих столбцов в public.subscriptions
-- ================================================================

DO $$ BEGIN
    ALTER TABLE public.subscriptions ADD COLUMN IF NOT EXISTS contract_id INTEGER;
    ALTER TABLE public.subscriptions ADD COLUMN IF NOT EXISTS sequential_number INTEGER DEFAULT 0;
    RAISE NOTICE 'Added missing columns to public.subscriptions';
EXCEPTION
    WHEN duplicate_column THEN
        RAISE NOTICE 'Column already exists in public.subscriptions';
END $$;

-- 9. Добавление отсутствующих столбцов в tenant objects (public.objects не существует на локале)
-- ================================================================

DO $$ 
DECLARE
    tenant_schema TEXT;
BEGIN
    FOR tenant_schema IN 
        SELECT schema_name 
        FROM information_schema.schemata 
        WHERE schema_name LIKE 'tenant_%'
    LOOP
        BEGIN
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS external_account_id BIGINT', tenant_schema);
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS external_account_name VARCHAR(200)', tenant_schema);
            EXECUTE format('ALTER TABLE %I.objects ADD COLUMN IF NOT EXISTS source VARCHAR(50)', tenant_schema);
            RAISE NOTICE 'Added missing columns to %.objects', tenant_schema;
        EXCEPTION
            WHEN duplicate_column THEN
                RAISE NOTICE 'Column already exists in %.objects', tenant_schema;
        END;
    END LOOP;
END $$;

-- 10. Добавление отсутствующих столбцов в integration_errors
-- ================================================================

DO $$ BEGIN
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS tenant_id BIGINT NOT NULL DEFAULT 0;
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS operation VARCHAR(50) NOT NULL DEFAULT 'unknown';
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS object_id BIGINT;
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS external_id VARCHAR(100);
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS service VARCHAR(50) NOT NULL DEFAULT 'unknown';
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS error_code VARCHAR(100);
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS retryable BOOLEAN DEFAULT true;
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS retry_count BIGINT DEFAULT 0;
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS max_retries BIGINT DEFAULT 3;
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ;
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS last_retry_at TIMESTAMPTZ;
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS request_data TEXT;
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS response_data TEXT;
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS stack_trace TEXT;
    ALTER TABLE public.integration_errors ADD COLUMN IF NOT EXISTS user_agent VARCHAR(255);
    RAISE NOTICE 'Added missing columns to public.integration_errors';
EXCEPTION
    WHEN duplicate_column THEN
        RAISE NOTICE 'Column already exists in public.integration_errors';
END $$;

-- 11. Создание недостающих tenant таблиц (monitoring_notification_templates)
-- ================================================================

DO $$ 
DECLARE
    tenant_schema TEXT;
BEGIN
    FOR tenant_schema IN 
        SELECT schema_name 
        FROM information_schema.schemata 
        WHERE schema_name LIKE 'tenant_%'
    LOOP
        EXECUTE format('
            CREATE TABLE IF NOT EXISTS %I.monitoring_notification_templates (
                id BIGSERIAL PRIMARY KEY,
                created_at TIMESTAMPTZ,
                updated_at TIMESTAMPTZ,
                deleted_at TIMESTAMPTZ,
                name VARCHAR(100) NOT NULL,
                description TEXT,
                type VARCHAR(50) NOT NULL,
                event_type VARCHAR(50) NOT NULL,
                email_subject VARCHAR(200),
                email_body TEXT,
                sms_message VARCHAR(160),
                telegram_message TEXT,
                webhook_payload TEXT,
                priority VARCHAR(20) DEFAULT ''normal'',
                retry_count BIGINT DEFAULT 3,
                retry_interval BIGINT DEFAULT 300,
                max_per_hour BIGINT DEFAULT 0,
                max_per_day BIGINT DEFAULT 0,
                active_from TIMESTAMPTZ,
                active_until TIMESTAMPTZ,
                week_days BIGINT DEFAULT 127,
                time_from VARCHAR(5),
                time_until VARCHAR(5),
                is_active BOOLEAN DEFAULT true,
                usage_count BIGINT DEFAULT 0,
                variables JSONB
            )', tenant_schema);
        
        RAISE NOTICE 'Created monitoring_notification_templates in %', tenant_schema;
    END LOOP;
END $$;

COMMENT ON COLUMN public.invoices.sequential_number IS 'Порядковый номер счета';
COMMENT ON COLUMN public.invoices.send_to_email IS 'Email адрес для отправки счета';
COMMENT ON COLUMN public.invoices.send_to_telegram IS 'Telegram ID для отправки счета';
COMMENT ON COLUMN public.invoices.send_to_max IS 'MAX messenger ID для отправки счета';
