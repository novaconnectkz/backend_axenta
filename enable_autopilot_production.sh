#!/bin/bash

# Скрипт для включения автопилота на продакшене
# Обновляет поле autopilot_enabled = true в таблице billing_settings

echo "🚀 Включение автопилота на продакшене..."
echo ""

# Загрузка переменных окружения
if [ -f "env.production" ]; then
    echo "✅ Загружаем конфигурацию из env.production"
    export $(cat env.production | grep -v '^#' | xargs)
else
    echo "❌ Файл env.production не найден!"
    exit 1
fi

# Проверяем наличие необходимых переменных
if [ -z "$DB_HOST" ] || [ -z "$DB_PORT" ] || [ -z "$DB_NAME" ] || [ -z "$DB_USER" ]; then
    echo "❌ Не все переменные окружения установлены!"
    echo "Требуются: DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD"
    exit 1
fi

echo "📊 Подключение к базе данных:"
echo "  Host: $DB_HOST"
echo "  Port: $DB_PORT"
echo "  Database: $DB_NAME"
echo "  User: $DB_USER"
echo ""

# Функция для выполнения SQL запроса
execute_sql() {
    PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "$1"
}

# 1. Показываем текущее состояние автопилота для всех компаний
echo "📋 Текущее состояние автопилота:"
execute_sql "SELECT id, company_id, autopilot_enabled, created_at, updated_at FROM billing_settings ORDER BY company_id;"
echo ""

# 2. Подсчитываем, сколько компаний с отключенным автопилотом
DISABLED_COUNT=$(PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -t -c "SELECT COUNT(*) FROM billing_settings WHERE autopilot_enabled = false;")
echo "❗ Компаний с отключенным автопилотом: $DISABLED_COUNT"
echo ""

# 3. Спрашиваем подтверждение
read -p "Вы уверены, что хотите включить автопилот для ВСЕХ компаний? (yes/no): " CONFIRM

if [ "$CONFIRM" != "yes" ]; then
    echo "❌ Операция отменена"
    exit 0
fi

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

