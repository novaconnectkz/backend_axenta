#!/bin/bash

# Скрипт для запуска миграций на продакшене в принудительном режиме
# Автор: AI Assistant
# Дата: $(date +"%Y-%m-%d %H:%M:%S")

set -e

echo "🔄 Запуск миграций базы данных на продакшене (ПРИНУДИТЕЛЬНЫЙ РЕЖИМ)"
echo "=================================================================="

# Проверяем наличие файла конфигурации
if [ ! -f "env.production" ]; then
    echo "❌ Файл env.production не найден"
    exit 1
fi

echo "📋 Загружаем переменные окружения..."
export $(grep -v '^#' env.production | grep -v '^$' | xargs)

echo "🗄️ Подключаемся к базе данных: $DATABASE_USER@$DATABASE_HOST:$DATABASE_PORT/$DATABASE_NAME"

# Создаем резервную копию перед миграцией
echo "💾 Создаем резервную копию базы данных..."
BACKUP_FILE="backup_before_migration_$(date +%Y%m%d_%H%M%S).sql"
PGPASSWORD="$DATABASE_PASSWORD" pg_dump -h "$DATABASE_HOST" -p "${DATABASE_PORT:-5432}" -U "$DATABASE_USER" -d "$DATABASE_NAME" > "$BACKUP_FILE"
echo "✅ Резервная копия создана: $BACKUP_FILE"

# Запускаем миграции в принудительном режиме
echo "🔧 Запускаем миграции в принудительном режиме..."
go run cmd/migrate/main.go -force

echo ""
echo "✅ Миграции завершены успешно!"
echo ""
echo "📊 Проверяем созданные таблицы..."

# Проверяем наличие таблиц
PGPASSWORD="$DATABASE_PASSWORD" psql -h "$DATABASE_HOST" -p "${DATABASE_PORT:-5432}" -U "$DATABASE_USER" -d "$DATABASE_NAME" -c "
SELECT 
    schemaname,
    tablename,
    tableowner
FROM pg_tables 
WHERE tablename IN ('companies', 'user_tokens', 'users', 'roles', 'permissions')
ORDER BY schemaname, tablename;
"

echo ""
echo "🎉 Процесс миграции завершен!"
