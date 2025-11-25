#!/bin/bash

# Скрипт для применения миграции добавления subscription_id в contract_objects
# Дата: 2025-11-25

set -e

echo "🚀 Применение миграции: добавление subscription_id в contract_objects"
echo "================================================================"

# Загружаем переменные окружения
if [ -f .env ]; then
    source .env
    echo "✅ Переменные окружения загружены из .env"
else
    echo "❌ Файл .env не найден"
    exit 1
fi

# Проверяем наличие необходимых переменных
if [ -z "$DB_HOST" ] || [ -z "$DB_PORT" ] || [ -z "$DB_NAME" ] || [ -z "$DB_USER" ] || [ -z "$DB_PASSWORD" ]; then
    echo "❌ Не все переменные окружения установлены"
    echo "Требуются: DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD"
    exit 1
fi

# Формируем строку подключения
export PGPASSWORD="$DB_PASSWORD"
DB_URL="postgresql://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"

echo "📊 Подключение к базе данных: ${DB_HOST}:${DB_PORT}/${DB_NAME}"
echo ""

# Применяем миграцию
echo "📝 Применение миграции..."
psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f migrations/20251125_add_subscription_id_to_contract_objects.sql

if [ $? -eq 0 ]; then
    echo ""
    echo "✅ Миграция успешно применена!"
    echo ""
    echo "📋 Что было сделано:"
    echo "   - Добавлено поле subscription_id в таблицу contract_objects всех tenant-схем"
    echo "   - Созданы индексы для улучшения производительности"
    echo ""
    echo "⚠️  Внимание:"
    echo "   - Существующие записи будут иметь subscription_id = NULL"
    echo "   - Новые подписки будут автоматически привязывать объекты через subscription_id"
    echo "   - Теперь каждая подписка видит только свои объекты"
    echo ""
else
    echo ""
    echo "❌ Ошибка при применении миграции"
    exit 1
fi

# Очищаем пароль из переменных окружения
unset PGPASSWORD

echo "✨ Готово!"

