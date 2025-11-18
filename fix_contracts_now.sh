#!/bin/bash

# 🚨 СРОЧНАЯ МИГРАЦИЯ БД
# Две простые SQL команды для исправления ошибки создания договоров

echo "🔧 Применение миграции к продакшен БД..."
echo ""
echo "⚠️  Эта команда выполнит:"
echo "   ALTER TABLE contracts ALTER COLUMN start_date DROP NOT NULL;"
echo "   ALTER TABLE contracts ALTER COLUMN end_date DROP NOT NULL;"
echo ""

# Вы можете указать свои данные подключения здесь:
# или передать через переменные окружения

# Пример 1: Если БД находится на том же сервере, что и бэкенд
# psql -h localhost -U axenta_user -d axenta_db

# Пример 2: Если БД находится на удаленном сервере
# psql -h api.axenta.glonass-saratov.ru -U your_user -d axenta_db

# Пример 3: Через SSH тоннель
# ssh -L 5433:localhost:5432 user@api.axenta.glonass-saratov.ru
# psql -h localhost -p 5433 -U axenta_user -d axenta_db

echo "📝 Введите данные подключения к продакшен БД:"
read -p "Хост (например, localhost или api.axenta.glonass-saratov.ru): " DB_HOST
read -p "Порт [5432]: " DB_PORT
DB_PORT=${DB_PORT:-5432}
read -p "База данных [axenta_db]: " DB_NAME
DB_NAME=${DB_NAME:-axenta_db}
read -p "Пользователь [axenta_user]: " DB_USER
DB_USER=${DB_USER:-axenta_user}
echo ""

echo "🔄 Подключаюсь к $DB_HOST:$DB_PORT/$DB_NAME..."
echo ""

# SQL команды
SQL="
BEGIN;

ALTER TABLE contracts ALTER COLUMN start_date DROP NOT NULL;
ALTER TABLE contracts ALTER COLUMN end_date DROP NOT NULL;

COMMENT ON COLUMN contracts.start_date IS 'Дата начала действия договора. Устанавливается через подписку.';
COMMENT ON COLUMN contracts.end_date IS 'Дата окончания действия договора. Устанавливается через подписку.';

COMMIT;

-- Проверка
SELECT column_name, is_nullable FROM information_schema.columns 
WHERE table_name = 'contracts' AND column_name IN ('start_date', 'end_date');
"

# Выполнение
psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "$SQL"

if [ $? -eq 0 ]; then
    echo ""
    echo "✅ ✅ ✅ МИГРАЦИЯ УСПЕШНО ПРИМЕНЕНА! ✅ ✅ ✅"
    echo ""
    echo "🎉 Теперь можете:"
    echo "   1. Обновить страницу (F5)"
    echo "   2. Попробовать создать договор"
    echo "   3. Ошибка должна исчезнуть!"
    echo ""
else
    echo ""
    echo "❌ Ошибка при применении миграции"
    echo ""
    echo "Возможные причины:"
    echo "  - Неверные данные подключения"
    echo "  - Нет доступа к БД"
    echo "  - Недостаточно прав у пользователя"
    echo ""
    echo "Попробуйте:"
    echo "  1. Проверить данные подключения"
    echo "  2. Подключиться с правами администратора (postgres)"
    echo "  3. Использовать SSH тоннель"
    exit 1
fi

