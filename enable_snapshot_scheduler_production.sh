#!/bin/bash

# Скрипт для включения планировщика снимков на продакшене
# Использование: ./enable_snapshot_scheduler_production.sh

set -e

echo "🔧 Включение планировщика снимков на продакшене..."

# Параметры подключения к продакшену
PROD_HOST="194.87.143.169"
PROD_USER="root"
PROD_PASSWORD="g-t+XM#3an2YJM"
APP_DIR="/opt/axenta/backend"

echo "📝 Шаг 1: Подключение к продакшен серверу..."

# Создаем временный скрипт для выполнения на сервере
cat > /tmp/enable_scheduler.sh << 'EOF'
#!/bin/bash
set -e

APP_DIR="/opt/axenta/backend"
ENV_FILE="$APP_DIR/.env"

echo "🔍 Проверяем текущий .env файл..."

if [ ! -f "$ENV_FILE" ]; then
    echo "❌ Файл .env не найден в $APP_DIR"
    exit 1
fi

# Проверяем, есть ли уже ENABLE_SNAPSHOT_SCHEDULER
if grep -q "^ENABLE_SNAPSHOT_SCHEDULER=" "$ENV_FILE"; then
    echo "📝 Обновляем ENABLE_SNAPSHOT_SCHEDULER=true..."
    sed -i 's/^ENABLE_SNAPSHOT_SCHEDULER=.*/ENABLE_SNAPSHOT_SCHEDULER=true/' "$ENV_FILE"
else
    echo "➕ Добавляем ENABLE_SNAPSHOT_SCHEDULER=true..."
    echo "" >> "$ENV_FILE"
    echo "# Планировщик снимков" >> "$ENV_FILE"
    echo "ENABLE_SNAPSHOT_SCHEDULER=true" >> "$ENV_FILE"
fi

# Проверяем, есть ли AXENTA_ADMIN_TOKEN
if ! grep -q "^AXENTA_ADMIN_TOKEN=" "$ENV_FILE" || grep -q "^AXENTA_ADMIN_TOKEN=$" "$ENV_FILE" || grep -q "^AXENTA_ADMIN_TOKEN=your_" "$ENV_FILE"; then
    echo "⚠️  ВНИМАНИЕ: AXENTA_ADMIN_TOKEN не установлен или имеет значение по умолчанию!"
    echo "   Планировщик снимков не сможет работать без токена."
    echo "   Установите токен вручную в файле $ENV_FILE"
fi

echo "✅ Конфигурация обновлена"
echo ""
echo "📋 Текущие настройки планировщика:"
grep -E "^(ENABLE_SNAPSHOT_SCHEDULER|AXENTA_ADMIN_TOKEN)=" "$ENV_FILE" || echo "   (не найдено)"

echo ""
echo "🔄 Для применения изменений перезапустите сервис:"
echo "   sudo systemctl restart axenta-backend"
EOF

chmod +x /tmp/enable_scheduler.sh

# Выполняем скрипт на продакшене через SSH
echo "🚀 Выполняем скрипт на продакшене..."
sshpass -p "$PROD_PASSWORD" ssh -o StrictHostKeyChecking=no "$PROD_USER@$PROD_HOST" 'bash -s' < /tmp/enable_scheduler.sh

echo ""
echo "✅ Готово! Теперь перезапустите сервис на продакшене:"
echo "   ssh $PROD_USER@$PROD_HOST"
echo "   sudo systemctl restart axenta-backend"
echo "   sudo systemctl status axenta-backend"

# Удаляем временный скрипт
rm -f /tmp/enable_scheduler.sh
