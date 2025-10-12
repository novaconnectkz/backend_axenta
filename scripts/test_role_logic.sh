#!/bin/bash

# Простой тест логики назначения ролей без реального токена
echo "🧪 Тестирование логики назначения ролей"
echo "======================================"

BASE_URL="http://localhost:8080/api"

# Тестируем публичные endpoints (без токена)
echo -e "\n🔍 Тестирование создания ролей (публичный endpoint)..."
ROLES_RESPONSE=$(curl -s "$BASE_URL/debug/roles")

echo "Ответ создания ролей:"
echo "$ROLES_RESPONSE"

# Тестируем endpoint тестового пользователя
echo -e "\n🔍 Тестирование пользователя с ролью (публичный endpoint)..."
USER_RESPONSE=$(curl -s "$BASE_URL/debug/user-role")

echo "Ответ тестового пользователя:"
echo "$USER_RESPONSE"

# Проверяем, есть ли роли в ответах
if echo "$ROLES_RESPONSE" | grep -q '"display_name"'; then
    echo -e "\n${GREEN}✅ Роли найдены в системе!${NC}"
else
    echo -e "\n${RED}❌ Роли не найдены${NC}"
fi

if echo "$USER_RESPONSE" | grep -q '"role_data"'; then
    echo -e "${GREEN}✅ Логика назначения ролей работает!${NC}"
else
    echo -e "${RED}❌ Проблема с логикой назначения ролей${NC}"
fi

echo -e "\n${BLUE}💡 Если тесты не проходят, проверьте логи сервера${NC}"
