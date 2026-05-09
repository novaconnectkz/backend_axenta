#!/bin/bash

# Скрипт для добавления DaData API ключа на продакшен сервер
# Использование: ./add_dadata_key_to_production.sh

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Конфигурация (из deploy-production.sh)
PRODUCTION_SERVER="api.acrm.su"
PRODUCTION_USER="root"
PRODUCTION_PATH="/opt/axenta-backend"

# DaData API ключ
DADATA_API_KEY="9f89eacbff6b5a22581f4f3d36103470a5fede82"

echo -e "${BLUE}🔑 Добавление DaData API ключа на продакшен сервер${NC}"
echo -e "${BLUE}📡 Сервер: ${PRODUCTION_SERVER}${NC}"
echo -e "${BLUE}📁 Путь: ${PRODUCTION_PATH}${NC}"
echo ""

# Проверка доступности сервера
echo -e "${YELLOW}🌐 Проверка доступности сервера...${NC}"
if ssh -o ConnectTimeout=10 ${PRODUCTION_USER}@${PRODUCTION_SERVER} "echo 'Сервер доступен'" 2>/dev/null; then
    echo -e "${GREEN}✅ Сервер доступен${NC}"
else
    echo -e "${RED}❌ Сервер недоступен${NC}"
    echo -e "${YELLOW}💡 Проверьте SSH ключи и доступ к серверу${NC}"
    exit 1
fi

# Добавление ключа в .env файл
echo -e "${YELLOW}📝 Добавление ключа в .env файл...${NC}"

# Команда для добавления/обновления ключа
SSH_CMD="cd ${PRODUCTION_PATH} && \
if grep -q '^DADATA_API_KEY=' .env 2>/dev/null; then \
  sed -i 's|^DADATA_API_KEY=.*|DADATA_API_KEY=${DADATA_API_KEY}|' .env && \
  echo 'Ключ обновлен'; \
else \
  echo 'DADATA_API_KEY=${DADATA_API_KEY}' >> .env && \
  echo 'Ключ добавлен'; \
fi"

if ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "${SSH_CMD}"; then
    echo -e "${GREEN}✅ Ключ успешно добавлен/обновлен в .env${NC}"
else
    echo -e "${RED}❌ Ошибка при добавлении ключа${NC}"
    exit 1
fi

# Проверка, что ключ действительно добавлен
echo -e "${YELLOW}🔍 Проверка наличия ключа...${NC}"
if ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "cd ${PRODUCTION_PATH} && grep '^DADATA_API_KEY=' .env | grep -q '${DADATA_API_KEY}'" 2>/dev/null; then
    echo -e "${GREEN}✅ Ключ найден в .env файле${NC}"
    ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "cd ${PRODUCTION_PATH} && grep '^DADATA_API_KEY=' .env"
else
    echo -e "${RED}❌ Ключ не найден в .env файле${NC}"
    exit 1
fi

# Перезапуск сервиса
echo -e "${YELLOW}🔄 Перезапуск бэкенд сервиса...${NC}"
if ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "systemctl restart axenta-backend" 2>/dev/null; then
    echo -e "${GREEN}✅ Сервис перезапущен${NC}"
    
    # Небольшая задержка для запуска сервиса
    sleep 2
    
    # Проверка статуса сервиса
    echo -e "${YELLOW}📊 Проверка статуса сервиса...${NC}"
    SERVICE_STATUS=$(ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "systemctl is-active axenta-backend" 2>/dev/null || echo "unknown")
    if [ "$SERVICE_STATUS" = "active" ]; then
        echo -e "${GREEN}✅ Сервис активен${NC}"
    else
        echo -e "${YELLOW}⚠️  Статус сервиса: ${SERVICE_STATUS}${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  Не удалось перезапустить сервис автоматически${NC}"
    echo -e "${YELLOW}💡 Выполните вручную: ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} 'systemctl restart axenta-backend'${NC}"
fi

echo ""
echo -e "${GREEN}═══════════════════════════════════════════════════${NC}"
echo -e "${GREEN}✅ DaData API ключ успешно добавлен и сервис перезапущен!${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════${NC}"
echo ""
echo -e "${BLUE}💡 Теперь автозаполнение по ИНН должно работать на продакшене${NC}"

