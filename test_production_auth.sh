#!/bin/bash

# 🔐 Скрипт для тестирования авторизации на продакшен API
# Использование: ./test_production_auth.sh

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

PRODUCTION_API="https://api.axenta.glonass-saratov.ru"

echo -e "${BLUE}🔐 Тестирование авторизации продакшен API${NC}"
echo -e "${BLUE}=========================================${NC}"
echo ""

# Функция для тестирования логина с разными учетными данными
test_login() {
    local username=$1
    local password=$2
    local description=$3
    
    echo -e "${BLUE}🔄 Тестирование: $description${NC}"
    echo -e "   Логин: $username"
    echo -e "   Пароль: $(echo "$password" | sed 's/./*/g')"
    
    local response
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$username\",\"password\":\"$password\"}" \
        "$PRODUCTION_API/api/auth/login")
    
    local http_code=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    local body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')
    
    case $http_code in
        200)
            echo -e "   ${GREEN}✅ Статус: $http_code (Успешная авторизация)${NC}"
            local token=$(echo "$body" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
            if [ -n "$token" ]; then
                echo -e "   ${GREEN}🔑 Токен получен: ${token:0:20}...${NC}"
                return 0
            fi
            ;;
        401)
            echo -e "   ${YELLOW}🔒 Статус: $http_code (Неверные учетные данные)${NC}"
            ;;
        500)
            echo -e "   ${RED}💥 Статус: $http_code (Ошибка сервера)${NC}"
            echo -e "   ${RED}⚠️ Возможна проблема с базой данных!${NC}"
            ;;
        *)
            echo -e "   ${YELLOW}⚠️ Статус: $http_code${NC}"
            ;;
    esac
    
    echo -e "   Ответ: $body"
    echo ""
    return $http_code
}

# Функция для тестирования API с токеном
test_with_token() {
    local token=$1
    
    echo -e "${BLUE}🧪 Тестирование API с токеном${NC}"
    
    local response
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" \
        -H "Authorization: Bearer $token" \
        "$PRODUCTION_API/api/auth/roles?page=1&limit=100&active_only=true")
    
    local http_code=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    local body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')
    
    case $http_code in
        200)
            echo -e "${GREEN}✅ API работает корректно!${NC}"
            echo -e "Ответ: $(echo "$body" | head -c 200)..."
            return 0
            ;;
        401)
            echo -e "${YELLOW}🔒 Токен недействителен или истек${NC}"
            ;;
        500)
            echo -e "${RED}💥 Ошибка сервера при обращении к API${NC}"
            echo -e "${RED}⚠️ Проблема с базой данных или кодом!${NC}"
            ;;
        *)
            echo -e "${YELLOW}⚠️ Неожиданный статус: $http_code${NC}"
            ;;
    esac
    
    echo -e "Ответ: $body"
    return $http_code
}

echo -e "${BLUE}📋 Тестирование различных учетных данных:${NC}"
echo ""

# Тестируем различные комбинации логин/пароль
CREDENTIALS=(
    "admin:admin:Стандартные admin учетные данные"
    "admin:password:Admin с паролем password"
    "admin:123456:Admin с простым паролем"
    "root:root:Root учетные данные"
    "user:user:Пользователь user"
    "test:test:Тестовые учетные данные"
    "axenta:axenta:Axenta учетные данные"
    "demo:demo:Demo учетные данные"
)

TOKEN=""
for cred in "${CREDENTIALS[@]}"; do
    IFS=':' read -r username password description <<< "$cred"
    if test_login "$username" "$password" "$description"; then
        # Если логин успешен, извлекаем токен
        response=$(curl -s -X POST \
            -H "Content-Type: application/json" \
            -d "{\"username\":\"$username\",\"password\":\"$password\"}" \
            "$PRODUCTION_API/api/auth/login")
        TOKEN=$(echo "$response" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
        break
    fi
done

echo ""
if [ -n "$TOKEN" ]; then
    echo -e "${GREEN}🎉 Токен получен успешно!${NC}"
    echo ""
    test_with_token "$TOKEN"
else
    echo -e "${YELLOW}⚠️ Не удалось получить валидный токен${NC}"
    echo ""
    echo -e "${BLUE}💡 Возможные причины:${NC}"
    echo -e "1. Учетные данные по умолчанию изменены"
    echo -e "2. Требуется создание пользователя в системе"
    echo -e "3. Проблемы с базой данных"
    echo -e "4. Проблемы с JWT конфигурацией"
    echo ""
    echo -e "${BLUE}🔧 Рекомендации:${NC}"
    echo -e "1. Проверьте логи сервера: journalctl -u axenta-backend -f"
    echo -e "2. Проверьте состояние базы данных"
    echo -e "3. Создайте пользователя через миграции"
    echo -e "4. Проверьте JWT_SECRET в переменных окружения"
fi

echo ""
echo -e "${BLUE}📊 Итоговая диагностика:${NC}"
echo -e "${BLUE}======================${NC}"

# Проверяем доступность сервера
if curl -s "$PRODUCTION_API/api/auth/login" > /dev/null; then
    echo -e "${GREEN}✅ Сервер доступен${NC}"
else
    echo -e "${RED}❌ Сервер недоступен${NC}"
fi

# Проверяем, возвращает ли сервер JSON
response=$(curl -s -X POST -H "Content-Type: application/json" -d '{}' "$PRODUCTION_API/api/auth/login")
if echo "$response" | grep -q "error"; then
    echo -e "${GREEN}✅ API возвращает корректные JSON ответы${NC}"
else
    echo -e "${RED}❌ API возвращает некорректные ответы${NC}"
fi

# Проверяем, есть ли ошибки 500
response=$(curl -s -w "%{http_code}" -X POST -H "Content-Type: application/json" -d '{"username":"test","password":"test"}' "$PRODUCTION_API/api/auth/login")
if echo "$response" | grep -q "500"; then
    echo -e "${RED}❌ Обнаружены ошибки 500 - проблема с сервером${NC}"
else
    echo -e "${GREEN}✅ Нет ошибок 500 на эндпоинте логина${NC}"
fi

echo ""
echo -e "${BLUE}🎯 Заключение:${NC}"
if [ -n "$TOKEN" ]; then
    echo -e "${GREEN}Проблема решена! API работает корректно с правильной авторизацией.${NC}"
else
    echo -e "${YELLOW}Проблема в авторизации. API работает, но нужны правильные учетные данные.${NC}"
fi
