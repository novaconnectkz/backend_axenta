#!/bin/bash

# 🔍 Скрипт диагностики проблем с базой данных на продакшен сервере
# Использование: ./diagnose_production_database.sh

set -e

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

echo -e "${BLUE}🔍 Диагностика проблем с базой данных на продакшен сервере${NC}"
echo -e "${BLUE}📡 Сервер: ${PRODUCTION_SERVER}${NC}"
echo -e "${BLUE}👤 Пользователь: ${PRODUCTION_USER}${NC}"
echo -e "${BLUE}📁 Путь: ${PRODUCTION_PATH}${NC}"
echo ""

# Функция для выполнения команд на удаленном сервере
run_remote() {
    echo -e "${BLUE}🔄 Выполнение на сервере: $1${NC}"
    ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "$1"
}

# Функция для получения вывода команд
get_remote_output() {
    ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "$1"
}

# Проверка доступности сервера
echo -e "${YELLOW}🌐 Проверка доступности сервера...${NC}"
if ssh -o ConnectTimeout=10 ${PRODUCTION_USER}@${PRODUCTION_SERVER} "echo 'Сервер доступен'" 2>/dev/null; then
    echo -e "${GREEN}✅ Сервер доступен${NC}"
else
    echo -e "${RED}❌ Сервер недоступен${NC}"
    exit 1
fi

echo ""

# 1. Проверка статуса сервиса
echo -e "${YELLOW}📋 1. Проверка статуса сервиса axenta-backend...${NC}"
SERVICE_STATUS=$(get_remote_output "systemctl is-active axenta-backend 2>/dev/null || echo 'inactive'")
echo -e "${BLUE}Статус сервиса: ${SERVICE_STATUS}${NC}"

if [ "$SERVICE_STATUS" = "active" ]; then
    echo -e "${GREEN}✅ Сервис запущен${NC}"
else
    echo -e "${RED}❌ Сервис не запущен${NC}"
fi

# 2. Проверка логов сервиса
echo ""
echo -e "${YELLOW}📋 2. Проверка последних логов сервиса...${NC}"
echo -e "${BLUE}Последние 20 строк логов:${NC}"
run_remote "journalctl -u axenta-backend -n 20 --no-pager"

# 3. Проверка конфигурации
echo ""
echo -e "${YELLOW}📋 3. Проверка конфигурации...${NC}"

# Проверка наличия .env файла
if get_remote_output "test -f ${PRODUCTION_PATH}/.env && echo 'exists' || echo 'missing'" | grep -q "exists"; then
    echo -e "${GREEN}✅ Файл .env существует${NC}"
    
    # Проверка основных переменных БД (без показа паролей)
    echo -e "${BLUE}Проверка переменных БД:${NC}"
    run_remote "cd ${PRODUCTION_PATH} && grep -E '^DB_(HOST|PORT|NAME|USER|SSLMODE)=' .env | head -5"
    
    # Проверка наличия пароля
    if get_remote_output "cd ${PRODUCTION_PATH} && grep -q '^DB_PASSWORD=' .env && echo 'has_password'" | grep -q "has_password"; then
        echo -e "${GREEN}✅ DB_PASSWORD настроен${NC}"
    else
        echo -e "${RED}❌ DB_PASSWORD не настроен${NC}"
    fi
    
else
    echo -e "${RED}❌ Файл .env не найден${NC}"
fi

# 4. Проверка подключения к PostgreSQL
echo ""
echo -e "${YELLOW}📋 4. Проверка подключения к PostgreSQL...${NC}"

# Проверка статуса PostgreSQL
POSTGRES_STATUS=$(get_remote_output "systemctl is-active postgresql 2>/dev/null || echo 'inactive'")
echo -e "${BLUE}Статус PostgreSQL: ${POSTGRES_STATUS}${NC}"

if [ "$POSTGRES_STATUS" = "active" ]; then
    echo -e "${GREEN}✅ PostgreSQL запущен${NC}"
    
    # Проверка доступности порта
    if get_remote_output "netstat -tlnp | grep :5432 | head -1"; then
        echo -e "${GREEN}✅ PostgreSQL слушает на порту 5432${NC}"
    else
        echo -e "${RED}❌ PostgreSQL не слушает на порту 5432${NC}"
    fi
    
else
    echo -e "${RED}❌ PostgreSQL не запущен${NC}"
fi

# 5. Проверка базы данных
echo ""
echo -e "${YELLOW}📋 5. Проверка базы данных...${NC}"

# Создание временного скрипта для проверки БД
run_remote "cat > /tmp/check_db.sql << 'EOF'
-- Проверка подключения к БД
SELECT 'Connected to database' as status;

-- Проверка существования базы данных
SELECT datname FROM pg_database WHERE datname = 'axenta_db';

-- Проверка схем
SELECT schema_name FROM information_schema.schemata WHERE schema_name IN ('public', 'tenant_test', 'tenant_demo') ORDER BY schema_name;

-- Проверка таблиц в схеме public
SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name;

-- Проверка таблиц в тенантных схемах
SELECT schemaname, tablename FROM pg_tables WHERE schemaname LIKE 'tenant_%' ORDER BY schemaname, tablename;
EOF"

# Выполнение проверки БД
echo -e "${BLUE}Проверка подключения к БД:${NC}"
if get_remote_output "cd ${PRODUCTION_PATH} && source .env 2>/dev/null && PGPASSWORD=\$DB_PASSWORD psql -h \$DB_HOST -p \$DB_PORT -U \$DB_USER -d \$DB_NAME -f /tmp/check_db.sql" 2>/dev/null; then
    echo -e "${GREEN}✅ Подключение к БД работает${NC}"
else
    echo -e "${RED}❌ Не удалось подключиться к БД${NC}"
    echo -e "${YELLOW}💡 Возможные причины:${NC}"
    echo -e "${YELLOW}   - Неправильные учетные данные${NC}"
    echo -e "${YELLOW}   - База данных не существует${NC}"
    echo -e "${YELLOW}   - Проблемы с правами доступа${NC}"
fi

# 6. Проверка структуры таблиц
echo ""
echo -e "${YELLOW}📋 6. Проверка структуры таблиц...${NC}"

# Создание скрипта для проверки таблиц
run_remote "cat > /tmp/check_tables.sql << 'EOF'
-- Проверка глобальных таблиц
SELECT 'Global Tables:' as section;
SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('companies', 'billing_plans', 'subscriptions', 'integrations', 'integration_errors', 'local_users', 'refresh_tokens') ORDER BY table_name;

-- Проверка тенантных таблиц в первой найденной тенантной схеме
SELECT 'Tenant Tables:' as section;
SELECT table_name FROM information_schema.tables WHERE table_schema = (SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tenant_%' LIMIT 1) AND table_name IN ('users', 'roles', 'permissions', 'user_templates', 'objects', 'contracts', 'equipment', 'installations') ORDER BY table_name;
EOF"

# Выполнение проверки таблиц
echo -e "${BLUE}Проверка структуры таблиц:${NC}"
if get_remote_output "cd ${PRODUCTION_PATH} && source .env 2>/dev/null && PGPASSWORD=\$DB_PASSWORD psql -h \$DB_HOST -p \$DB_PORT -U \$DB_USER -d \$DB_NAME -f /tmp/check_tables.sql" 2>/dev/null; then
    echo -e "${GREEN}✅ Проверка таблиц выполнена${NC}"
else
    echo -e "${RED}❌ Ошибка при проверке таблиц${NC}"
fi

# 7. Проверка исполняемого файла
echo ""
echo -e "${YELLOW}📋 7. Проверка исполняемого файла...${NC}"

if get_remote_output "test -f ${PRODUCTION_PATH}/main && echo 'exists' || echo 'missing'" | grep -q "exists"; then
    echo -e "${GREEN}✅ Исполняемый файл main существует${NC}"
    
    # Проверка прав доступа
    FILE_PERMS=$(get_remote_output "ls -la ${PRODUCTION_PATH}/main | awk '{print \$1}'")
    echo -e "${BLUE}Права доступа: ${FILE_PERMS}${NC}"
    
    # Проверка размера файла
    FILE_SIZE=$(get_remote_output "ls -lh ${PRODUCTION_PATH}/main | awk '{print \$5}'")
    echo -e "${BLUE}Размер файла: ${FILE_SIZE}${NC}"
    
else
    echo -e "${RED}❌ Исполняемый файл main не найден${NC}"
fi

# 8. Попытка запуска миграций вручную
echo ""
echo -e "${YELLOW}📋 8. Попытка запуска миграций вручную...${NC}"

# Создание скрипта миграций
run_remote "cat > ${PRODUCTION_PATH}/migrate_manual.go << 'EOF'
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
	log.Println(\"🚀 Ручной запуск миграций базы данных\")

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf(\"Failed to load configuration: %v\", err)
	}

	log.Printf(\"📊 Конфигурация БД: %s:%s/%s\", cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

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

	log.Println(\"🎉 Ручные миграции завершены!\")
}
EOF"

# Сборка и выполнение миграций
echo -e "${BLUE}Сборка скрипта миграций...${NC}"
if run_remote "cd ${PRODUCTION_PATH} && go build -o migrate_manual migrate_manual.go" 2>/dev/null; then
    echo -e "${GREEN}✅ Скрипт миграций собран${NC}"
    
    # Выполнение миграций
    echo -e "${BLUE}Выполнение миграций...${NC}"
    if run_remote "cd ${PRODUCTION_PATH} && ./migrate_manual" 2>/dev/null; then
        echo -e "${GREEN}✅ Миграции выполнены успешно${NC}"
    else
        echo -e "${RED}❌ Ошибка выполнения миграций${NC}"
        echo -e "${YELLOW}🔍 Детальные логи:${NC}"
        run_remote "cd ${PRODUCTION_PATH} && ./migrate_manual 2>&1 | tail -20"
    fi
    
    # Очистка файлов миграций
    run_remote "rm -f ${PRODUCTION_PATH}/migrate_manual.go ${PRODUCTION_PATH}/migrate_manual"
else
    echo -e "${RED}❌ Ошибка сборки скрипта миграций${NC}"
fi

# 9. Очистка временных файлов
echo ""
echo -e "${YELLOW}🧹 Очистка временных файлов...${NC}"
run_remote "rm -f /tmp/check_db.sql /tmp/check_tables.sql"

echo ""
echo -e "${GREEN}🎉 Диагностика завершена!${NC}"
echo ""
echo -e "${BLUE}📊 Резюме:${NC}"
echo -e "${BLUE}  - Статус сервиса: ${SERVICE_STATUS}${NC}"
echo -e "${BLUE}  - Статус PostgreSQL: ${POSTGRES_STATUS}${NC}"
echo ""
echo -e "${YELLOW}🔍 Для дополнительной диагностики:${NC}"
echo -e "${YELLOW}  - Проверьте логи: journalctl -u axenta-backend -f${NC}"
echo -e "${YELLOW}  - Проверьте логи PostgreSQL: journalctl -u postgresql -f${NC}"
echo -e "${YELLOW}  - Проверьте подключение к БД вручную${NC}"
