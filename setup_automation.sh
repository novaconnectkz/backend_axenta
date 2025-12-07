#!/bin/bash

# Скрипт для настройки автоматического выполнения всех процессов
# - Ежедневные снимки партнеров
# - Billing snapshots
# - Синхронизация аккаунтов (опционально)

echo "🔧 Настройка автоматизации процессов..."

# Определяем путь к .env файлу
if [ -f ".env.production" ]; then
    ENV_FILE=".env.production"
elif [ -f ".env" ]; then
    ENV_FILE=".env"
else
    echo "❌ Файл .env не найден!"
    exit 1
fi

echo "📝 Используем файл: $ENV_FILE"

# 1. Включаем планировщик снимков
if grep -q "^ENABLE_SNAPSHOT_SCHEDULER=" "$ENV_FILE"; then
    echo "📝 Обновляем ENABLE_SNAPSHOT_SCHEDULER=true..."
    sed -i.bak 's/^ENABLE_SNAPSHOT_SCHEDULER=.*/ENABLE_SNAPSHOT_SCHEDULER=true/' "$ENV_FILE"
else
    echo "➕ Добавляем ENABLE_SNAPSHOT_SCHEDULER=true..."
    echo "" >> "$ENV_FILE"
    echo "# Автоматический планировщик ежедневных снимков (00:30 UTC / 03:30 MSK)" >> "$ENV_FILE"
    echo "ENABLE_SNAPSHOT_SCHEDULER=true" >> "$ENV_FILE"
fi

# 2. Проверяем наличие токена для Axenta API
if ! grep -q "^AXENTA_ADMIN_TOKEN=" "$ENV_FILE"; then
    echo "⚠️  ВНИМАНИЕ: AXENTA_ADMIN_TOKEN не установлен!"
    echo "   Установите токен для доступа к Axenta API:"
    echo "   echo 'AXENTA_ADMIN_TOKEN=ваш_токен' >> $ENV_FILE"
fi

echo ""
echo "✅ Настройка завершена!"
echo ""
echo "📋 Что будет выполняться автоматически:"
echo "   1. ✅ Ежедневные снимки партнеров - каждый день в 00:30 UTC (03:30 MSK)"
echo "   2. ✅ Billing snapshots - автоматически после партнерских снимков"
echo "   3. ⚠️  Синхронизация аккаунтов - отключена (можно включить в main.go)"
echo ""
echo "🔄 Для применения изменений перезапустите backend:"
echo "   systemctl restart backend_axenta"
echo "   # или"
echo "   pm2 restart backend_axenta"
echo ""
echo "📊 Проверка настроек:"
grep -E "^(ENABLE_SNAPSHOT_SCHEDULER|AXENTA_ADMIN_TOKEN)=" "$ENV_FILE" || echo "   (не найдено)"
