#!/bin/bash
# Скрипт для применения миграции колонок contracts на всех tenant-схемах
# Использование: ./apply_contracts_migration.sh

# Цвета для вывода
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}🔄 Применение миграции для таблицы contracts...${NC}"

# Получаем параметры подключения из переменных окружения или используем значения по умолчанию
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-axenta_crm}"
DB_USER="${DB_USER:-postgres}"

echo "📊 База данных: $DB_HOST:$DB_PORT/$DB_NAME"
echo "👤 Пользователь: $DB_USER"
echo ""

# Проверяем наличие psql
if ! command -v psql &> /dev/null; then
    echo -e "${RED}❌ Ошибка: psql не установлен${NC}"
    exit 1
fi

# Получаем список всех tenant-схем
echo -e "${YELLOW}🔍 Поиск tenant-схем...${NC}"
SCHEMAS=$(PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tenant_%' ORDER BY schema_name;")

if [ -z "$SCHEMAS" ]; then
    echo -e "${YELLOW}⚠️  Не найдено tenant-схем${NC}"
    exit 0
fi

# Считаем количество схем
SCHEMA_COUNT=$(echo "$SCHEMAS" | wc -l | tr -d ' ')
echo -e "${GREEN}✅ Найдено схем: $SCHEMA_COUNT${NC}"
echo ""

# Счетчики
SUCCESS_COUNT=0
ERROR_COUNT=0

# Применяем миграцию для каждой схемы
for schema in $SCHEMAS; do
    schema=$(echo "$schema" | xargs) # Удаляем пробелы
    
    echo -e "${YELLOW}🔄 Обработка схемы: $schema${NC}"
    
    # Проверяем существование таблицы contracts
    TABLE_EXISTS=$(PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = '$schema' AND table_name = 'contracts');")
    
    if [ "$TABLE_EXISTS" = " f" ]; then
        echo -e "  ${YELLOW}⏭️  Таблица contracts не существует, пропускаем${NC}"
        continue
    fi
    
    # Проверяем текущее состояние колонок
    CURRENT_STATE=$(PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT column_name, is_nullable FROM information_schema.columns WHERE table_schema = '$schema' AND table_name = 'contracts' AND column_name IN ('start_date', 'end_date') ORDER BY column_name;")
    
    echo "  📋 Текущее состояние колонок:"
    echo "$CURRENT_STATE" | while read line; do
        echo "    $line"
    done
    
    # Применяем миграцию
    MIGRATION_SQL="
        SET search_path TO $schema;
        ALTER TABLE contracts ALTER COLUMN start_date DROP NOT NULL;
        ALTER TABLE contracts ALTER COLUMN end_date DROP NOT NULL;
    "
    
    if PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "$MIGRATION_SQL" &> /dev/null; then
        echo -e "  ${GREEN}✅ Миграция применена успешно${NC}"
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    else
        echo -e "  ${RED}❌ Ошибка применения миграции${NC}"
        ERROR_COUNT=$((ERROR_COUNT + 1))
    fi
    
    echo ""
done

# Возвращаемся к схеме public
PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "SET search_path TO public;" &> /dev/null

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${GREEN}✅ Миграция завершена${NC}"
echo "📊 Обработано схем: $SCHEMA_COUNT"
echo -e "${GREEN}✅ Успешно: $SUCCESS_COUNT${NC}"
if [ $ERROR_COUNT -gt 0 ]; then
    echo -e "${RED}❌ Ошибок: $ERROR_COUNT${NC}"
fi
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

