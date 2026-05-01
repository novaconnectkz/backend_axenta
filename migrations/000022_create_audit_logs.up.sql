-- Миграция для создания таблицы audit_logs
-- Создает таблицу для хранения аудит-логов действий пользователей

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    user_id VARCHAR(50),
    username VARCHAR(255),
    role VARCHAR(100),
    tenant_id VARCHAR(50),
    ip VARCHAR(45),
    user_agent VARCHAR(512),
    action VARCHAR(255) NOT NULL,
    resource VARCHAR(255),
    method VARCHAR(10),
    path VARCHAR(512),
    status_code INTEGER,
    details JSONB,
    success BOOLEAN DEFAULT true,
    level VARCHAR(20) DEFAULT 'info',
    error TEXT,
    duration_ms BIGINT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Создание индексов для оптимизации запросов
CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_id ON audit_logs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_success ON audit_logs(success);
CREATE INDEX IF NOT EXISTS idx_audit_logs_level ON audit_logs(level);
CREATE INDEX IF NOT EXISTS idx_audit_logs_deleted_at ON audit_logs(deleted_at);

-- Индекс для полнотекстового поиска по действиям
CREATE INDEX IF NOT EXISTS idx_audit_logs_action_text ON audit_logs USING gin(to_tsvector('russian', action));

-- Индекс для поиска по JSONB details
CREATE INDEX IF NOT EXISTS idx_audit_logs_details ON audit_logs USING gin(details);

-- Комментарии к таблице и колонкам
COMMENT ON TABLE audit_logs IS 'Таблица для хранения аудит-логов действий пользователей';
COMMENT ON COLUMN audit_logs.timestamp IS 'Время события';
COMMENT ON COLUMN audit_logs.user_id IS 'ID пользователя';
COMMENT ON COLUMN audit_logs.username IS 'Имя пользователя';
COMMENT ON COLUMN audit_logs.role IS 'Роль пользователя';
COMMENT ON COLUMN audit_logs.tenant_id IS 'ID компании (tenant)';
COMMENT ON COLUMN audit_logs.ip IS 'IP адрес пользователя';
COMMENT ON COLUMN audit_logs.user_agent IS 'User Agent браузера';
COMMENT ON COLUMN audit_logs.action IS 'Выполненное действие';
COMMENT ON COLUMN audit_logs.resource IS 'Ресурс над которым выполнено действие';
COMMENT ON COLUMN audit_logs.method IS 'HTTP метод запроса';
COMMENT ON COLUMN audit_logs.path IS 'Путь запроса';
COMMENT ON COLUMN audit_logs.status_code IS 'HTTP код ответа';
COMMENT ON COLUMN audit_logs.details IS 'Дополнительные детали в формате JSON';
COMMENT ON COLUMN audit_logs.success IS 'Успешность операции';
COMMENT ON COLUMN audit_logs.level IS 'Уровень лога: info, success, warning, error';
COMMENT ON COLUMN audit_logs.error IS 'Текст ошибки если есть';
COMMENT ON COLUMN audit_logs.duration_ms IS 'Длительность операции в миллисекундах';

