#!/bin/bash

# Тестирование создания пользователя CMS

echo "Тестирование создания пользователя CMS..."

# Проверяем, что сервер запущен
if ! curl -s http://localhost:8080/ping > /dev/null; then
    echo "❌ Сервер не запущен на порту 8080"
    echo "Запускаем сервер..."
    AXENTA_API_TOKENS="test-token-123,another-token-456,5e515a8f2874fc78f31c74af45260333f2c84c35" go run main.go &
    SERVER_PID=$!
    echo "Сервер запущен с PID: $SERVER_PID"
    sleep 15
fi

echo "✅ Сервер запущен"

# Тестируем создание пользователя
echo "Создаем пользователя..."
curl -X POST http://localhost:8080/api/cms/users/ \
  -H "Content-Type: application/json" \
  -H "Authorization: Token 5e515a8f2874fc78f31c74af45260333f2c84c35" \
  -d '{
    "name": "Test User CMS Script",
    "username": "test_user_cms_script",
    "email": "test_user_cms_script@example.com",
    "password": "password123",
    "hasAdminAccess": false,
    "visibleTabsNames": ["monitoring"],
    "accesses": {
      "common": ["view"]
    }
  }' && echo ""

echo "Тест завершен"
