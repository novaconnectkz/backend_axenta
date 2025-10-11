-- Скрипт для очистки базы данных от пользователей и демо данных
-- ВНИМАНИЕ: Это удалит ВСЕ пользовательские данные!

-- Отключаем проверки внешних ключей
SET session_replication_role = replica;

-- Очищаем таблицы пользователей (в порядке зависимостей)
DELETE FROM refresh_tokens;
DELETE FROM local_users;
DELETE FROM users;

-- Очищаем роли и права (если есть демо данные)
DELETE FROM role_permissions;
DELETE FROM user_templates;
DELETE FROM roles WHERE name LIKE '%demo%' OR name LIKE '%test%';
DELETE FROM permissions WHERE name LIKE '%demo%' OR name LIKE '%test%';

-- Очищаем бизнес-данные (если есть демо)
DELETE FROM installation_equipment;
DELETE FROM installations;
DELETE FROM warehouse_operations;
DELETE FROM equipment;
DELETE FROM locations;
DELETE FROM installers;
DELETE FROM stock_alerts;

-- Очищаем биллинг
DELETE FROM invoice_items;
DELETE FROM invoices;
DELETE FROM billing_history;
DELETE FROM subscriptions;
DELETE FROM contract_appendices;
DELETE FROM contracts;

-- Очищаем отчеты
DELETE FROM report_executions;
DELETE FROM report_schedules;
DELETE FROM report_templates;
DELETE FROM reports;

-- Очищаем интеграции
DELETE FROM integration_errors;
DELETE FROM integrations WHERE name LIKE '%demo%' OR name LIKE '%test%';

-- Очищаем шаблоны
DELETE FROM monitoring_notification_templates;
DELETE FROM monitoring_templates;
DELETE FROM object_templates;

-- Включаем обратно проверки внешних ключей
SET session_replication_role = DEFAULT;

-- Сбрасываем автоинкремент для основных таблиц
ALTER SEQUENCE IF EXISTS users_id_seq RESTART WITH 1;
ALTER SEQUENCE IF EXISTS local_users_id_seq RESTART WITH 1;
ALTER SEQUENCE IF EXISTS roles_id_seq RESTART WITH 1;
ALTER SEQUENCE IF EXISTS permissions_id_seq RESTART WITH 1;
ALTER SEQUENCE IF EXISTS contracts_id_seq RESTART WITH 1;
ALTER SEQUENCE IF EXISTS invoices_id_seq RESTART WITH 1;
ALTER SEQUENCE IF EXISTS reports_id_seq RESTART WITH 1;
ALTER SEQUENCE IF EXISTS installations_id_seq RESTART WITH 1;

-- Выводим результат
SELECT 'База данных очищена!' as result;
