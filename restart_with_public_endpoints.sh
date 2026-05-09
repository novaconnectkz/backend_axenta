#!/bin/bash

echo "🔧 Перекомпиляция и перезапуск сервера с публичными эндпоинтами"
echo "=============================================================="

# Переходим в директорию проекта
cd "$(dirname "$0")"

echo "📦 Компилируем сервер..."
go build -o main .

if [ $? -ne 0 ]; then
    echo "❌ Ошибка компиляции сервера"
    exit 1
fi

echo "✅ Сервер скомпилирован успешно"

echo "🚀 Запускаем сервер в фоновом режиме..."
nohup ./main > server_public_endpoints.log 2>&1 &
SERVER_PID=$!

echo "⏳ Ждем запуска сервера (5 секунд)..."
sleep 5

echo "🧪 Тестируем новые публичные эндпоинты..."

echo "📋 Тест /api/public/roles:"
curl -s -w "HTTP Status: %{http_code}\n" "https://api.acrm.su/api/public/roles?page=1&limit=100&active_only=true" || echo "Ошибка запроса"

echo ""
echo "📋 Тест /api/public/user-templates:"
curl -s -w "HTTP Status: %{http_code}\n" "https://api.acrm.su/api/public/user-templates?page=1&limit=100&active_only=true" || echo "Ошибка запроса"

echo ""
echo "✅ Тестирование завершено!"
echo "📝 Логи сервера: server_public_endpoints.log"
echo "🆔 PID сервера: $SERVER_PID"
