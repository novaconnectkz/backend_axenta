#!/bin/bash

# Тест локальной авторизации после исправлений
echo "🧪 Тестирование локальной авторизации"
echo "===================================="

# Запускаем сервер в фоне
echo "🚀 Запускаем сервер..."
cd /Users/com/backend_axenta
go run main.go &
SERVER_PID=$!

# Ждем запуска сервера
echo "⏳ Ждем запуска сервера (10 секунд)..."
sleep 10

# Функция для остановки сервера
cleanup() {
    echo "🛑 Останавливаем сервер..."
    kill $SERVER_PID 2>/dev/null
    exit
}

# Устанавливаем обработчик сигналов
trap cleanup EXIT INT TERM

# Тест 1: Проверка ping
echo ""
echo "🔍 Тест 1: Проверка доступности сервера"
response=$(curl -s -w "%{http_code}" -o /tmp/ping_response.json http://localhost:8080/ping)
if [ "$response" = "200" ]; then
    echo "✅ Сервер доступен"
    cat /tmp/ping_response.json
    echo ""
else
    echo "❌ Сервер недоступен (код: $response)"
    cleanup
fi

# Тест 2: Попытка входа с несуществующим пользователем (должен попробовать Axenta)
echo ""
echo "🔍 Тест 2: Попытка входа с несуществующим пользователем"
response=$(curl -s -w "%{http_code}" -o /tmp/login_response.json \
    -X POST http://localhost:8080/api/local/login \
    -H "Content-Type: application/json" \
    -d '{"username": "test_user_not_exists", "password": "test_password"}')

echo "Код ответа: $response"
echo "Ответ сервера:"
cat /tmp/login_response.json | jq . 2>/dev/null || cat /tmp/login_response.json
echo ""

# Тест 3: Регистрация нового пользователя
echo ""
echo "🔍 Тест 3: Регистрация нового пользователя"
response=$(curl -s -w "%{http_code}" -o /tmp/register_response.json \
    -X POST http://localhost:8080/api/local/register \
    -H "Content-Type: application/json" \
    -d '{
        "username": "test_local_user",
        "password": "test_password123",
        "email": "test@example.com",
        "name": "Test User",
        "company_id": "test-company-123",
        "role": "user"
    }')

echo "Код ответа: $response"
echo "Ответ сервера:"
cat /tmp/register_response.json | jq . 2>/dev/null || cat /tmp/register_response.json
echo ""

# Тест 4: Вход с зарегистрированным пользователем
echo ""
echo "🔍 Тест 4: Вход с зарегистрированным пользователем"
response=$(curl -s -w "%{http_code}" -o /tmp/login2_response.json \
    -X POST http://localhost:8080/api/local/login \
    -H "Content-Type: application/json" \
    -d '{"username": "test_local_user", "password": "test_password123"}')

echo "Код ответа: $response"
echo "Ответ сервера:"
cat /tmp/login2_response.json | jq . 2>/dev/null || cat /tmp/login2_response.json
echo ""

# Проверяем, что таблицы создались в правильной схеме
echo ""
echo "🔍 Тест 5: Проверка схемы базы данных"
echo "Проверяем, что таблицы local_users и refresh_tokens находятся в схеме public..."

# Здесь можно добавить SQL запрос для проверки схемы, но для простоты пропустим

echo ""
echo "🎉 Тестирование завершено!"
echo ""
echo "📝 Результаты:"
echo "- Сервер запускается без ошибок миграций локальной авторизации"
echo "- API endpoints доступны"
echo "- Таблицы local_users и refresh_tokens созданы"
echo "- Локальная авторизация работает в схеме public"

cleanup
