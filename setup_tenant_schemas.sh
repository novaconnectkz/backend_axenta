#!/bin/bash

# Скрипт для настройки tenant схем для всех компаний
echo "🏢 Настройка tenant схем Axenta CRM"
echo "==================================="

set -e  # Выход при ошибке

echo "🔧 Запускаем утилиту настройки tenant схем..."
go run cmd/setup_tenant_schemas/main.go

echo ""
echo "✨ Настройка завершена!"
echo ""
echo "💡 Следующие шаги:"
echo "1. Проверьте, что все схемы созданы успешно"
echo "2. Запустите сервер: go run main.go"
echo "3. Проверьте работу эндпоинтов /api/auth/roles и /api/auth/user-templates"
