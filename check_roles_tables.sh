#!/bin/bash

# Скрипт для быстрой проверки состояния таблиц roles и user_templates
# Автор: AI Assistant

set -e

echo "🔍 Проверка состояния таблиц roles и user_templates"
echo "=================================================="

# Проверяем наличие файла конфигурации
if [ -f "env.production" ]; then
    echo "📋 Используем env.production..."
    source env.production
elif [ -f ".env" ]; then
    echo "📋 Используем .env..."
    source .env
else
    echo "❌ Файл конфигурации не найден (env.production или .env)"
    exit 1
fi

# Проверяем наличие необходимых переменных
if [ -z "$DATABASE_HOST" ] || [ -z "$DATABASE_USER" ] || [ -z "$DATABASE_PASSWORD" ] || [ -z "$DATABASE_NAME" ]; then
    echo "❌ Не все переменные базы данных настроены"
    echo "Необходимые переменные: DATABASE_HOST, DATABASE_USER, DATABASE_PASSWORD, DATABASE_NAME"
    exit 1
fi

echo "🗄️ Подключаемся к базе данных: $DATABASE_USER@$DATABASE_HOST:$DATABASE_PORT/$DATABASE_NAME"

# Проверяем наличие таблиц
echo ""
echo "📊 Проверка наличия таблиц:"

PGPASSWORD="$DATABASE_PASSWORD" psql -h "$DATABASE_HOST" -p "${DATABASE_PORT:-5432}" -U "$DATABASE_USER" -d "$DATABASE_NAME" -c "
SELECT 
    CASE 
        WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'roles') 
        THEN '✅' 
        ELSE '❌' 
    END as status,
    'roles' as table_name,
    CASE 
        WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'roles') 
        THEN (SELECT COUNT(*) FROM roles)::text
        ELSE 'не существует'
    END as record_count
UNION ALL
SELECT 
    CASE 
        WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'permissions') 
        THEN '✅' 
        ELSE '❌' 
    END,
    'permissions',
    CASE 
        WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'permissions') 
        THEN (SELECT COUNT(*) FROM permissions)::text
        ELSE 'не существует'
    END
UNION ALL
SELECT 
    CASE 
        WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'role_permissions') 
        THEN '✅' 
        ELSE '❌' 
    END,
    'role_permissions',
    CASE 
        WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'role_permissions') 
        THEN (SELECT COUNT(*) FROM role_permissions)::text
        ELSE 'не существует'
    END
UNION ALL
SELECT 
    CASE 
        WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'user_templates') 
        THEN '✅' 
        ELSE '❌' 
    END,
    'user_templates',
    CASE 
        WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'user_templates') 
        THEN (SELECT COUNT(*) FROM user_templates)::text
        ELSE 'не существует'
    END;
"

echo ""
echo "📋 Список ролей (если таблица существует):"

PGPASSWORD="$DATABASE_PASSWORD" psql -h "$DATABASE_HOST" -p "${DATABASE_PORT:-5432}" -U "$DATABASE_USER" -d "$DATABASE_NAME" -c "
SELECT 
    CASE 
        WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'roles') 
        THEN (SELECT string_agg(name || ' (' || display_name || ')', ', ') FROM roles WHERE is_active = true)
        ELSE 'Таблица roles не существует'
    END as active_roles;
"

echo ""
echo "📋 Список шаблонов пользователей (если таблица существует):"

PGPASSWORD="$DATABASE_PASSWORD" psql -h "$DATABASE_HOST" -p "${DATABASE_PORT:-5432}" -U "$DATABASE_USER" -d "$DATABASE_NAME" -c "
SELECT 
    CASE 
        WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'user_templates') 
        THEN (SELECT string_agg(name, ', ') FROM user_templates WHERE is_active = true)
        ELSE 'Таблица user_templates не существует'
    END as active_templates;
"

echo ""
echo "🎯 Рекомендации:"
echo "- Если таблицы отсутствуют (❌), запустите: ./fix_production_roles_tables.sh"
echo "- Если таблицы пустые (0 записей), проверьте миграции: ./run_production_migrations.sh"
echo "- Если все в порядке (✅), проблема может быть в коде или конфигурации"
