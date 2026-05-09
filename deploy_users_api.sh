#!/bin/bash

# 🚀 Скрипт развертывания новых endpoints пользователей на продакшн
# Добавляет прокси для загрузки пользователей из Axenta Cloud API

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_header() {
    echo -e "${BLUE}"
    echo "================================================================"
    echo "🚀 РАЗВЕРТЫВАНИЕ ENDPOINTS ПОЛЬЗОВАТЕЛЕЙ"
    echo "================================================================"
    echo -e "${NC}"
}

print_header

print_info "Развертывание новых endpoints для загрузки пользователей из Axenta Cloud API"
print_info "Сервер: api.acrm.su"
print_info "Ветка: 2025-10-11-izac-45ad8"

# Проверяем подключение к серверу
print_info "Проверка подключения к продакшн серверу..."
if ! ping -c 1 api.acrm.su > /dev/null 2>&1; then
    print_error "Не удается подключиться к серверу api.acrm.su"
    exit 1
fi
print_success "Подключение к серверу установлено"

# Проверяем наличие SSH ключей
if [ ! -f ~/.ssh/id_rsa ] && [ ! -f ~/.ssh/id_ed25519 ]; then
    print_error "SSH ключи не найдены. Настройте SSH доступ к серверу."
    exit 1
fi

print_info "Подключение к серверу для развертывания..."

# Команды для выполнения на сервере
ssh_commands="
cd /opt/axenta/backend &&
git fetch origin &&
git checkout 2025-10-11-izac-45ad8 &&
git pull origin 2025-10-11-izac-45ad8 &&
go build -o axenta_backend main.go &&
sudo systemctl restart axenta-backend &&
sleep 3 &&
curl -s http://localhost:8080/ping
"

# Выполняем команды на сервере
if ssh root@api.acrm.su "$ssh_commands"; then
    print_success "Развертывание завершено успешно!"
    print_info "Новые endpoints доступны:"
    print_info "  • GET /api/auth/users - список пользователей из Axenta Cloud"
    print_info "  • GET /api/auth/users/stats - статистика пользователей"
    print_info "  • GET /api/auth/cms/users - альтернативный endpoint"
    
    print_warning "Не забудьте вернуть конфигурацию фронтенда на продакшн URL!"
    print_info "В src/config/env.ts измените:"
    print_info "  backendUrl: 'http://localhost:8080' → 'https://api.acrm.su'"
else
    print_error "Ошибка развертывания!"
    exit 1
fi
