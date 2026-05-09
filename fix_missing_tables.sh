#!/bin/bash

echo "🔧 Исправление отсутствующих таблиц в базе данных"
echo "=================================================="

# Переходим в директорию проекта
cd "$(dirname "$0")"

echo "📦 Компилируем утилиту создания таблиц..."
go build -o create_missing_tables ./cmd/create_missing_tables/

if [ $? -ne 0 ]; then
    echo "❌ Ошибка компиляции утилиты создания таблиц"
    exit 1
fi

echo "✅ Утилита скомпилирована успешно"

echo "🚀 Запускаем создание недостающих таблиц..."
./create_missing_tables

if [ $? -eq 0 ]; then
    echo ""
    echo "🎉 Таблицы созданы успешно!"
    echo "✨ Теперь эндпоинты /api/auth/roles и /api/auth/user-templates должны работать"
    
    echo ""
    echo "🧪 Тестируем эндпоинты..."
    
    # Тест эндпоинта ролей
    echo "📋 Тест /api/auth/roles:"
    curl -s -w "HTTP Status: %{http_code}\n" "https://api.acrm.su/api/auth/roles?page=1&limit=100&active_only=true" || echo "Ошибка запроса"
    
    echo ""
    echo "📋 Тест /api/auth/user-templates:"
    curl -s -w "HTTP Status: %{http_code}\n" "https://api.acrm.su/api/auth/user-templates?page=1&limit=100&active_only=true" || echo "Ошибка запроса"
    
else
    echo "❌ Ошибка при создании таблиц"
    exit 1
fi

# Удаляем временный файл
rm -f create_missing_tables

echo ""
echo "✅ Исправление завершено!"
