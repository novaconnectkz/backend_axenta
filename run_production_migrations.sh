#!/bin/bash

# 🔄 Скрипт для выполнения миграций базы данных на продакшене
# Использование: ./run_production_migrations.sh

set -e  # Остановка при любой ошибке

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Конфигурация
PRODUCTION_SERVER="api.axenta.glonass-saratov.ru"
PRODUCTION_USER="root"
PRODUCTION_PATH="/opt/axenta-backend"
SERVICE_NAME="axenta-backend"

echo -e "${BLUE}🔄 Выполнение миграций базы данных на продакшене${NC}"
echo -e "${BLUE}📡 Сервер: ${PRODUCTION_SERVER}${NC}"
echo -e "${BLUE}👤 Пользователь: ${PRODUCTION_USER}${NC}"
echo -e "${BLUE}📁 Путь: ${PRODUCTION_PATH}${NC}"
echo ""

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

# Создание скрипта миграций на сервере
echo -e "${YELLOW}📝 Создание скрипта миграций на сервере...${NC}"
run_remote "cat > ${PRODUCTION_PATH}/migrate.go << 'EOF'
package main

import (
	\"fmt\"
	\"log\"
	\"os\"

	\"backend_axenta/config\"
	\"backend_axenta/database\"
	\"backend_axenta/models\"

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
	checkTables := []string{\"roles\", \"user_templates\", \"permissions\", \"users\"}
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
echo -e "${YELLOW}🔨 Сборка и выполнение миграций...${NC}"
run_remote "cd ${PRODUCTION_PATH} && go build -o migrate migrate.go"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ Скрипт миграций собран${NC}"
else
    echo -e "${RED}❌ Ошибка сборки скрипта миграций${NC}"
    exit 1
fi

# Выполнение миграций
echo -e "${YELLOW}🔄 Выполнение миграций...${NC}"
run_remote "cd ${PRODUCTION_PATH} && ./migrate"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ Миграции выполнены успешно${NC}"
else
    echo -e "${RED}❌ Ошибка выполнения миграций${NC}"
    exit 1
fi

# Перезапуск сервиса для применения изменений
echo -e "${YELLOW}🔄 Перезапуск сервиса...${NC}"
run_remote "systemctl restart ${SERVICE_NAME}"

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

# Тестирование API после миграций
echo -e "${YELLOW}🧪 Тестирование API после миграций...${NC}"
sleep 5

# Проверка roles endpoint
ROLES_STATUS=$(curl -s -o /dev/null -w "%{http_code}" https://${PRODUCTION_SERVER}/api/auth/roles || echo "000")
echo -e "${BLUE}👥 Roles API: ${ROLES_STATUS}${NC}"

# Проверка user-templates endpoint
TEMPLATES_STATUS=$(curl -s -o /dev/null -w "%{http_code}" https://${PRODUCTION_SERVER}/api/auth/user-templates || echo "000")
echo -e "${BLUE}📋 User Templates API: ${TEMPLATES_STATUS}${NC}"

# Очистка временных файлов
echo -e "${YELLOW}🧹 Очистка временных файлов...${NC}"
run_remote "rm -f ${PRODUCTION_PATH}/migrate.go ${PRODUCTION_PATH}/migrate"

echo ""
echo -e "${GREEN}🎉 Миграции завершены!${NC}"
echo ""
echo -e "${BLUE}📊 Результаты:${NC}"
echo -e "${BLUE}  - Сервис: ${SERVICE_STATUS}${NC}"
echo -e "${BLUE}  - Roles API: ${ROLES_STATUS}${NC}"
echo -e "${BLUE}  - Templates API: ${TEMPLATES_STATUS}${NC}"
echo ""

if [ "$ROLES_STATUS" != "000" ] && [ "$TEMPLATES_STATUS" != "000" ]; then
    echo -e "${GREEN}✅ Миграции успешны! API эндпоинты отвечают.${NC}"
    exit 0
else
    echo -e "${RED}⚠️ Миграции завершены, но API может работать некорректно.${NC}"
    echo -e "${YELLOW}🔍 Проверьте логи сервера для диагностики${NC}"
    exit 1
fi
