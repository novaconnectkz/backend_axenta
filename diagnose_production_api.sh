#!/bin/bash

# 🔍 Скрипт для диагностики продакшен API Axenta CRM
# Использование: ./diagnose_production_api.sh [--token TOKEN]

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Конфигурация
PRODUCTION_API="https://api.axenta.glonass-saratov.ru"
TOKEN=""

echo -e "${BLUE}🔍 Диагностика продакшен API Axenta CRM${NC}"
echo -e "${BLUE}=====================================${NC}"
echo ""

# Функция для показа справки
show_help() {
    echo "Использование: $0 [опции]"
    echo ""
    echo "Опции:"
    echo "  --token TOKEN    JWT токен для авторизации"
    echo "  --help, -h       Показать эту справку"
    echo ""
    echo "Примеры:"
    echo "  $0"
    echo "  $0 --token eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
    exit 0
}

# Функция для тестирования эндпоинта
test_endpoint() {
    local endpoint=$1
    local method=${2:-GET}
    local description=$3
    local auth_required=${4:-true}
    
    echo -e "${BLUE}🔄 Тестирование: $description${NC}"
    echo -e "   Эндпоинт: $endpoint"
    echo -e "   Метод: $method"
    
    local headers=""
    if [ "$auth_required" = "true" ] && [ -n "$TOKEN" ]; then
        headers="-H \"Authorization: Bearer $TOKEN\""
    fi
    
    local response
    local http_code
    
    if [ "$method" = "GET" ]; then
        response=$(eval "curl -s -w \"HTTPSTATUS:%{http_code}\" $headers \"$PRODUCTION_API$endpoint\"")
    else
        response=$(eval "curl -s -w \"HTTPSTATUS:%{http_code}\" -X $method $headers -H \"Content-Type: application/json\" \"$PRODUCTION_API$endpoint\"")
    fi
    
    http_code=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')
    
    case $http_code in
        200)
            echo -e "   ${GREEN}✅ Статус: $http_code (OK)${NC}"
            ;;
        401)
            echo -e "   ${YELLOW}🔒 Статус: $http_code (Unauthorized)${NC}"
            ;;
        404)
            echo -e "   ${RED}❌ Статус: $http_code (Not Found)${NC}"
            ;;
        500)
            echo -e "   ${RED}💥 Статус: $http_code (Internal Server Error)${NC}"
            ;;
        *)
            echo -e "   ${YELLOW}⚠️ Статус: $http_code${NC}"
            ;;
    esac
    
    # Показываем тело ответа если оно короткое
    if [ ${#body} -lt 200 ] && [ -n "$body" ]; then
        echo -e "   Ответ: $body"
    elif [ ${#body} -ge 200 ]; then
        echo -e "   Ответ: $(echo "$body" | head -c 100)..."
    fi
    
    echo ""
    return $http_code
}

# Функция для получения токена (если возможно)
try_get_token() {
    echo -e "${BLUE}🔑 Попытка получения токена для тестирования...${NC}"
    
    # Пробуем с тестовыми данными
    local test_credentials='{"username":"admin","password":"admin"}'
    local response
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d "$test_credentials" \
        "$PRODUCTION_API/api/auth/login")
    
    local http_code=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    local body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')
    
    if [ "$http_code" = "200" ]; then
        TOKEN=$(echo "$body" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
        if [ -n "$TOKEN" ]; then
            echo -e "${GREEN}✅ Токен получен успешно${NC}"
            return 0
        fi
    fi
    
    echo -e "${YELLOW}⚠️ Не удалось получить токен автоматически${NC}"
    echo -e "${YELLOW}💡 Используйте --token для указания токена вручную${NC}"
    return 1
}

# Парсинг аргументов
while [[ $# -gt 0 ]]; do
    case $1 in
        --token)
            TOKEN="$2"
            shift 2
            ;;
        --help|-h)
            show_help
            ;;
        *)
            echo -e "${RED}❌ Неизвестный аргумент: $1${NC}"
            echo "Используйте --help для справки"
            exit 1
            ;;
    esac
done

echo -e "${BLUE}🌐 API сервер: $PRODUCTION_API${NC}"
if [ -n "$TOKEN" ]; then
    echo -e "${GREEN}🔑 Токен предоставлен${NC}"
else
    echo -e "${YELLOW}🔑 Токен не предоставлен${NC}"
fi
echo ""

# Пробуем получить токен автоматически если не предоставлен
if [ -z "$TOKEN" ]; then
    try_get_token
    echo ""
fi

# Тестируем различные эндпоинты
echo -e "${BLUE}📋 Тестирование эндпоинтов:${NC}"
echo ""

# 1. Базовые эндпоинты (без авторизации)
test_endpoint "/api/auth/login" "POST" "Эндпоинт логина" false
test_endpoint "/api/health" "GET" "Health check" false
test_endpoint "/" "GET" "Корневой эндпоинт" false

# 2. Эндпоинты с авторизацией
test_endpoint "/api/auth/roles" "GET" "Список ролей (основная проблема)"
test_endpoint "/api/auth/roles?page=1&limit=100&active_only=true" "GET" "Список ролей с параметрами"
test_endpoint "/api/auth/user-templates" "GET" "Шаблоны пользователей"
test_endpoint "/api/auth/permissions" "GET" "Разрешения"
test_endpoint "/api/companies" "GET" "Компании"
test_endpoint "/api/objects" "GET" "Объекты"

# Сводка
echo -e "${BLUE}📊 Сводка диагностики:${NC}"
echo -e "${BLUE}=====================${NC}"

if [ -n "$TOKEN" ]; then
    echo -e "${GREEN}✅ Авторизация настроена${NC}"
else
    echo -e "${YELLOW}⚠️ Авторизация не настроена${NC}"
    echo -e "${YELLOW}💡 Для полной диагностики требуется валидный JWT токен${NC}"
fi

echo ""
echo -e "${BLUE}💡 Рекомендации:${NC}"
echo -e "1. Если API возвращает 401 - проблема в авторизации"
echo -e "2. Если API возвращает 500 - проблема в базе данных или коде"
echo -e "3. Если API возвращает 404 - эндпоинт не найден"
echo -e "4. Проверьте логи сервера для детальной диагностики"

echo ""
echo -e "${BLUE}🔧 Для получения токена:${NC}"
echo -e "1. Войдите в систему через веб-интерфейс"
echo -e "2. Откройте Developer Tools -> Network"
echo -e "3. Скопируйте Authorization header из любого запроса"
echo -e "4. Запустите: $0 --token YOUR_TOKEN"
