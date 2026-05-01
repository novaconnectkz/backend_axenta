-- Откат миграции для таблицы audit_logs

DROP INDEX IF EXISTS idx_audit_logs_details;
DROP INDEX IF EXISTS idx_audit_logs_action_text;
DROP INDEX IF EXISTS idx_audit_logs_deleted_at;
DROP INDEX IF EXISTS idx_audit_logs_level;
DROP INDEX IF EXISTS idx_audit_logs_success;
DROP INDEX IF EXISTS idx_audit_logs_action;
DROP INDEX IF EXISTS idx_audit_logs_tenant_id;
DROP INDEX IF EXISTS idx_audit_logs_user_id;
DROP INDEX IF EXISTS idx_audit_logs_timestamp;

DROP TABLE IF EXISTS audit_logs;

