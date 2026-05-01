-- Миграция: Создание таблиц contracts, contract_appendices, objects, users в схеме public
-- Дата: 2026-01-15
-- Описание: Синхронизация локальной БД с продакшен сервером
-- Эти таблицы уже существуют в тенантных схемах, добавляем в public для обратной совместимости

-- Устанавливаем схему public
SET search_path TO public;

-- ============================================
-- 1. Таблица users
-- ============================================
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    -- Основные поля
    username TEXT NOT NULL,
    email TEXT NOT NULL,
    password TEXT NOT NULL,
    
    -- Дополнительные поля
    first_name TEXT,
    last_name TEXT,
    name VARCHAR(200),
    phone VARCHAR(50),
    telegram_id VARCHAR(50),
    is_active BOOLEAN DEFAULT true,
    
    -- Поля для интеграций
    user_type VARCHAR(50) DEFAULT 'user',
    external_id VARCHAR(100),
    external_source VARCHAR(50),
    
    -- Связи
    company_id BIGINT,
    role_id BIGINT,
    template_id BIGINT,
    
    -- Статистика
    last_login TIMESTAMP WITH TIME ZONE,
    login_count BIGINT DEFAULT 0,
    
    -- Axenta интеграция
    axenta_user_id VARCHAR(50),
    axenta_user_type VARCHAR(20),
    is_axenta_user BOOLEAN DEFAULT false,
    
    -- Синхронизация
    last_synced_at TIMESTAMP WITH TIME ZONE,
    sync_status VARCHAR(20) DEFAULT 'idle',
    sync_version BIGINT DEFAULT 0,
    sync_checksum TEXT,
    is_dirty BOOLEAN DEFAULT false,
    sync_error TEXT,
    source_of_truth VARCHAR(20) DEFAULT 'local',
    sync_queued_at TIMESTAMP WITH TIME ZONE,
    sync_attempted_at TIMESTAMP WITH TIME ZONE
);

-- Индексы для users
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);
CREATE INDEX IF NOT EXISTS idx_users_company_id ON users(company_id);
CREATE INDEX IF NOT EXISTS idx_users_role_id ON users(role_id);
CREATE INDEX IF NOT EXISTS idx_users_fulltext ON users USING gin(to_tsvector('russian', first_name));

-- ============================================
-- 2. Таблица contracts
-- ============================================
CREATE TABLE IF NOT EXISTS contracts (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    -- Основные поля
    number VARCHAR(50) NOT NULL,
    title VARCHAR(200) NOT NULL,
    description TEXT,
    
    -- Связи
    company_id BIGINT NOT NULL,
    tariff_plan_id BIGINT NOT NULL,
    
    -- Клиент - основные данные
    client_name VARCHAR(200) NOT NULL,
    client_inn VARCHAR(20),
    client_kpp VARCHAR(20),
    client_email VARCHAR(100),
    client_phone VARCHAR(20),
    client_address TEXT,
    
    -- Даты
    start_date TIMESTAMP WITH TIME ZONE,
    end_date TIMESTAMP WITH TIME ZONE,
    signed_at TIMESTAMP WITH TIME ZONE,
    
    -- Финансы
    total_amount NUMERIC(15,2),
    currency VARCHAR(3) DEFAULT 'RUB',
    
    -- Статус
    status VARCHAR(20) DEFAULT 'draft',
    is_active BOOLEAN DEFAULT true,
    notify_before BIGINT DEFAULT 30,
    notes TEXT,
    external_id VARCHAR(100),
    
    -- Дополнительные поля клиента
    client_short_name VARCHAR(200),
    client_type VARCHAR(20),
    client_website VARCHAR(200),
    client_legal_address TEXT,
    client_postal_address TEXT,
    client_ogrn VARCHAR(20),
    client_okpo VARCHAR(20),
    client_director VARCHAR(200),
    client_based_on VARCHAR(200),
    
    -- Банковские реквизиты
    client_bank_name VARCHAR(200),
    client_bank_bik VARCHAR(20),
    client_bank_correspondent_account VARCHAR(20),
    client_bank_account VARCHAR(20),
    client_bank_recipient VARCHAR(200),
    
    -- Паспортные данные (для физ. лиц)
    client_passport_series VARCHAR(10),
    client_passport_number VARCHAR(20),
    client_passport_issued_by TEXT,
    client_passport_issue_date VARCHAR(20),
    client_passport_department_code VARCHAR(10),
    client_registration_address TEXT,
    client_actual_address TEXT,
    client_snils VARCHAR(20),
    client_ogrn_ip VARCHAR(20),
    
    -- Нумерация
    sequential_number INTEGER DEFAULT 0,
    
    -- Синхронизация
    last_synced_at TIMESTAMP WITH TIME ZONE,
    sync_status VARCHAR(20) DEFAULT 'idle',
    sync_version BIGINT DEFAULT 0,
    sync_checksum TEXT,
    is_dirty BOOLEAN DEFAULT false,
    sync_error TEXT,
    source_of_truth VARCHAR(20) DEFAULT 'local',
    sync_queued_at TIMESTAMP WITH TIME ZONE,
    sync_attempted_at TIMESTAMP WITH TIME ZONE
);

-- Индексы для contracts
CREATE UNIQUE INDEX IF NOT EXISTS idx_contracts_number ON contracts(number);
CREATE INDEX IF NOT EXISTS idx_contracts_deleted_at ON contracts(deleted_at);
CREATE INDEX IF NOT EXISTS idx_contracts_company_id ON contracts(company_id);
CREATE INDEX IF NOT EXISTS idx_contracts_client_inn ON contracts(client_inn);
CREATE INDEX IF NOT EXISTS idx_contracts_end_date ON contracts(end_date);
CREATE INDEX IF NOT EXISTS idx_contracts_expiring ON contracts(end_date);
CREATE INDEX IF NOT EXISTS idx_contracts_sequential_number ON contracts(sequential_number);

-- ============================================
-- 3. Таблица contract_appendices
-- ============================================
CREATE TABLE IF NOT EXISTS contract_appendices (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    -- Связь с договором
    contract_id BIGINT NOT NULL,
    
    -- Основные поля
    number VARCHAR(50) NOT NULL,
    title VARCHAR(200) NOT NULL,
    description TEXT,
    
    -- Даты
    start_date TIMESTAMP WITH TIME ZONE NOT NULL,
    end_date TIMESTAMP WITH TIME ZONE NOT NULL,
    signed_at TIMESTAMP WITH TIME ZONE,
    
    -- Финансы
    amount NUMERIC(15,2),
    currency VARCHAR(3) DEFAULT 'RUB',
    
    -- Статус
    status VARCHAR(20) DEFAULT 'draft',
    is_active BOOLEAN DEFAULT true,
    notes TEXT,
    external_id VARCHAR(100)
);

-- Индексы для contract_appendices
CREATE INDEX IF NOT EXISTS idx_contract_appendices_contract_id ON contract_appendices(contract_id);
CREATE INDEX IF NOT EXISTS idx_contract_appendices_deleted_at ON contract_appendices(deleted_at);

-- FK для contract_appendices
ALTER TABLE contract_appendices 
    DROP CONSTRAINT IF EXISTS fk_contracts_appendices;
ALTER TABLE contract_appendices 
    ADD CONSTRAINT fk_contracts_appendices 
    FOREIGN KEY (contract_id) REFERENCES contracts(id);

-- ============================================
-- 4. Таблица objects
-- ============================================
CREATE TABLE IF NOT EXISTS objects (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    -- Основные поля
    name VARCHAR(100) NOT NULL,
    type VARCHAR(50) NOT NULL,
    description TEXT,
    
    -- Геолокация
    latitude NUMERIC,
    longitude NUMERIC,
    address TEXT,
    
    -- Идентификаторы устройства
    imei VARCHAR(20),
    phone_number VARCHAR(20),
    serial_number VARCHAR(50),
    
    -- Статус
    status VARCHAR(20) DEFAULT 'active',
    is_active BOOLEAN DEFAULT true,
    scheduled_delete_at TIMESTAMP WITH TIME ZONE,
    last_activity_at TIMESTAMP WITH TIME ZONE,
    
    -- Связи
    contract_id BIGINT NOT NULL,
    template_id BIGINT,
    location_id BIGINT,
    company_id BIGINT,
    
    -- Настройки
    settings JSONB,
    tags TEXT[],
    notes TEXT,
    external_id VARCHAR(100),
    
    -- Axenta интеграция
    external_account_id BIGINT,
    external_account_name VARCHAR(200),
    source VARCHAR(50),
    
    -- Синхронизация
    last_synced_at TIMESTAMP WITH TIME ZONE,
    sync_status VARCHAR(20) DEFAULT 'idle',
    sync_version BIGINT DEFAULT 0,
    sync_checksum TEXT,
    is_dirty BOOLEAN DEFAULT false,
    sync_error TEXT,
    source_of_truth VARCHAR(20) DEFAULT 'local',
    sync_queued_at TIMESTAMP WITH TIME ZONE,
    sync_attempted_at TIMESTAMP WITH TIME ZONE
);

-- Индексы для objects
CREATE UNIQUE INDEX IF NOT EXISTS idx_objects_imei ON objects(imei);
CREATE INDEX IF NOT EXISTS idx_objects_deleted_at ON objects(deleted_at);
CREATE INDEX IF NOT EXISTS idx_objects_contract_id ON objects(contract_id);
CREATE INDEX IF NOT EXISTS idx_objects_company_id ON objects(company_id);
CREATE INDEX IF NOT EXISTS idx_objects_location_id ON objects(location_id);
CREATE INDEX IF NOT EXISTS idx_objects_phone ON objects(phone_number);
CREATE INDEX IF NOT EXISTS idx_objects_scheduled_delete ON objects(scheduled_delete_at);
CREATE INDEX IF NOT EXISTS idx_objects_fulltext ON objects USING gin(to_tsvector('russian', COALESCE(name, '') || ' ' || COALESCE(description, '')));
CREATE INDEX IF NOT EXISTS idx_objects_contract ON objects(contract_id);

-- FK для objects
ALTER TABLE objects 
    DROP CONSTRAINT IF EXISTS fk_contracts_objects;
ALTER TABLE objects 
    ADD CONSTRAINT fk_contracts_objects 
    FOREIGN KEY (contract_id) REFERENCES contracts(id);

ALTER TABLE objects 
    DROP CONSTRAINT IF EXISTS fk_locations_objects;
ALTER TABLE objects 
    ADD CONSTRAINT fk_locations_objects 
    FOREIGN KEY (location_id) REFERENCES locations(id);

ALTER TABLE objects 
    DROP CONSTRAINT IF EXISTS fk_object_templates_objects;
ALTER TABLE objects 
    ADD CONSTRAINT fk_object_templates_objects 
    FOREIGN KEY (template_id) REFERENCES object_templates(id);

-- ============================================
-- 5. Добавляем FK для зависимых таблиц
-- ============================================

-- FK для equipment -> objects
ALTER TABLE equipment 
    DROP CONSTRAINT IF EXISTS fk_objects_equipment;
ALTER TABLE equipment 
    ADD CONSTRAINT fk_objects_equipment 
    FOREIGN KEY (object_id) REFERENCES objects(id) 
    ON DELETE SET NULL;

-- FK для installations -> objects  
ALTER TABLE installations 
    DROP CONSTRAINT IF EXISTS fk_objects_installations;
ALTER TABLE installations 
    ADD CONSTRAINT fk_objects_installations 
    FOREIGN KEY (object_id) REFERENCES objects(id) 
    ON DELETE SET NULL;

-- FK для installations -> users (created_by_user_id)
ALTER TABLE installations 
    DROP CONSTRAINT IF EXISTS fk_installations_created_by_user;
ALTER TABLE installations 
    ADD CONSTRAINT fk_installations_created_by_user 
    FOREIGN KEY (created_by_user_id) REFERENCES users(id) 
    ON DELETE SET NULL;

-- FK для roles -> users
ALTER TABLE users 
    DROP CONSTRAINT IF EXISTS fk_roles_users;
ALTER TABLE users 
    ADD CONSTRAINT fk_roles_users 
    FOREIGN KEY (role_id) REFERENCES roles(id) 
    ON DELETE SET NULL;

-- FK для user_templates -> users
ALTER TABLE users 
    DROP CONSTRAINT IF EXISTS fk_user_templates_users;
ALTER TABLE users 
    ADD CONSTRAINT fk_user_templates_users 
    FOREIGN KEY (template_id) REFERENCES user_templates(id) 
    ON DELETE SET NULL;

-- ============================================
-- Завершение миграции
-- ============================================
-- Обновляем комментарии к таблицам
COMMENT ON TABLE users IS 'Пользователи системы';
COMMENT ON TABLE contracts IS 'Договоры с клиентами';
COMMENT ON TABLE contract_appendices IS 'Приложения к договорам';
COMMENT ON TABLE objects IS 'Объекты мониторинга';

SELECT 'Миграция завершена успешно: созданы таблицы users, contracts, contract_appendices, objects в схеме public' as result;
