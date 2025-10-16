#!/bin/bash

# Тестовый скрипт для создания CMS пользователя
# Убедитесь, что AXENTA_API_TOKENS и AXENTA_DEFAULT_ACCOUNT_ID установлены в .env

echo "🧪 Тестирование создания CMS пользователя..."

# Читаем переменные из .env файла
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Проверяем наличие токена
if [ -z "$AXENTA_API_TOKENS" ]; then
    echo "❌ Ошибка: AXENTA_API_TOKENS не установлен"
    exit 1
fi

# Берем первый токен из списка
TOKEN=$(echo $AXENTA_API_TOKENS | cut -d',' -f1 | xargs)
echo "🔑 Используем токен: ${TOKEN:0:10}..."

# Проверяем наличие account ID
if [ -z "$AXENTA_DEFAULT_ACCOUNT_ID" ]; then
    echo "⚠️  Предупреждение: AXENTA_DEFAULT_ACCOUNT_ID не установлен, используем значение по умолчанию"
    ACCOUNT_ID="1"
else
    ACCOUNT_ID="$AXENTA_DEFAULT_ACCOUNT_ID"
fi

echo "🏢 Account ID: $ACCOUNT_ID"

# Данные для создания пользователя
USER_DATA='{
  "name": "Тестовый Пользователь",
  "username": "test_user_'$(date +%s)'",
  "email": "test'$(date +%s)'@example.com",
  "password": "testpass123",
  "hasAdminAccess": false,
  "visibleTabsNames": ["monitoring", "reports", "objects"],
  "accesses": {
    "objects": {"perms": ["view", "edit"]},
    "users": {"perms": ["view"]},
    "reports": {"perms": ["view"]},
    "monitoring": {"perms": ["view"]}
  }
}'

echo "📝 Данные пользователя:"
echo "$USER_DATA" | jq '.'

# Отправляем запрос
echo "🚀 Отправляем запрос на создание пользователя..."

RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "http://localhost:8080/api/cms/users/" \
  -H "Content-Type: application/json" \
  -H "Authorization: Token $TOKEN" \
  -d "$USER_DATA")

# Разделяем ответ и код статуса
HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$RESPONSE" | head -n -1)

echo "📊 HTTP код: $HTTP_CODE"
echo "📋 Ответ сервера:"
echo "$RESPONSE_BODY" | jq '.' 2>/dev/null || echo "$RESPONSE_BODY"

# Проверяем результат
if [ "$HTTP_CODE" -eq 201 ]; then
    echo "✅ Пользователь успешно создан!"
    
    # Извлекаем ID созданного пользователя
    USER_ID=$(echo "$RESPONSE_BODY" | jq -r '.id' 2>/dev/null)
    if [ "$USER_ID" != "null" ] && [ "$USER_ID" != "" ]; then
        echo "🆔 ID созданного пользователя: $USER_ID"
    fi
else
    echo "❌ Ошибка при создании пользователя"
    echo "💡 Проверьте:"
    echo "   - Запущен ли сервер на localhost:8080"
    echo "   - Правильно ли настроен AXENTA_API_TOKENS"
    echo "   - Есть ли база данных и миграции"
fi

echo "🏁 Тест завершен"
