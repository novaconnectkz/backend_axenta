#!/bin/bash

# Скрипт для включения автопилота в локальной базе данных (которая может содержать продакшн данные)
# Обновляет поле autopilot_enabled = true в таблице billing_settings

echo "🚀 Включение автопилота в базе данных..."
echo ""

# Параметры подключения к локальной БД
DB_HOST="localhost"
DB_PORT="5432"
DB_NAME="axenta_db"
DB_USER="postgres"

echo "📊 Подключение к базе данных:"
echo "  Host: $DB_HOST"
echo "  Port: $DB_PORT"
echo "  Database: $DB_NAME"
echo "  User: $DB_USER"
echo ""

# Функция для выполнения SQL запроса
execute_sql() {
    psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "$1"
}

# 1. Показываем текущее состояние автопилота для всех компаний
echo "📋 Текущее состояние автопилота:"
execute_sql "SELECT id, company_id, autopilot_enabled, created_at, updated_at FROM billing_settings ORDER BY company_id;"
echo ""

# 2. Подсчитываем, сколько компаний с отключенным автопилотом
DISABLED_COUNT=$(psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -t -c "SELECT COUNT(*) FROM billing_settings WHERE autopilot_enabled = false;")
echo "❗ Компаний с отключенным автопилотом: $DISABLED_COUNT"
echo ""

# 3. Спрашиваем подтверждение
echo "Внимание! Вы собираетесь включить автопилот для ВСЕХ компаний."
echo "Вы уже подтвердили действие с помощью 'yes'"
echo ""

echo "🔄 Включаем автопилот для всех компаний..."

# 4. Обновляем настройки автопилота
execute_sql "UPDATE billing_settings SET autopilot_enabled = true, updated_at = NOW() WHERE autopilot_enabled = false;"

# 5. Проверяем результат
echo ""
echo "✅ Автопилот включен!"
echo ""
echo "📋 Обновленное состояние:"
execute_sql "SELECT id, company_id, autopilot_enabled, updated_at FROM billing_settings ORDER BY company_id;"

echo ""
echo "🎉 Готово! Автопилот успешно включен для всех компаний."

