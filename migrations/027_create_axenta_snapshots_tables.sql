-- Создание таблиц для снимков Axenta Cloud аккаунтов и объектов
-- Дата: 2025-12-02

\echo 'Создание таблиц axenta_account_snapshots и axenta_object_snapshots во всех tenant схемах...'

-- Функция для создания таблиц в указанной схеме
CREATE OR REPLACE FUNCTION create_axenta_snapshot_tables(schema_name text) RETURNS void AS $$
BEGIN
    -- Создаем таблицу axenta_account_snapshots
    EXECUTE format('
        CREATE TABLE IF NOT EXISTS %I.axenta_account_snapshots (
            id SERIAL PRIMARY KEY,
            created_at TIMESTAMP NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
            deleted_at TIMESTAMP,
            
            admin_account_id INTEGER NOT NULL,
            external_account_id BIGINT NOT NULL,
            account_name VARCHAR(200),
            account_type VARCHAR(50),
            admin_fullname VARCHAR(200),
            admin_external_id BIGINT,
            parent_account_name VARCHAR(200),
            hierarchy TEXT,
            comment TEXT,
            is_active BOOLEAN DEFAULT true,
            
            blocking_datetime TIMESTAMP,
            days_before_blocking INTEGER,
            
            objects_active INTEGER DEFAULT 0,
            objects_total INTEGER DEFAULT 0,
            
            last_synced_at TIMESTAMP NOT NULL,
            raw_payload JSONB,
            
            CONSTRAINT idx_axenta_account_admin_external UNIQUE (admin_account_id, external_account_id)
        )', schema_name);
    
    -- Индексы для axenta_account_snapshots
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%I_axenta_account_admin ON %I.axenta_account_snapshots(admin_account_id)', 
        schema_name, schema_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%I_axenta_account_active ON %I.axenta_account_snapshots(is_active)', 
        schema_name, schema_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%I_axenta_account_synced ON %I.axenta_account_snapshots(last_synced_at)', 
        schema_name, schema_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%I_axenta_account_deleted ON %I.axenta_account_snapshots(deleted_at)', 
        schema_name, schema_name);
    
    -- Создаем таблицу axenta_object_snapshots
    EXECUTE format('
        CREATE TABLE IF NOT EXISTS %I.axenta_object_snapshots (
            id SERIAL PRIMARY KEY,
            created_at TIMESTAMP NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
            deleted_at TIMESTAMP,
            
            admin_account_id INTEGER NOT NULL,
            account_external_id BIGINT NOT NULL,
            external_object_id BIGINT NOT NULL,
            object_name VARCHAR(200),
            unique_id VARCHAR(100),
            device_type_name VARCHAR(200),
            account_name VARCHAR(200),
            status VARCHAR(50),
            is_active BOOLEAN DEFAULT true,
            scheduled_delete_at TIMESTAMP,
            last_communication_at TIMESTAMP,
            
            last_synced_at TIMESTAMP NOT NULL,
            raw_payload JSONB,
            
            CONSTRAINT idx_axenta_object_admin_external UNIQUE (admin_account_id, external_object_id)
        )', schema_name);
    
    -- Индексы для axenta_object_snapshots
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%I_axenta_object_admin ON %I.axenta_object_snapshots(admin_account_id)', 
        schema_name, schema_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%I_axenta_object_account ON %I.axenta_object_snapshots(account_external_id)', 
        schema_name, schema_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%I_axenta_object_active ON %I.axenta_object_snapshots(is_active)', 
        schema_name, schema_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%I_axenta_object_synced ON %I.axenta_object_snapshots(last_synced_at)', 
        schema_name, schema_name);
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%I_axenta_object_deleted ON %I.axenta_object_snapshots(deleted_at)', 
        schema_name, schema_name);
    
    RAISE NOTICE 'Таблицы снимков Axenta созданы в схеме %', schema_name;
END;
$$ LANGUAGE plpgsql;

-- Применяем для всех tenant схем
SELECT create_axenta_snapshot_tables('tenant_186');
SELECT create_axenta_snapshot_tables('tenant_default');

-- Удаляем функцию
DROP FUNCTION create_axenta_snapshot_tables(text);

\echo 'Готово!'

