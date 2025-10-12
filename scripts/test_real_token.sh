#!/bin/bash

# Тест с реальным токеном пользователя
echo "🔐 Тестирование с реальным токеном пользователя"
echo "=============================================="

BASE_URL="http://localhost:8080/api"

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Шаг 1: Получение токена через аутентификацию${NC}"
echo "Введите данные для входа в Axenta:"
read -p "Username: " USERNAME
read -s -p "Password: " PASSWORD
echo

# Получаем токен через аутентификацию
echo -e "\n${BLUE}📡 Аутентификация в Axenta...${NC}"
LOGIN_RESPONSE=$(curl -s -X POST "$BASE_URL/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}")

echo "Ответ аутентификации:"
echo "$LOGIN_RESPONSE" | jq '.' 2>/dev/null || echo "$LOGIN_RESPONSE"

# Извлекаем токен из ответа
TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r '.data.token' 2>/dev/null)

if [ "$TOKEN" = "null" ] || [ -z "$TOKEN" ]; then
    echo -e "${RED}❌ Не удалось получить токен. Проверьте логин и пароль.${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Токен получен: ${TOKEN:0:20}...${NC}"

echo -e "\n${YELLOW}Шаг 2: Тестирование создания ролей в tenant схеме${NC}"
ROLES_RESPONSE=$(curl -s "$BASE_URL/auth/test/roles" \
    -H "Authorization: Token $TOKEN")

echo "Ответ создания ролей:"
echo "$ROLES_RESPONSE" | jq '.' 2>/dev/null || echo "$ROLES_RESPONSE"

echo -e "\n${YELLOW}Шаг 3: Тестирование пользователя с ролью${NC}"
USER_ROLE_RESPONSE=$(curl -s "$BASE_URL/auth/test/user-role" \
    -H "Authorization: Token $TOKEN")

echo "Ответ тестового пользователя:"
echo "$USER_ROLE_RESPONSE" | jq '.' 2>/dev/null || echo "$USER_ROLE_RESPONSE"

echo -e "\n${YELLOW}Шаг 4: Получение реальных пользователей${NC}"
USERS_RESPONSE=$(curl -s "$BASE_URL/auth/users?page=1&per_page=5" \
    -H "Authorization: Token $TOKEN")

echo "Ответ списка пользователей:"
echo "$USERS_RESPONSE" | jq '.data.items[0:2] | .[] | {username, account_type, role_id, role}' 2>/dev/null || echo "$USERS_RESPONSE"

# Проверяем наличие ролей
if echo "$USERS_RESPONSE" | grep -q '"role":{'; then
    echo -e "\n${GREEN}✅ Роли найдены в ответе API!${NC}"
    
    # Подсчитываем пользователей с ролями
    USERS_WITH_ROLES=$(echo "$USERS_RESPONSE" | jq '[.data.items[] | select(.role != null)] | length' 2>/dev/null || echo "0")
    TOTAL_USERS=$(echo "$USERS_RESPONSE" | jq '.data.items | length' 2>/dev/null || echo "0")
    
    echo "Пользователей с ролями: $USERS_WITH_ROLES из $TOTAL_USERS"
else
    echo -e "\n${RED}❌ Роли не найдены в ответе API${NC}"
    echo "Возможные причины:"
    echo "- Роли не созданы в tenant схеме"
    echo "- Ошибка в логике назначения ролей"
    echo "- Проблема с базой данных"
fi

echo -e "\n${BLUE}💡 Для отладки проверьте логи сервера на наличие сообщений о создании ролей${NC}"
