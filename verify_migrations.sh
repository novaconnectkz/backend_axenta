#!/bin/bash

# 🔍 Скрипт для проверки миграций базы данных Axenta CRM
# Использование: ./verify_migrations.sh [local|production|both] [--save-report]

set -e  # Остановка при любой ошибке

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Конфигурация
PRODUCTION_SERVER="api.acrm.su"
PRODUCTION_USER="root"
PRODUCTION_PATH="/opt/axenta-backend"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")

echo -e "${BLUE}🔍 Проверка миграций базы данных Axenta CRM${NC}"
echo -e "${BLUE}===============================================${NC}"
echo ""

# Функция для показа справки
show_help() {
    echo "Использование: $0 [режим] [опции]"
    echo ""
    echo "Режимы:"
    echo "  local       Проверить только локальную базу данных"
    echo "  production  Проверить только продакшен базу данных"
    echo "  both        Проверить обе базы данных (по умолчанию)"
    echo ""
    echo "Опции:"
    echo "  --save-report    Сохранить отчеты в JSON файлы"
    echo "  --help, -h       Показать эту справку"
    echo ""
    echo "Примеры:"
    echo "  $0 local"
    echo "  $0 production --save-report"
    echo "  $0 both --save-report"
    exit 0
}

# Функция для проверки локальной базы данных
check_local_database() {
    echo -e "${YELLOW}🏠 Проверка локальной базы данных...${NC}"
    echo ""
    
    # Проверяем, что Go модуль доступен
    if ! go mod tidy &>/dev/null; then
        echo -e "${RED}❌ Ошибка: не удалось обновить Go модули${NC}"
        return 1
    fi
    
    # Запускаем проверку
    local output_flag=""
    if [ "$SAVE_REPORT" = "true" ]; then
        output_flag="--output migration_check_local_${TIMESTAMP}.json"
    fi
    
    if go run cmd/verify_migration/main.go --env development $output_flag; then
        echo -e "${GREEN}✅ Локальная база данных: проверка пройдена${NC}"
        return 0
    else
        local exit_code=$?
        if [ $exit_code -eq 2 ]; then
            echo -e "${YELLOW}⚠️ Локальная база данных: есть предупреждения${NC}"
            return 2
        else
            echo -e "${RED}❌ Локальная база данных: обнаружены ошибки${NC}"
            return 1
        fi
    fi
}

# Функция для проверки продакшен базы данных
check_production_database() {
    echo -e "${YELLOW}🌐 Проверка продакшен базы данных...${NC}"
    echo ""
    
    # Проверяем доступность сервера
    if ! ssh -o ConnectTimeout=10 ${PRODUCTION_USER}@${PRODUCTION_SERVER} "echo 'Сервер доступен'" &>/dev/null; then
        echo -e "${RED}❌ Продакшен сервер недоступен${NC}"
        echo -e "${YELLOW}💡 Проверьте SSH ключи и доступ к серверу${NC}"
        return 1
    fi
    
    echo -e "${GREEN}✅ Соединение с продакшен сервером установлено${NC}"
    
    # Создаем скрипт проверки на сервере
    local output_flag=""
    local output_file=""
    if [ "$SAVE_REPORT" = "true" ]; then
        output_file="migration_check_production_${TIMESTAMP}.json"
        output_flag="--output $output_file"
    fi
    
    ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "cd ${PRODUCTION_PATH} && go run cmd/verify_migration/main.go --production $output_flag"
    local exit_code=$?
    
    # Скачиваем отчет если он был создан
    if [ "$SAVE_REPORT" = "true" ] && [ -n "$output_file" ]; then
        echo -e "${BLUE}📥 Скачивание отчета с продакшен сервера...${NC}"
        scp ${PRODUCTION_USER}@${PRODUCTION_SERVER}:${PRODUCTION_PATH}/$output_file . || true
        ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "rm -f ${PRODUCTION_PATH}/$output_file" || true
    fi
    
    if [ $exit_code -eq 0 ]; then
        echo -e "${GREEN}✅ Продакшен база данных: проверка пройдена${NC}"
        return 0
    elif [ $exit_code -eq 2 ]; then
        echo -e "${YELLOW}⚠️ Продакшен база данных: есть предупреждения${NC}"
        return 2
    else
        echo -e "${RED}❌ Продакшен база данных: обнаружены ошибки${NC}"
        return 1
    fi
}

# Функция для сравнения результатов
compare_environments() {
    echo -e "${PURPLE}🔄 Сравнение локального и продакшен окружений...${NC}"
    echo ""
    
    if [ "$SAVE_REPORT" = "true" ]; then
        local local_report="migration_check_local_${TIMESTAMP}.json"
        local prod_report="migration_check_production_${TIMESTAMP}.json"
        
        if [ -f "$local_report" ] && [ -f "$prod_report" ]; then
            echo -e "${BLUE}📊 Отчеты сохранены:${NC}"
            echo -e "  📄 Локальный: $local_report"
            echo -e "  📄 Продакшен: $prod_report"
            echo ""
            
            # Простое сравнение количества таблиц
            local local_tables=$(jq -r '.summary.total_tables' "$local_report" 2>/dev/null || echo "N/A")
            local prod_tables=$(jq -r '.summary.total_tables' "$prod_report" 2>/dev/null || echo "N/A")
            
            echo -e "${BLUE}📈 Сравнение:${NC}"
            echo -e "  🏠 Локальных таблиц: $local_tables"
            echo -e "  🌐 Продакшен таблиц: $prod_tables"
            
            if [ "$local_tables" != "$prod_tables" ] && [ "$local_tables" != "N/A" ] && [ "$prod_tables" != "N/A" ]; then
                echo -e "${YELLOW}⚠️ Количество таблиц отличается между окружениями${NC}"
            fi
        fi
    fi
}

# Парсинг аргументов
MODE="both"
SAVE_REPORT="false"

for arg in "$@"; do
    case $arg in
        local|production|both)
            MODE="$arg"
            ;;
        --save-report)
            SAVE_REPORT="true"
            ;;
        --help|-h)
            show_help
            ;;
        *)
            echo -e "${RED}❌ Неизвестный аргумент: $arg${NC}"
            echo "Используйте --help для справки"
            exit 1
            ;;
    esac
done

echo -e "${BLUE}🎯 Режим проверки: $MODE${NC}"
if [ "$SAVE_REPORT" = "true" ]; then
    echo -e "${BLUE}💾 Отчеты будут сохранены${NC}"
fi
echo ""

# Выполняем проверки
LOCAL_STATUS=0
PROD_STATUS=0

case $MODE in
    "local")
        check_local_database
        LOCAL_STATUS=$?
        ;;
    "production")
        check_production_database
        PROD_STATUS=$?
        ;;
    "both")
        echo -e "${BLUE}🔄 Проверка обоих окружений...${NC}"
        echo ""
        
        check_local_database
        LOCAL_STATUS=$?
        echo ""
        
        check_production_database
        PROD_STATUS=$?
        echo ""
        
        compare_environments
        ;;
esac

# Итоговый отчет
echo ""
echo -e "${BLUE}📋 Итоговый отчет:${NC}"
echo -e "${BLUE}==================${NC}"

if [ "$MODE" = "local" ] || [ "$MODE" = "both" ]; then
    case $LOCAL_STATUS in
        0) echo -e "🏠 Локальная БД: ${GREEN}✅ OK${NC}" ;;
        1) echo -e "🏠 Локальная БД: ${RED}❌ ОШИБКИ${NC}" ;;
        2) echo -e "🏠 Локальная БД: ${YELLOW}⚠️ ПРЕДУПРЕЖДЕНИЯ${NC}" ;;
    esac
fi

if [ "$MODE" = "production" ] || [ "$MODE" = "both" ]; then
    case $PROD_STATUS in
        0) echo -e "🌐 Продакшен БД: ${GREEN}✅ OK${NC}" ;;
        1) echo -e "🌐 Продакшен БД: ${RED}❌ ОШИБКИ${NC}" ;;
        2) echo -e "🌐 Продакшен БД: ${YELLOW}⚠️ ПРЕДУПРЕЖДЕНИЯ${NC}" ;;
    esac
fi

# Рекомендации
echo ""
echo -e "${BLUE}💡 Рекомендации:${NC}"

if [ $LOCAL_STATUS -eq 1 ] || [ $PROD_STATUS -eq 1 ]; then
    echo -e "  ${RED}🚨 Обнаружены критические ошибки миграций${NC}"
    echo -e "  ${YELLOW}🔧 Выполните: ./run_migrations.sh${NC}"
    echo -e "  ${YELLOW}🔧 Для продакшена: ./run_production_migrations.sh${NC}"
elif [ $LOCAL_STATUS -eq 2 ] || [ $PROD_STATUS -eq 2 ]; then
    echo -e "  ${YELLOW}⚠️ Есть предупреждения, рекомендуется проверка${NC}"
    echo -e "  ${BLUE}📖 Изучите детали в выводе выше${NC}"
else
    echo -e "  ${GREEN}✅ Все миграции выполнены корректно${NC}"
    echo -e "  ${GREEN}🎉 Базы данных готовы к работе${NC}"
fi

# Определяем код выхода
FINAL_EXIT_CODE=0
if [ $LOCAL_STATUS -eq 1 ] || [ $PROD_STATUS -eq 1 ]; then
    FINAL_EXIT_CODE=1
elif [ $LOCAL_STATUS -eq 2 ] || [ $PROD_STATUS -eq 2 ]; then
    FINAL_EXIT_CODE=2
fi

echo ""
echo -e "${BLUE}🏁 Проверка завершена${NC}"
exit $FINAL_EXIT_CODE
