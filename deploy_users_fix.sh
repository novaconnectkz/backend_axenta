#!/bin/bash

# 🚀 Скрипт исправления 404 ошибок для endpoints пользователей

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

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_header() {
    echo -e "${BLUE}"
    echo "================================================================"
    echo "🚀 ИСПРАВЛЕНИЕ 404 ОШИБОК ПОЛЬЗОВАТЕЛЕЙ API"
    echo "================================================================"
    echo -e "${NC}"
}

print_header

print_info "Развертывание исправлений для endpoints пользователей"
print_info "Сервер: api.acrm.su"

# Команды для выполнения на сервере
ssh_commands="
echo '🔄 Переходим в рабочую директорию...' &&
cd /var/www/backend_axenta &&
echo '📥 Обновляем код из GitHub...' &&
git fetch --all --prune &&
git pull origin main &&
echo '🔨 Собираем новую версию...' &&
/usr/local/go/bin/go build -ldflags='-w -s' -o axenta_backend main.go &&
echo '🔄 Перезапускаем сервис...' &&
sudo systemctl restart axenta-backend &&
echo '⏳ Ждем запуска сервиса...' &&
sleep 5 &&
echo '✅ Проверяем статус сервиса...' &&
sudo systemctl status axenta-backend --no-pager -l &&
echo '🌐 Проверяем доступность endpoints...' &&
curl -s -w 'HTTP Status: %{http_code}\\n' http://localhost:8080/api/auth/roles || echo 'Endpoint недоступен'
"

print_info "Подключение к серверу для развертывания..."

# Выполняем команды на сервере
if ssh -o StrictHostKeyChecking=no root@api.acrm.su "$ssh_commands"; then
    print_success "Развертывание завершено!"
    
    print_info "Проверяем endpoints с внешнего доступа..."
    sleep 2
    
    # Проверяем endpoints
    echo ""
    print_info "Проверка /api/auth/roles:"
    curl -s -w "HTTP Status: %{http_code}\n" https://api.acrm.su/api/auth/roles || echo "Ошибка проверки"
    
    print_info "Проверка /api/auth/users:"
    curl -s -w "HTTP Status: %{http_code}\n" https://api.acrm.su/api/auth/users || echo "Ошибка проверки"
    
    print_info "Проверка /api/auth/user-templates:"
    curl -s -w "HTTP Status: %{http_code}\n" https://api.acrm.su/api/auth/user-templates || echo "Ошибка проверки"
    
    print_info "Проверка /api/auth/users/stats:"
    curl -s -w "HTTP Status: %{http_code}\n" https://api.acrm.su/api/auth/users/stats || echo "Ошибка проверки"
    
    echo ""
    print_success "Исправления развернуты! Теперь endpoints должны возвращать 401 (Unauthorized) вместо 404"
    
else
    print_error "Ошибка развертывания!"
    exit 1
fi
