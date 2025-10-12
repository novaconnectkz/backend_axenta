#!/bin/bash

# Тестовый скрипт для проверки функциональности ролей Axenta
# Использование: ./scripts/test_axenta_roles.sh

echo "🚀 Тестирование функциональности ролей Axenta"
echo "=============================================="

BASE_URL="http://localhost:8080/api/auth"
TOKEN="your_axenta_token_here"

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Функция для выполнения HTTP запроса
make_request() {
    local method=$1
    local endpoint=$2
    local data=$3
    local description=$4
    
    echo -e "\n${BLUE}📡 $description${NC}"
    echo "   $method $BASE_URL$endpoint"
    
    if [ -n "$data" ]; then
        echo "   Data: $data"
        response=$(curl -s -X $method "$BASE_URL$endpoint" \
            -H "Content-Type: application/json" \
            -H "Authorization: Token $TOKEN" \
            -d "$data")
    else
        response=$(curl -s -X $method "$BASE_URL$endpoint" \
            -H "Authorization: Token $TOKEN")
    fi
    
    # Проверяем статус ответа
    if echo "$response" | grep -q '"status":"success"'; then
        echo -e "   ${GREEN}✅ Успешно${NC}"
        echo "$response" | jq '.' 2>/dev/null || echo "$response"
    else
        echo -e "   ${RED}❌ Ошибка${NC}"
        echo "$response" | jq '.' 2>/dev/null || echo "$response"
    fi
}

echo -e "\n${YELLOW}1. Создание ролей по умолчанию${NC}"
make_request "POST" "/axenta-users/ensure-roles" "" "Создание ролей по умолчанию"

echo -e "\n${YELLOW}2. Получение статистики пользователей Axenta${NC}"
make_request "GET" "/axenta-users/stats" "" "Статистика пользователей по типам"

echo -e "\n${YELLOW}3. Получение всех пользователей Axenta${NC}"
make_request "GET" "/axenta-users?type=all" "" "Все пользователи Axenta"

echo -e "\n${YELLOW}4. Получение партнеров${NC}"
make_request "GET" "/axenta-users?type=partner" "" "Пользователи-партнеры"

echo -e "\n${YELLOW}5. Получение клиентов${NC}"
make_request "GET" "/axenta-users?type=client" "" "Пользователи-клиенты"

echo -e "\n${YELLOW}6. Получение локальных пользователей${NC}"
make_request "GET" "/axenta-users?type=local" "" "Локальные пользователи"

echo -e "\n${YELLOW}7. Создание локального пользователя${NC}"
local_user_data='{
    "username": "testlocal",
    "email": "testlocal@example.com",
    "password": "testpassword123",
    "first_name": "Тестовый",
    "last_name": "Локальный",
    "phone": "+7 900 123-45-67",
    "role_id": 3
}'
make_request "POST" "/axenta-users/local" "$local_user_data" "Создание локального пользователя"

echo -e "\n${YELLOW}8. Синхронизация всех пользователей из Axenta${NC}"
make_request "POST" "/axenta-users/sync-all" "" "Массовая синхронизация пользователей"

echo -e "\n${YELLOW}9. Получение синхронизированных пользователей${NC}"
make_request "GET" "/users/synced" "" "Синхронизированные пользователи из локальной БД"

echo -e "\n${GREEN}✅ Тестирование завершено!${NC}"
echo -e "\n${BLUE}💡 Подсказки:${NC}"
echo "   - Замените 'your_axenta_token_here' на реальный токен Axenta"
echo "   - Убедитесь, что сервер запущен на localhost:8080"
echo "   - Проверьте логи сервера для детальной информации"
echo "   - Используйте jq для красивого форматирования JSON ответов"
