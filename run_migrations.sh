#!/bin/bash

# Скрипт для выполнения миграций базы данных Axenta CRM
# Использование: ./run_migrations.sh [global-only|force|help]

set -e  # Выход при ошибке

echo "🚀 Запуск миграций Axenta CRM"
echo "================================"

# Проверяем аргументы
case "${1:-}" in
    "global-only")
        echo "📋 Режим: только глобальные миграции"
        go run cmd/migrate/main.go -global-only
        ;;
    "force")
        echo "⚠️ Принудительный режим"
        go run cmd/migrate/main.go -force
        ;;
    "help"|"-h"|"--help")
        go run cmd/migrate/main.go -help
        ;;
    "")
        echo "📋 Режим: все миграции"
        go run cmd/migrate/main.go
        ;;
    *)
        echo "❌ Неизвестный аргумент: $1"
        echo "Используйте: ./run_migrations.sh [global-only|force|help]"
        exit 1
        ;;
esac

echo ""
echo "✨ Миграции завершены!"
