#!/bin/bash

# 🚀 Скрипт автоматического деплоя backend на продакшен
# Использование: ./deploy-production.sh

set -e  # Остановка при любой ошибке

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Конфигурация
PRODUCTION_SERVER="api.acrm.su"
PRODUCTION_USER="root"  # Замените на реального пользователя
PRODUCTION_PATH="/opt/axenta-backend"  # Замените на реальный путь
SERVICE_NAME="axenta-backend"
BINARY_NAME="main"

echo -e "${BLUE}🚀 Начинаем деплой Axenta Backend на продакшен${NC}"
echo -e "${BLUE}📡 Сервер: ${PRODUCTION_SERVER}${NC}"
echo -e "${BLUE}👤 Пользователь: ${PRODUCTION_USER}${NC}"
echo -e "${BLUE}📁 Путь: ${PRODUCTION_PATH}${NC}"
echo ""

# Проверка наличия необходимых файлов
echo -e "${YELLOW}📋 Проверка файлов...${NC}"
if [ ! -f "main.go" ]; then
    echo -e "${RED}❌ Файл main.go не найден${NC}"
    exit 1
fi

if [ ! -d "handlers" ]; then
    echo -e "${RED}❌ Папка handlers не найдена${NC}"
    exit 1
fi

if [ ! -f "handlers/accounts.go" ]; then
    echo -e "${RED}❌ Файл handlers/accounts.go не найден${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Все необходимые файлы найдены${NC}"

# Сборка для продакшена
echo -e "${YELLOW}🔨 Сборка для продакшена...${NC}"
echo "Архитектура: linux/amd64"
GOOS=linux GOARCH=amd64 go build -o ${BINARY_NAME}_linux .

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ Сборка успешна${NC}"
else
    echo -e "${RED}❌ Ошибка сборки${NC}"
    exit 1
fi

# Проверка размера файла
FILE_SIZE=$(ls -lh ${BINARY_NAME}_linux | awk '{print $5}')
echo -e "${BLUE}📊 Размер исполняемого файла: ${FILE_SIZE}${NC}"

# Создание архива с необходимыми файлами
echo -e "${YELLOW}📦 Создание архива для деплоя...${NC}"
tar -czf deploy.tar.gz ${BINARY_NAME}_linux handlers/ config/ 2>/dev/null || tar -czf deploy.tar.gz ${BINARY_NAME}_linux handlers/

echo -e "${GREEN}✅ Архив создан: deploy.tar.gz${NC}"

# Функция для выполнения команд на удаленном сервере
run_remote() {
    echo -e "${BLUE}🔄 Выполнение на сервере: $1${NC}"
    ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "$1"
}

# Проверка доступности сервера
echo -e "${YELLOW}🌐 Проверка доступности сервера...${NC}"
if ssh -o ConnectTimeout=10 ${PRODUCTION_USER}@${PRODUCTION_SERVER} "echo 'Сервер доступен'" 2>/dev/null; then
    echo -e "${GREEN}✅ Сервер доступен${NC}"
else
    echo -e "${RED}❌ Сервер недоступен${NC}"
    echo -e "${YELLOW}💡 Проверьте SSH ключи и доступ к серверу${NC}"
    exit 1
fi

# Копирование файлов на сервер
echo -e "${YELLOW}📤 Копирование файлов на сервер...${NC}"
scp deploy.tar.gz ${PRODUCTION_USER}@${PRODUCTION_SERVER}:/tmp/

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ Файлы скопированы${NC}"
else
    echo -e "${RED}❌ Ошибка копирования файлов${NC}"
    exit 1
fi

# Деплой на сервере
echo -e "${YELLOW}🚀 Выполнение деплоя на сервере...${NC}"

# Создание папки и распаковка
run_remote "mkdir -p ${PRODUCTION_PATH}/backup"
run_remote "cd ${PRODUCTION_PATH} && tar -xzf /tmp/deploy.tar.gz"

# Остановка сервиса
echo -e "${YELLOW}⏹️ Остановка сервиса...${NC}"
run_remote "systemctl stop ${SERVICE_NAME} || true"

# Создание бэкапа текущей версии
echo -e "${YELLOW}💾 Создание бэкапа...${NC}"
run_remote "cp ${PRODUCTION_PATH}/${BINARY_NAME} ${PRODUCTION_PATH}/backup/${BINARY_NAME}_$(date +%Y%m%d_%H%M%S) 2>/dev/null || true"

# Замена исполняемого файла
echo -e "${YELLOW}🔄 Замена исполняемого файла...${NC}"
run_remote "mv ${PRODUCTION_PATH}/${BINARY_NAME}_linux ${PRODUCTION_PATH}/${BINARY_NAME}"
run_remote "chmod +x ${PRODUCTION_PATH}/${BINARY_NAME}"

# Запуск сервиса
echo -e "${YELLOW}▶️ Запуск сервиса...${NC}"
run_remote "systemctl start ${SERVICE_NAME}"

# Проверка статуса
echo -e "${YELLOW}🔍 Проверка статуса сервиса...${NC}"
sleep 3
SERVICE_STATUS=$(ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "systemctl is-active ${SERVICE_NAME}" 2>/dev/null || echo "unknown")

if [ "$SERVICE_STATUS" = "active" ]; then
    echo -e "${GREEN}✅ Сервис запущен успешно${NC}"
else
    echo -e "${RED}❌ Проблема с запуском сервиса${NC}"
    echo -e "${YELLOW}📋 Статус: ${SERVICE_STATUS}${NC}"
    echo -e "${YELLOW}🔍 Проверьте логи: journalctl -u ${SERVICE_NAME} -f${NC}"
fi

# Тестирование API
echo -e "${YELLOW}🧪 Тестирование API...${NC}"
sleep 5

# Проверка health endpoint
HEALTH_STATUS=$(curl -s -o /dev/null -w "%{http_code}" https://${PRODUCTION_SERVER}/health || echo "000")
echo -e "${BLUE}🏥 Health check: ${HEALTH_STATUS}${NC}"

# Проверка accounts endpoint
ACCOUNTS_STATUS=$(curl -s -o /dev/null -w "%{http_code}" https://${PRODUCTION_SERVER}/api/accounts || echo "000")
echo -e "${BLUE}📊 Accounts API: ${ACCOUNTS_STATUS}${NC}"

if [ "$ACCOUNTS_STATUS" = "401" ]; then
    echo -e "${GREEN}✅ API эндпоинт работает (401 - требуется авторизация)${NC}"
elif [ "$ACCOUNTS_STATUS" = "200" ]; then
    echo -e "${GREEN}✅ API эндпоинт работает (200 - успешно)${NC}"
else
    echo -e "${RED}❌ API эндпоинт не работает (${ACCOUNTS_STATUS})${NC}"
fi

# Очистка временных файлов
echo -e "${YELLOW}🧹 Очистка временных файлов...${NC}"
rm -f ${BINARY_NAME}_linux deploy.tar.gz
run_remote "rm -f /tmp/deploy.tar.gz"

echo ""
echo -e "${GREEN}🎉 Деплой завершен!${NC}"
echo ""
echo -e "${BLUE}📊 Результаты:${NC}"
echo -e "${BLUE}  - Сервис: ${SERVICE_STATUS}${NC}"
echo -e "${BLUE}  - Health: ${HEALTH_STATUS}${NC}"
echo -e "${BLUE}  - Accounts API: ${ACCOUNTS_STATUS}${NC}"
echo ""
echo -e "${YELLOW}🔍 Для проверки логов:${NC}"
echo -e "${YELLOW}  ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} 'journalctl -u ${SERVICE_NAME} -f'${NC}"
echo ""
echo -e "${YELLOW}🧪 Для полного тестирования:${NC}"
echo -e "${YELLOW}  cd frontend && node test-accounts-api.cjs${NC}"

if [ "$ACCOUNTS_STATUS" = "401" ] || [ "$ACCOUNTS_STATUS" = "200" ]; then
    echo -e "${GREEN}✅ Деплой успешен! API готов к работе.${NC}"
    exit 0
else
    echo -e "${RED}⚠️ Деплой завершен, но API может работать некорректно.${NC}"
    exit 1
fi
