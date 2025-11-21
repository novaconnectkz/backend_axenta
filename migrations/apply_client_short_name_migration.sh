#!/bin/bash

# Цвета для вывода
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "🔧 Применение миграции client_short_name ко всем tenant-схемам..."
echo ""

# Получаем список tenant-схем
SCHEMAS=$(psql postgresql://postgres:postgres@localhost:5432/axenta_db -t -c "SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tenant_%' ORDER BY schema_name;")

if [ -z "$SCHEMAS" ]; then
    echo -e "${RED}❌ Не найдено tenant-схем${NC}"
    exit 1
fi

# Счетчики
SUCCESS_COUNT=0
FAIL_COUNT=0

# Применяем миграцию к каждой схеме
for schema in $SCHEMAS; do
    schema=$(echo $schema | xargs) # Убираем пробелы
    echo -e "${BLUE}📦 Схема: $schema${NC}"
    
    # Проверяем существование таблицы contracts
    TABLE_EXISTS=$(psql postgresql://postgres:postgres@localhost:5432/axenta_db -t -c "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = '$schema' AND table_name = 'contracts');")
    
    if [ "$TABLE_EXISTS" = " f" ]; then
        echo -e "  ${RED}⏭️  Таблица contracts не существует, пропускаем${NC}"
        echo ""
        continue
    fi
    
    # SQL для добавления колонки
    MIGRATION_SQL="
    SET search_path TO $schema;
    ALTER TABLE contracts 
    ADD COLUMN IF NOT EXISTS client_short_name VARCHAR(200) DEFAULT NULL;
    COMMENT ON COLUMN contracts.client_short_name IS 'Сокращенное название с ОПФ (для организаций)';
    "
    
    # Применяем миграцию
    if psql postgresql://postgres:postgres@localhost:5432/axenta_db -c "$MIGRATION_SQL" &> /dev/null; then
        echo -e "  ${GREEN}✅ Миграция применена успешно${NC}"
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    else
        echo -e "  ${RED}❌ Ошибка применения миграции${NC}"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
    echo ""
done

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${GREEN}✅ Миграция завершена${NC}"
echo -e "Успешно: ${GREEN}$SUCCESS_COUNT${NC}"
echo -e "Ошибки: ${RED}$FAIL_COUNT${NC}"

