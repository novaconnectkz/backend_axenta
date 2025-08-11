#!/bin/bash

echo "🚀 Выгружаем Backend на GitHub..."

# Проверяем статус Git
echo "📋 Статус Git:"
git status

# Выгружаем код
echo "📤 Выгружаем на GitHub..."
git push -u origin main

if [ $? -eq 0 ]; then
    echo "✅ Backend успешно выгружен на GitHub!"
    echo "🔗 Репозиторий: https://github.com/novaconnectkz/backend_axenta"
else
    echo "❌ Ошибка при выгрузке. Проверьте:"
    echo "1. Создан ли репозиторий на GitHub"
    echo "2. Правильно ли настроен remote"
    echo "3. Есть ли права доступа"
fi
