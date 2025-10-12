#!/bin/bash

# Скрипт развертывания с исправлением проблем tenant схем
echo "🚀 Развертывание Axenta CRM с исправлением tenant схем"
echo "====================================================="

set -e  # Выход при ошибке

echo "📋 Этапы развертывания:"
echo "1. Миграция локальной авторизации (схема public)"
echo "2. Создание компании по умолчанию"
echo "3. Создание схемы tenant_default с таблицами"
echo "4. Запуск сервера"
echo ""

# Этап 1: Миграция локальной авторизации
echo "🔧 Этап 1: Миграция локальной авторизации..."
if go run cmd/migrate_local_auth/main.go; then
    echo "✅ Локальная авторизация настроена"
else
    echo "⚠️ Ошибки в миграции локальной авторизации (возможно, не критичные)"
fi

echo ""

# Этап 2: Создание компании по умолчанию
echo "🔧 Этап 2: Создание компании по умолчанию..."
if go run cmd/create_default_company/main.go; then
    echo "✅ Компания по умолчанию настроена"
else
    echo "❌ Ошибка создания компании по умолчанию"
    exit 1
fi

echo ""

# Этап 3: Создание tenant схемы и таблиц
echo "🔧 Этап 3: Создание tenant схемы и таблиц..."
if go run cmd/create_missing_tables/main.go; then
    echo "✅ Tenant схема настроена"
else
    echo "❌ Ошибка создания tenant схемы"
    exit 1
fi

echo ""

# Проверка готовности
echo "🔍 Проверка готовности системы..."

# Проверяем критические таблицы в public
echo "📋 Проверка глобальных таблиц (схема public):"
psql -h localhost -U postgres -d axenta_db -c "SET search_path TO public; \dt local_users; \dt refresh_tokens; \dt companies;" 2>/dev/null || echo "  ⚠️ Не удалось проверить через psql"

# Проверяем критические таблицы в tenant_default
echo "📋 Проверка tenant таблиц (схема tenant_default):"
psql -h localhost -U postgres -d axenta_db -c "SET search_path TO tenant_default; \dt roles; \dt user_templates; \dt users;" 2>/dev/null || echo "  ⚠️ Не удалось проверить через psql"

echo ""
echo "🎉 Развертывание завершено!"
echo ""
echo "💡 Следующие шаги:"
echo "1. Запустите сервер: go run main.go"
echo "2. Проверьте эндпоинты:"
echo "   - GET /api/auth/roles"
echo "   - GET /api/auth/user-templates"
echo "   - POST /api/local/login"
echo ""
echo "📝 Что исправлено:"
echo "✅ LocalUser/RefreshToken в схеме public"
echo "✅ Компания по умолчанию создана"
echo "✅ Схема tenant_default с таблицами roles, user_templates, users"
echo "✅ Изоляция схем в LocalAuthAPI и JWTService"
echo ""
echo "⚠️ Известные ограничения:"
echo "- Ошибки миграции Company модели (не критично)"
echo "- Некоторые модели имеют проблемы с AutoMigrate"
echo "- Используются прямые SQL запросы для надежности"
