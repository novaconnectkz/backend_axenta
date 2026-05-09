#!/bin/bash

# 🚀 Улучшенный скрипт автоматического деплоя backend на продакшен с миграциями
# Использование: ./deploy_production_with_migrations.sh

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

echo -e "${BLUE}🚀 Начинаем деплой Axenta Backend на продакшен с миграциями${NC}"
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

if [ ! -d "database" ]; then
    echo -e "${RED}❌ Папка database не найдена${NC}"
    exit 1
fi

if [ ! -f "database/migrations.go" ]; then
    echo -e "${RED}❌ Файл database/migrations.go не найден${NC}"
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
tar -czf deploy.tar.gz ${BINARY_NAME}_linux handlers/ database/ models/ config/ middleware/ services/ 2>/dev/null || \
tar -czf deploy.tar.gz ${BINARY_NAME}_linux handlers/ database/ models/ config/ middleware/ 2>/dev/null || \
tar -czf deploy.tar.gz ${BINARY_NAME}_linux handlers/ database/ models/ config/ 2>/dev/null || \
tar -czf deploy.tar.gz ${BINARY_NAME}_linux handlers/

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

# ВЫПОЛНЕНИЕ МИГРАЦИЙ БАЗЫ ДАННЫХ
echo -e "${YELLOW}🔄 Выполнение миграций базы данных...${NC}"

# Создание скрипта миграций на сервере
run_remote "cat > ${PRODUCTION_PATH}/migrate.go << 'EOF'
package main

import (
	\"fmt\"
	\"log\"
	\"os\"

	\"backend_axenta/config\"
	\"backend_axenta/database\"

	\"gorm.io/driver/postgres\"
	\"gorm.io/gorm\"
	\"gorm.io/gorm/logger\"
)

func main() {
	log.Println(\"🚀 Начинаем выполнение миграций базы данных\")

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf(\"Failed to load configuration: %v\", err)
	}

	// Подключаемся к базе данных
	dsn := cfg.GetDatabaseDSN()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf(\"Failed to connect to database: %v\", err)
	}

	log.Println(\"✅ Подключение к базе данных установлено\")

	// Устанавливаем глобальную переменную DB
	database.DB = db

	// Выполняем все миграции
	if err := database.RunAllMigrations(false); err != nil {
		log.Fatalf(\"❌ Ошибка выполнения миграций: %v\", err)
	}

	log.Println(\"✅ Все миграции выполнены успешно\")

	// Проверяем, что таблицы созданы
	checkTables := []string{\"roles\", \"user_templates\", \"permissions\", \"users\", \"companies\"}
	for _, table := range checkTables {
		if db.Migrator().HasTable(table) {
			log.Printf(\"✅ Таблица %s существует\", table)
		} else {
			log.Printf(\"❌ Таблица %s не найдена\", table)
		}
	}

	log.Println(\"🎉 Миграции завершены успешно!\")
}
EOF"

# Сборка и выполнение миграций
echo -e "${YELLOW}🔨 Сборка скрипта миграций...${NC}"
run_remote "cd ${PRODUCTION_PATH} && go build -o migrate migrate.go"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ Скрипт миграций собран${NC}"
    
    # Выполнение миграций
    echo -e "${YELLOW}🔄 Выполнение миграций...${NC}"
    run_remote "cd ${PRODUCTION_PATH} && ./migrate"
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✅ Миграции выполнены успешно${NC}"
    else
        echo -e "${RED}❌ Ошибка выполнения миграций${NC}"
        echo -e "${YELLOW}⚠️ Продолжаем деплой, но могут быть проблемы с БД${NC}"
    fi
    
    # Очистка файлов миграций
    run_remote "rm -f ${PRODUCTION_PATH}/migrate.go ${PRODUCTION_PATH}/migrate"
else
    echo -e "${RED}❌ Ошибка сборки скрипта миграций${NC}"
    echo -e "${YELLOW}⚠️ Продолжаем деплой без миграций${NC}"
fi

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

# Проверка roles endpoint
ROLES_STATUS=$(curl -s -o /dev/null -w "%{http_code}" https://${PRODUCTION_SERVER}/api/auth/roles || echo "000")
echo -e "${BLUE}👥 Roles API: ${ROLES_STATUS}${NC}"

# Проверка user-templates endpoint
TEMPLATES_STATUS=$(curl -s -o /dev/null -w "%{http_code}" https://${PRODUCTION_SERVER}/api/auth/user-templates || echo "000")
echo -e "${BLUE}📋 User Templates API: ${TEMPLATES_STATUS}${NC}"

if [ "$ACCOUNTS_STATUS" = "401" ]; then
    echo -e "${GREEN}✅ Accounts API работает (401 - требуется авторизация)${NC}"
elif [ "$ACCOUNTS_STATUS" = "200" ]; then
    echo -e "${GREEN}✅ Accounts API работает (200 - успешно)${NC}"
else
    echo -e "${RED}❌ Accounts API не работает (${ACCOUNTS_STATUS})${NC}"
fi

if [ "$ROLES_STATUS" != "000" ]; then
    echo -e "${GREEN}✅ Roles API отвечает (${ROLES_STATUS})${NC}"
else
    echo -e "${RED}❌ Roles API не отвечает${NC}"
fi

if [ "$TEMPLATES_STATUS" != "000" ]; then
    echo -e "${GREEN}✅ User Templates API отвечает (${TEMPLATES_STATUS})${NC}"
else
    echo -e "${RED}❌ User Templates API не отвечает${NC}"
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
echo -e "${BLUE}  - Roles API: ${ROLES_STATUS}${NC}"
echo -e "${BLUE}  - Templates API: ${TEMPLATES_STATUS}${NC}"
echo ""
echo -e "${YELLOW}🔍 Для проверки логов:${NC}"
echo -e "${YELLOW}  ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} 'journalctl -u ${SERVICE_NAME} -f'${NC}"
echo ""
echo -e "${YELLOW}🧪 Для полного тестирования:${NC}"
echo -e "${YELLOW}  cd frontend && node test-accounts-api.cjs${NC}"

# Определение успешности деплоя
SUCCESS=true
if [ "$SERVICE_STATUS" != "active" ]; then
    SUCCESS=false
fi
if [ "$ROLES_STATUS" = "000" ] || [ "$TEMPLATES_STATUS" = "000" ]; then
    SUCCESS=false
fi

if [ "$SUCCESS" = true ]; then
    echo -e "${GREEN}✅ Деплой успешен! API готов к работе.${NC}"
    exit 0
else
    echo -e "${RED}⚠️ Деплой завершен, но обнаружены проблемы.${NC}"
    echo -e "${YELLOW}🔍 Проверьте логи сервера для диагностики${NC}"
    exit 1
fi
