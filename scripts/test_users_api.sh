#!/bin/bash

# Тест API пользователей для проверки назначения ролей
echo "🧪 Тестирование API пользователей"
echo "================================="

BASE_URL="http://localhost:8080/api/auth"
TOKEN="your_token_here"

# Функция для тестирования endpoint
test_endpoint() {
    local endpoint=$1
    local description=$2
    
    echo -e "\n🔍 $description"
    echo "   GET $BASE_URL$endpoint"
    
    response=$(curl -s "$BASE_URL$endpoint" \
        -H "Authorization: Token $TOKEN" \
        -H "Content-Type: application/json")
    
    # Проверяем наличие ролей в ответе
    if echo "$response" | grep -q '"role"'; then
        echo "   ✅ Роли найдены в ответе"
        # Показываем первого пользователя с ролью
        echo "$response" | jq '.data.items[0] | {username, account_type, role}' 2>/dev/null || echo "   (Ошибка парсинга JSON)"
    else
        echo "   ❌ Роли не найдены в ответе"
        echo "$response" | head -c 200
    fi
}

# Тестируем разные endpoints
test_endpoint "/users" "Основной endpoint пользователей"
test_endpoint "/users/" "Endpoint с trailing slash"
test_endpoint "/axenta-users?type=all" "Новый endpoint Axenta пользователей"

echo -e "\n💡 Подсказка:"
echo "   Замените 'your_token_here' на реальный токен Axenta"
echo "   Убедитесь, что сервер запущен на localhost:8080"
