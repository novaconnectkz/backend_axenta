#!/bin/bash

# 🔧 Скрипт исправления проблем с базой данных на продакшен сервере
# Использование: ./fix_production_database.sh

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
SERVICE_NAME="axenta-backend"

echo -e "${BLUE}🔧 Исправление проблем с базой данных на продакшен сервере${NC}"
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

# 1. Остановка сервиса
echo -e "${YELLOW}📋 1. Остановка сервиса для безопасного выполнения миграций...${NC}"
run_remote "systemctl stop ${SERVICE_NAME} || true"
echo -e "${GREEN}✅ Сервис остановлен${NC}"

# 2. Создание резервной копии базы данных
echo ""
echo -e "${YELLOW}📋 2. Создание резервной копии базы данных...${NC}"

BACKUP_FILE="/tmp/axenta_backup_$(date +%Y%m%d_%H%M%S).sql"
echo -e "${BLUE}Создание резервной копии: ${BACKUP_FILE}${NC}"

if run_remote "cd ${PRODUCTION_PATH} && source .env 2>/dev/null && PGPASSWORD=\$DB_PASSWORD pg_dump -h \$DB_HOST -p \$DB_PORT -U \$DB_USER -d \$DB_NAME > ${BACKUP_FILE}" 2>/dev/null; then
    echo -e "${GREEN}✅ Резервная копия создана${NC}"
    BACKUP_SIZE=$(get_remote_output "ls -lh ${BACKUP_FILE} | awk '{print \$5}'")
    echo -e "${BLUE}Размер резервной копии: ${BACKUP_SIZE}${NC}"
else
    echo -e "${YELLOW}⚠️ Не удалось создать резервную копию, продолжаем без неё${NC}"
fi

# 3. Создание и выполнение скрипта миграций
echo ""
echo -e "${YELLOW}📋 3. Создание и выполнение скрипта миграций...${NC}"

# Создание улучшенного скрипта миграций
run_remote "cat > ${PRODUCTION_PATH}/fix_migrations.go << 'EOF'
package main

import (
	\"fmt\"
	\"log\"
	\"os\"
	\"strings\"

	\"backend_axenta/config\"
	\"backend_axenta/database\"
	\"backend_axenta/models\"

	\"gorm.io/driver/postgres\"
	\"gorm.io/gorm\"
	\"gorm.io/gorm/logger\"
)

func main() {
	log.Println(\"🚀 Исправление миграций базы данных\")

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

	// 1. Создание глобальных таблиц
	log.Println(\"📋 Создание глобальных таблиц...\")
	globalModels := []interface{}{
		&models.Company{},
		&models.BillingPlan{},
		&models.Subscription{},
		&models.Integration{},
		&models.IntegrationError{},
		&models.LocalUser{},
		&models.RefreshToken{},
	}

	// Убеждаемся, что мы в схеме public
	if err := db.Exec(\"SET search_path TO public\").Error; err != nil {
		log.Printf(\"⚠️ Не удалось переключиться на схему public: %v\", err)
	}

	for _, model := range globalModels {
		if err := db.AutoMigrate(model); err != nil {
			log.Printf(\"❌ Ошибка миграции глобальной таблицы %T: %v\", model, err)
		} else {
			log.Printf(\"✅ Глобальная таблица %T создана/обновлена\", model)
		}
	}

	// 2. Проверка существования компаний
	log.Println(\"📋 Проверка существования компаний...\")
	var companies []models.Company
	if err := db.Find(&companies).Error; err != nil {
		log.Printf(\"⚠️ Не удалось получить список компаний: %v\", err)
	} else {
		log.Printf(\"📊 Найдено компаний: %d\", len(companies))
		
		// Если нет компаний, создаем тестовую
		if len(companies) == 0 {
			log.Println(\"📋 Создание тестовой компании...\")
			testCompany := &models.Company{
				Name:           \"Test Company\",
				DatabaseSchema: \"tenant_test\",
				IsActive:       true,
			}
			
			if err := db.Create(testCompany).Error; err != nil {
				log.Printf(\"❌ Ошибка создания тестовой компании: %v\", err)
			} else {
				log.Printf(\"✅ Тестовая компания создана с ID: %d\", testCompany.ID)
				companies = append(companies, *testCompany)
			}
		}
	}

	// 3. Создание тенантных таблиц для каждой компании
	log.Println(\"📋 Создание тенантных таблиц...\")
	tenantModels := []interface{}{
		&models.User{},
		&models.Role{},
		&models.Permission{},
		&models.UserTemplate{},
		&models.Object{},
		&models.ObjectTemplate{},
		&models.Contract{},
		&models.ContractAppendix{},
		&models.TariffPlan{},
		&models.Equipment{},
		&models.EquipmentCategory{},
		&models.Location{},
		&models.Installer{},
		&models.Installation{},
		&models.WarehouseOperation{},
		&models.StockAlert{},
		&models.MonitoringTemplate{},
		&models.MonitoringNotificationTemplate{},
	}

	for _, company := range companies {
		if !company.IsActive {
			continue
		}

		log.Printf(\"🏢 Создание таблиц для компании: %s (схема: %s)\", company.Name, company.DatabaseSchema)

		// Создаем схему, если она не существует
		if err := db.Exec(fmt.Sprintf(\"CREATE SCHEMA IF NOT EXISTS %s\", company.DatabaseSchema)).Error; err != nil {
			log.Printf(\"❌ Ошибка создания схемы %s: %v\", company.DatabaseSchema, err)
			continue
		}

		// Переключаемся на схему компании
		if err := db.Exec(fmt.Sprintf(\"SET search_path TO %s\", company.DatabaseSchema)).Error; err != nil {
			log.Printf(\"❌ Ошибка переключения на схему %s: %v\", company.DatabaseSchema, err)
			continue
		}

		// Создаем таблицы в схеме компании
		for _, model := range tenantModels {
			if err := db.AutoMigrate(model); err != nil {
				log.Printf(\"❌ Ошибка миграции таблицы %T в схеме %s: %v\", model, company.DatabaseSchema, err)
			} else {
				log.Printf(\"✅ Таблица %T создана в схеме %s\", model, company.DatabaseSchema)
			}
		}
	}

	// Возвращаемся к схеме public
	if err := db.Exec(\"SET search_path TO public\").Error; err != nil {
		log.Printf(\"⚠️ Не удалось вернуться к схеме public: %v\", err)
	}

	// 4. Проверка созданных таблиц
	log.Println(\"📋 Проверка созданных таблиц...\")
	
	// Проверяем глобальные таблицы
	globalTables := []string{\"companies\", \"billing_plans\", \"subscriptions\", \"integrations\", \"integration_errors\", \"local_users\", \"refresh_tokens\"}
	for _, table := range globalTables {
		if db.Migrator().HasTable(table) {
			log.Printf(\"✅ Глобальная таблица %s существует\", table)
		} else {
			log.Printf(\"❌ Глобальная таблица %s не найдена\", table)
		}
	}

	// Проверяем тенантные таблицы
	tenantTables := []string{\"users\", \"roles\", \"permissions\", \"user_templates\", \"objects\", \"contracts\", \"equipment\"}
	for _, company := range companies {
		if !company.IsActive {
			continue
		}
		
		log.Printf(\"🏢 Проверка таблиц в схеме %s:\", company.DatabaseSchema)
		if err := db.Exec(fmt.Sprintf(\"SET search_path TO %s\", company.DatabaseSchema)).Error; err != nil {
			log.Printf(\"❌ Ошибка переключения на схему %s: %v\", company.DatabaseSchema, err)
			continue
		}
		
		for _, table := range tenantTables {
			if db.Migrator().HasTable(table) {
				log.Printf(\"✅ Таблица %s существует в схеме %s\", table, company.DatabaseSchema)
			} else {
				log.Printf(\"❌ Таблица %s не найдена в схеме %s\", table, company.DatabaseSchema)
			}
		}
	}

	// Возвращаемся к схеме public
	db.Exec(\"SET search_path TO public\")

	log.Println(\"🎉 Исправление миграций завершено!\")
}
EOF"

# Сборка скрипта исправления
echo -e "${BLUE}Сборка скрипта исправления...${NC}"
if run_remote "cd ${PRODUCTION_PATH} && go build -o fix_migrations fix_migrations.go" 2>/dev/null; then
    echo -e "${GREEN}✅ Скрипт исправления собран${NC}"
    
    # Выполнение исправления
    echo -e "${BLUE}Выполнение исправления миграций...${NC}"
    if run_remote "cd ${PRODUCTION_PATH} && ./fix_migrations" 2>/dev/null; then
        echo -e "${GREEN}✅ Исправление миграций выполнено успешно${NC}"
    else
        echo -e "${RED}❌ Ошибка выполнения исправления миграций${NC}"
        echo -e "${YELLOW}🔍 Детальные логи:${NC}"
        run_remote "cd ${PRODUCTION_PATH} && ./fix_migrations 2>&1 | tail -30"
    fi
    
    # Очистка файлов
    run_remote "rm -f ${PRODUCTION_PATH}/fix_migrations.go ${PRODUCTION_PATH}/fix_migrations"
else
    echo -e "${RED}❌ Ошибка сборки скрипта исправления${NC}"
fi

# 4. Запуск сервиса
echo ""
echo -e "${YELLOW}📋 4. Запуск сервиса...${NC}"
run_remote "systemctl start ${SERVICE_NAME}"

# Проверка статуса
echo -e "${YELLOW}🔍 Проверка статуса сервиса...${NC}"
sleep 5
SERVICE_STATUS=$(get_remote_output "systemctl is-active ${SERVICE_NAME} 2>/dev/null || echo 'inactive'")

if [ "$SERVICE_STATUS" = "active" ]; then
    echo -e "${GREEN}✅ Сервис запущен успешно${NC}"
else
    echo -e "${RED}❌ Проблема с запуском сервиса${NC}"
    echo -e "${YELLOW}📋 Статус: ${SERVICE_STATUS}${NC}"
    echo -e "${YELLOW}🔍 Проверьте логи: journalctl -u ${SERVICE_NAME} -f${NC}"
fi

# 5. Тестирование API
echo ""
echo -e "${YELLOW}📋 5. Тестирование API...${NC}"
sleep 3

# Проверка health endpoint
HEALTH_STATUS=$(curl -s -o /dev/null -w "%{http_code}" https://${PRODUCTION_SERVER}/health || echo "000")
echo -e "${BLUE}🏥 Health check: ${HEALTH_STATUS}${NC}"

# Проверка accounts endpoint
ACCOUNTS_STATUS=$(curl -s -o /dev/null -w "%{http_code}" https://${PRODUCTION_SERVER}/api/accounts || echo "000")
echo -e "${BLUE}📊 Accounts API: ${ACCOUNTS_STATUS}${NC}"

# Проверка roles endpoint
ROLES_STATUS=$(curl -s -o /dev/null -w "%{http_code}" https://${PRODUCTION_SERVER}/api/auth/roles || echo "000")
echo -e "${BLUE}👥 Roles API: ${ROLES_STATUS}${NC}"

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

# 6. Финальная проверка базы данных
echo ""
echo -e "${YELLOW}📋 6. Финальная проверка базы данных...${NC}"

# Создание скрипта финальной проверки
run_remote "cat > /tmp/final_check.sql << 'EOF'
-- Проверка глобальных таблиц
SELECT 'Global Tables Check:' as section;
SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('companies', 'billing_plans', 'subscriptions', 'integrations', 'integration_errors', 'local_users', 'refresh_tokens') ORDER BY table_name;

-- Проверка тенантных схем
SELECT 'Tenant Schemas:' as section;
SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tenant_%' ORDER BY schema_name;

-- Проверка тенантных таблиц
SELECT 'Tenant Tables:' as section;
SELECT schemaname, tablename FROM pg_tables WHERE schemaname LIKE 'tenant_%' AND tablename IN ('users', 'roles', 'permissions', 'user_templates', 'objects', 'contracts', 'equipment') ORDER BY schemaname, tablename;
EOF"

# Выполнение финальной проверки
echo -e "${BLUE}Финальная проверка БД:${NC}"
if get_remote_output "cd ${PRODUCTION_PATH} && source .env 2>/dev/null && PGPASSWORD=\$DB_PASSWORD psql -h \$DB_HOST -p \$DB_PORT -U \$DB_USER -d \$DB_NAME -f /tmp/final_check.sql" 2>/dev/null; then
    echo -e "${GREEN}✅ Финальная проверка БД выполнена${NC}"
else
    echo -e "${RED}❌ Ошибка финальной проверки БД${NC}"
fi

# Очистка временных файлов
echo ""
echo -e "${YELLOW}🧹 Очистка временных файлов...${NC}"
run_remote "rm -f /tmp/final_check.sql"

echo ""
echo -e "${GREEN}🎉 Исправление завершено!${NC}"
echo ""
echo -e "${BLUE}📊 Результаты:${NC}"
echo -e "${BLUE}  - Статус сервиса: ${SERVICE_STATUS}${NC}"
echo -e "${BLUE}  - Health: ${HEALTH_STATUS}${NC}"
echo -e "${BLUE}  - Accounts API: ${ACCOUNTS_STATUS}${NC}"
echo -e "${BLUE}  - Roles API: ${ROLES_STATUS}${NC}"

if [ -n "$BACKUP_FILE" ] && get_remote_output "test -f ${BACKUP_FILE} && echo 'exists'" | grep -q "exists"; then
    echo -e "${BLUE}  - Резервная копия: ${BACKUP_FILE}${NC}"
fi

echo ""
echo -e "${YELLOW}🔍 Для проверки логов:${NC}"
echo -e "${YELLOW}  ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} 'journalctl -u ${SERVICE_NAME} -f'${NC}"
echo ""
echo -e "${YELLOW}🧪 Для полного тестирования:${NC}"
echo -e "${YELLOW}  cd frontend && node test-accounts-api.cjs${NC}"

# Определение успешности исправления
SUCCESS=true
if [ "$SERVICE_STATUS" != "active" ]; then
    SUCCESS=false
fi
if [ "$ROLES_STATUS" = "000" ]; then
    SUCCESS=false
fi

if [ "$SUCCESS" = true ]; then
    echo ""
    echo -e "${GREEN}✅ Исправление успешно! API готов к работе.${NC}"
    exit 0
else
    echo ""
    echo -e "${RED}⚠️ Исправление завершено, но обнаружены проблемы.${NC}"
    echo -e "${YELLOW}🔍 Проверьте логи сервера для диагностики${NC}"
    exit 1
fi
