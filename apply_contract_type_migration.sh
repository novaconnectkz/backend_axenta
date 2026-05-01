#!/bin/bash

# Применение миграции для добавления полей contract_type и partner_company_id

echo "🚀 Применение миграции для полей contract_type и partner_company_id..."

# Читаем параметры подключения из .env или используем значения по умолчанию
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_NAME="${DB_NAME:-axenta_backend}"

echo "📊 Подключение к базе данных: $DB_USER@$DB_HOST:$DB_PORT/$DB_NAME"

# Применяем миграцию 023
psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f migrations/023_add_contract_type_fields.sql

if [ $? -eq 0 ]; then
    echo "✅ Миграция 023 успешно применена"
else
    echo "❌ Ошибка при применении миграции 023"
    exit 1
fi

echo "🎉 Миграция завершена!"

