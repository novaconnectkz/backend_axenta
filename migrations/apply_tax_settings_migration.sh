#!/bin/bash

# Скрипт для применения миграции настроек налогов к компаниям
# Дата: 2025-11-24

set -e

echo "🔧 Применение миграции: добавление настроек налогов в таблицу companies..."

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

# Применяем миграцию
echo "📝 Применение миграции 20251124_add_tax_settings_to_companies.sql..."
psql -h "${DB_HOST:-localhost}" \
     -p "${DB_PORT:-5432}" \
     -U "${DB_USER:-postgres}" \
     -d "${DB_NAME:-axenta}" \
     -f migrations/20251124_add_tax_settings_to_companies.sql

echo "✅ Миграция успешно применена!"
echo ""
echo "ℹ️  Добавлены поля:"
echo "  - default_tax_rate (decimal(5,2), по умолчанию 20)"
echo "  - tax_included (boolean, по умолчанию false)"
echo ""
echo "📋 Следующие шаги:"
echo "  1. Проверьте, что поля добавлены: SELECT default_tax_rate, tax_included FROM companies LIMIT 5;"
echo "  2. Перезапустите backend для применения изменений в модели"
echo "  3. Настройки НДС теперь доступны в настройках каждой компании"

