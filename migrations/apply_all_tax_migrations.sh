#!/bin/bash

# Скрипт для применения всех миграций настроек налогов
# Дата: 2025-11-24

set -e

echo "🔧 Применение миграций: добавление настроек налогов..."

# Загружаем переменные окружения
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
elif [ -f env.production ]; then
    export $(cat env.production | grep -v '^#' | xargs)
fi

# Проверяем наличие psql
if ! command -v psql &> /dev/null; then
    echo "❌ Ошибка: psql не установлен"
    exit 1
fi

# Применяем миграции
echo "📝 Применение миграции для таблицы companies..."
psql -h "${DB_HOST:-localhost}" \
     -p "${DB_PORT:-5432}" \
     -U "${DB_USER:-postgres}" \
     -d "${DB_NAME:-axenta}" \
     -f migrations/20251124_add_tax_settings_to_companies.sql

echo "📝 Применение миграции для таблицы system_settings..."
psql -h "${DB_HOST:-localhost}" \
     -p "${DB_PORT:-5432}" \
     -U "${DB_USER:-postgres}" \
     -d "${DB_NAME:-axenta}" \
     -f migrations/20251124_add_tax_fields_to_system_settings.sql

echo "✅ Все миграции успешно применены!"
echo ""
echo "ℹ️  Добавлены поля в таблицы:"
echo "  companies:"
echo "    - default_tax_rate (decimal(5,2), по умолчанию 20)"
echo "    - tax_included (boolean, по умолчанию false)"
echo "  system_settings:"
echo "    - default_tax_rate (decimal(5,2), по умолчанию 20)"
echo "    - tax_included (boolean, по умолчанию false)"
echo ""
echo "📋 Следующие шаги:"
echo "  1. Перезапустите backend: make run"
echo "  2. Настройки НДС доступны в настройках компании"
echo "  3. Настройки автоматически синхронизируются между всеми таблицами"

