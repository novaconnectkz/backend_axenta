#!/bin/bash

# Скрипт для подключения к продакшен серверу
# Использование: ./connect_production.sh

PROD_HOST="194.87.143.169"
PROD_USER="root"
PROD_PASSWORD="g-t+XM#3an2YJM"

echo "🔌 Подключение к продакшен серверу..."
echo "   Host: $PROD_HOST"
echo "   User: $PROD_USER"
echo ""

# Пробуем подключиться с использованием expect
if command -v expect &> /dev/null; then
    expect << EOF
spawn ssh -o StrictHostKeyChecking=no $PROD_USER@$PROD_HOST
expect {
    "password:" {
        send "$PROD_PASSWORD\r"
        exp_continue
    }
    "~#" {
        interact
    }
    "~$" {
        interact
    }
}
EOF
else
    echo "⚠️  expect не установлен. Подключитесь вручную:"
    echo ""
    echo "   ssh $PROD_USER@$PROD_HOST"
    echo "   Пароль: $PROD_PASSWORD"
    echo ""
    echo "Или установите expect:"
    echo "   brew install expect  # macOS"
    echo "   sudo apt-get install expect  # Linux"
fi
