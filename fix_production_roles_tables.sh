#!/bin/bash

# Скрипт для исправления отсутствующих таблиц roles и user_templates на продакшене
# Автор: AI Assistant
# Дата: $(date +"%Y-%m-%d %H:%M:%S")

set -e

echo "🔧 Исправление отсутствующих таблиц roles и user_templates на продакшене"
echo "=================================================================="

# Проверяем наличие файла конфигурации
if [ ! -f "env.production" ]; then
    echo "❌ Файл env.production не найден"
    exit 1
fi

echo "📋 Загружаем переменные окружения..."
source env.production

# Проверяем наличие необходимых переменных
if [ -z "$DATABASE_HOST" ] || [ -z "$DATABASE_USER" ] || [ -z "$DATABASE_PASSWORD" ] || [ -z "$DATABASE_NAME" ]; then
    echo "❌ Не все переменные базы данных настроены"
    exit 1
fi

echo "🗄️ Подключаемся к базе данных: $DATABASE_USER@$DATABASE_HOST:$DATABASE_PORT/$DATABASE_NAME"

# SQL скрипт для создания отсутствующих таблиц
SQL_SCRIPT=$(cat << 'EOF'
-- Создание таблицы permissions если не существует
CREATE TABLE IF NOT EXISTS permissions (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    name VARCHAR(100) UNIQUE NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    description TEXT,
    resource VARCHAR(50) NOT NULL,
    action VARCHAR(50) NOT NULL,
    category VARCHAR(50),
    is_active BOOLEAN DEFAULT true
);

-- Создание таблицы roles если не существует
CREATE TABLE IF NOT EXISTS roles (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    name VARCHAR(100) UNIQUE NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    description TEXT,
    color VARCHAR(7),
    priority INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    is_system BOOLEAN DEFAULT false
);

-- Создание связующей таблицы role_permissions если не существует
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id INTEGER NOT NULL,
    permission_id INTEGER NOT NULL,
    PRIMARY KEY (role_id, permission_id),
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE
);

-- Создание таблицы user_templates если не существует
CREATE TABLE IF NOT EXISTS user_templates (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    role_id INTEGER,
    settings JSONB,
    is_active BOOLEAN DEFAULT true
);

-- Создание индексов для оптимизации
CREATE INDEX IF NOT EXISTS idx_permissions_resource ON permissions(resource);
CREATE INDEX IF NOT EXISTS idx_permissions_action ON permissions(action);
CREATE INDEX IF NOT EXISTS idx_permissions_category ON permissions(category);
CREATE INDEX IF NOT EXISTS idx_permissions_is_active ON permissions(is_active);

CREATE INDEX IF NOT EXISTS idx_roles_name ON roles(name);
CREATE INDEX IF NOT EXISTS idx_roles_is_active ON roles(is_active);
CREATE INDEX IF NOT EXISTS idx_roles_priority ON roles(priority);

CREATE INDEX IF NOT EXISTS idx_user_templates_name ON user_templates(name);
CREATE INDEX IF NOT EXISTS idx_user_templates_role_id ON user_templates(role_id);
CREATE INDEX IF NOT EXISTS idx_user_templates_is_active ON user_templates(is_active);

-- Вставка базовых ролей если их нет
INSERT INTO roles (name, display_name, description, color, priority, is_active, is_system) 
VALUES 
    ('admin', 'Администратор', 'Полный доступ ко всем функциям системы', '#dc2626', 100, true, true),
    ('manager', 'Менеджер', 'Управление объектами и пользователями', '#059669', 80, true, true),
    ('tech', 'Техник', 'Работа с оборудованием и установками', '#2563eb', 60, true, true),
    ('accountant', 'Бухгалтер', 'Просмотр отчетов и истории', '#7c3aed', 40, true, true),
    ('user', 'Пользователь', 'Базовый доступ к системе', '#6b7280', 20, true, true)
ON CONFLICT (name) DO NOTHING;

-- Вставка базовых разрешений если их нет
INSERT INTO permissions (name, display_name, description, resource, action, category, is_active) 
VALUES 
    ('users.create', 'Создание пользователей', 'Создание новых пользователей', 'users', 'create', 'users', true),
    ('users.read', 'Просмотр пользователей', 'Просмотр списка пользователей', 'users', 'read', 'users', true),
    ('users.update', 'Редактирование пользователей', 'Редактирование данных пользователей', 'users', 'update', 'users', true),
    ('users.delete', 'Удаление пользователей', 'Удаление пользователей', 'users', 'delete', 'users', true),
    ('objects.create', 'Создание объектов', 'Создание новых объектов мониторинга', 'objects', 'create', 'objects', true),
    ('objects.read', 'Просмотр объектов', 'Просмотр списка объектов', 'objects', 'read', 'objects', true),
    ('objects.update', 'Редактирование объектов', 'Редактирование данных объектов', 'objects', 'update', 'objects', true),
    ('objects.delete', 'Удаление объектов', 'Удаление объектов', 'objects', 'delete', 'objects', true),
    ('warehouse.create', 'Создание складских операций', 'Создание складских операций', 'warehouse', 'create', 'warehouse', true),
    ('warehouse.read', 'Просмотр склада', 'Просмотр складских операций', 'warehouse', 'read', 'warehouse', true),
    ('warehouse.update', 'Редактирование склада', 'Редактирование складских операций', 'warehouse', 'update', 'warehouse', true),
    ('warehouse.delete', 'Удаление складских операций', 'Удаление складских операций', 'warehouse', 'delete', 'warehouse', true)
ON CONFLICT (name) DO NOTHING;

-- Назначение разрешений ролям
-- Администратор получает все разрешения
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'admin'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Менеджер получает разрешения на пользователей и объекты
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'manager' AND p.resource IN ('users', 'objects')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Техник получает разрешения на объекты и склад
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'tech' AND p.resource IN ('objects', 'warehouse')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Бухгалтер получает только чтение
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'accountant' AND p.action = 'read'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Пользователь получает только чтение объектов
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'user' AND p.resource = 'objects' AND p.action = 'read'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Создание базовых шаблонов пользователей
INSERT INTO user_templates (name, description, role_id, settings, is_active)
SELECT 
    'Администратор',
    'Шаблон для создания администраторов системы',
    r.id,
    '{"notifications": {"email": true, "telegram": true}, "dashboard": {"default_view": "admin"}}'::jsonb,
    true
FROM roles r WHERE r.name = 'admin'
ON CONFLICT DO NOTHING;

INSERT INTO user_templates (name, description, role_id, settings, is_active)
SELECT 
    'Менеджер',
    'Шаблон для создания менеджеров',
    r.id,
    '{"notifications": {"email": true, "telegram": false}, "dashboard": {"default_view": "manager"}}'::jsonb,
    true
FROM roles r WHERE r.name = 'manager'
ON CONFLICT DO NOTHING;

INSERT INTO user_templates (name, description, role_id, settings, is_active)
SELECT 
    'Техник',
    'Шаблон для создания техников',
    r.id,
    '{"notifications": {"email": false, "telegram": true}, "dashboard": {"default_view": "tech"}}'::jsonb,
    true
FROM roles r WHERE r.name = 'tech'
ON CONFLICT DO NOTHING;

-- Проверка созданных таблиц
SELECT 'roles' as table_name, COUNT(*) as record_count FROM roles
UNION ALL
SELECT 'permissions', COUNT(*) FROM permissions
UNION ALL
SELECT 'role_permissions', COUNT(*) FROM role_permissions
UNION ALL
SELECT 'user_templates', COUNT(*) FROM user_templates;
EOF
)

echo "🔧 Выполняем создание таблиц и данных..."

# Выполняем SQL скрипт
PGPASSWORD="$DATABASE_PASSWORD" psql -h "$DATABASE_HOST" -p "${DATABASE_PORT:-5432}" -U "$DATABASE_USER" -d "$DATABASE_NAME" -c "$SQL_SCRIPT"

echo ""
echo "✅ Таблицы успешно созданы и заполнены!"
echo ""
echo "📊 Результат:"
echo "- Таблица roles создана"
echo "- Таблица permissions создана" 
echo "- Таблица role_permissions создана"
echo "- Таблица user_templates создана"
echo "- Базовые роли добавлены"
echo "- Базовые разрешения добавлены"
echo "- Связи между ролями и разрешениями установлены"
echo "- Шаблоны пользователей созданы"
echo ""
echo "🎉 Исправление завершено успешно!"
