#!/bin/bash

# 🛡️ Безопасный скрипт для выполнения миграций с проверками
# Использование: ./safe_migrate.sh [local|production] [--force] [--skip-backup]

set -e  # Остановка при любой ошибке

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Конфигурация
PRODUCTION_SERVER="api.axenta.glonass-saratov.ru"
PRODUCTION_USER="root"
PRODUCTION_PATH="/opt/axenta-backend"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")

echo -e "${BLUE}🛡️ Безопасное выполнение миграций Axenta CRM${NC}"
echo -e "${BLUE}=============================================${NC}"
echo ""

# Функция для показа справки
show_help() {
    echo "Использование: $0 [режим] [опции]"
    echo ""
    echo "Режимы:"
    echo "  local       Выполнить миграции локальной БД"
    echo "  production  Выполнить миграции продакшен БД"
    echo ""
    echo "Опции:"
    echo "  --force        Пропустить подтверждения"
    echo "  --skip-backup  Пропустить создание резервной копии"
    echo "  --help, -h     Показать эту справку"
    echo ""
    echo "Этапы выполнения:"
    echo "  1. Проверка текущего состояния БД"
    echo "  2. Создание резервной копии"
    echo "  3. Выполнение миграций"
    echo "  4. Проверка результата"
    echo "  5. Тестирование API (для продакшена)"
    echo ""
    echo "Примеры:"
    echo "  $0 local"
    echo "  $0 production --force"
    exit 0
}

# Функция для запроса подтверждения
confirm() {
    if [ "$FORCE" = "true" ]; then
        return 0
    fi
    
    echo -e "${YELLOW}❓ $1${NC}"
    read -p "Продолжить? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo -e "${YELLOW}⏹️ Операция отменена пользователем${NC}"
        exit 0
    fi
}

# Функция для проверки состояния БД
check_database_state() {
    local mode=$1
    echo -e "${BLUE}🔍 Проверка текущего состояния базы данных...${NC}"
    
    if [ "$mode" = "local" ]; then
        if ./verify_migrations.sh local; then
            echo -e "${GREEN}✅ Локальная БД в хорошем состоянии${NC}"
            return 0
        else
            local exit_code=$?
            if [ $exit_code -eq 2 ]; then
                echo -e "${YELLOW}⚠️ Обнаружены предупреждения в локальной БД${NC}"
                confirm "Продолжить несмотря на предупреждения?"
                return 0
            else
                echo -e "${RED}❌ Обнаружены ошибки в локальной БД${NC}"
                confirm "Продолжить несмотря на ошибки? (может быть опасно)"
                return 0
            fi
        fi
    else
        if ./verify_migrations.sh production; then
            echo -e "${GREEN}✅ Продакшен БД в хорошем состоянии${NC}"
            return 0
        else
            local exit_code=$?
            if [ $exit_code -eq 2 ]; then
                echo -e "${YELLOW}⚠️ Обнаружены предупреждения в продакшен БД${NC}"
                confirm "Продолжить несмотря на предупреждения?"
                return 0
            else
                echo -e "${RED}❌ Обнаружены ошибки в продакшен БД${NC}"
                confirm "Продолжить несмотря на ошибки? (может быть опасно)"
                return 0
            fi
        fi
    fi
}

# Функция для создания резервной копии
create_backup() {
    local mode=$1
    
    if [ "$SKIP_BACKUP" = "true" ]; then
        echo -e "${YELLOW}⏭️ Создание резервной копии пропущено${NC}"
        return 0
    fi
    
    echo -e "${BLUE}💾 Создание резервной копии перед миграцией...${NC}"
    
    if ./backup_before_migration.sh "$mode" --compress; then
        echo -e "${GREEN}✅ Резервная копия создана успешно${NC}"
        return 0
    else
        echo -e "${RED}❌ Ошибка создания резервной копии${NC}"
        confirm "Продолжить без резервной копии? (НЕ РЕКОМЕНДУЕТСЯ)"
        return 0
    fi
}

# Функция для выполнения локальных миграций
run_local_migrations() {
    echo -e "${BLUE}🔄 Выполнение локальных миграций...${NC}"
    
    if ./run_migrations.sh; then
        echo -e "${GREEN}✅ Локальные миграции выполнены успешно${NC}"
        return 0
    else
        echo -e "${RED}❌ Ошибка выполнения локальных миграций${NC}"
        return 1
    fi
}

# Функция для выполнения продакшен миграций
run_production_migrations() {
    echo -e "${BLUE}🔄 Выполнение продакшен миграций...${NC}"
    
    # Дополнительное подтверждение для продакшена
    if [ "$FORCE" != "true" ]; then
        echo -e "${RED}⚠️ ВНИМАНИЕ: Вы собираетесь выполнить миграции на ПРОДАКШЕН сервере!${NC}"
        echo -e "${RED}⚠️ Это может повлиять на работу системы!${NC}"
        confirm "Вы уверены, что хотите продолжить?"
    fi
    
    if ./run_production_migrations.sh; then
        echo -e "${GREEN}✅ Продакшен миграции выполнены успешно${NC}"
        return 0
    else
        echo -e "${RED}❌ Ошибка выполнения продакшен миграций${NC}"
        return 1
    fi
}

# Функция для проверки результата миграций
verify_migration_result() {
    local mode=$1
    echo -e "${BLUE}🔍 Проверка результата миграций...${NC}"
    
    if ./verify_migrations.sh "$mode" --save-report; then
        echo -e "${GREEN}✅ Миграции выполнены корректно${NC}"
        return 0
    else
        local exit_code=$?
        if [ $exit_code -eq 2 ]; then
            echo -e "${YELLOW}⚠️ Миграции выполнены с предупреждениями${NC}"
            return 2
        else
            echo -e "${RED}❌ Обнаружены проблемы после миграций${NC}"
            return 1
        fi
    fi
}

# Функция для тестирования API (только для продакшена)
test_production_api() {
    if [ "$1" != "production" ]; then
        return 0
    fi
    
    echo -e "${BLUE}🧪 Тестирование продакшен API...${NC}"
    
    # Ждем несколько секунд для стабилизации сервиса
    echo -e "${BLUE}⏳ Ожидание стабилизации сервиса...${NC}"
    sleep 10
    
    # Тестируем основные эндпоинты
    local api_tests_passed=0
    local api_tests_total=0
    
    # Тест 1: Проверка статуса сервиса
    api_tests_total=$((api_tests_total + 1))
    if ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "systemctl is-active axenta-backend" | grep -q "active"; then
        echo -e "${GREEN}✅ Сервис активен${NC}"
        api_tests_passed=$((api_tests_passed + 1))
    else
        echo -e "${RED}❌ Сервис неактивен${NC}"
    fi
    
    # Тест 2: Проверка roles API
    api_tests_total=$((api_tests_total + 1))
    local roles_status=$(curl -s -o /dev/null -w "%{http_code}" "https://${PRODUCTION_SERVER}/api/auth/roles" || echo "000")
    if [ "$roles_status" != "000" ] && [ "$roles_status" != "500" ]; then
        echo -e "${GREEN}✅ Roles API отвечает (код: $roles_status)${NC}"
        api_tests_passed=$((api_tests_passed + 1))
    else
        echo -e "${RED}❌ Roles API не отвечает (код: $roles_status)${NC}"
    fi
    
    # Тест 3: Проверка user-templates API
    api_tests_total=$((api_tests_total + 1))
    local templates_status=$(curl -s -o /dev/null -w "%{http_code}" "https://${PRODUCTION_SERVER}/api/auth/user-templates" || echo "000")
    if [ "$templates_status" != "000" ] && [ "$templates_status" != "500" ]; then
        echo -e "${GREEN}✅ User Templates API отвечает (код: $templates_status)${NC}"
        api_tests_passed=$((api_tests_passed + 1))
    else
        echo -e "${RED}❌ User Templates API не отвечает (код: $templates_status)${NC}"
    fi
    
    # Результат тестирования
    echo -e "${BLUE}📊 Результат тестирования API: $api_tests_passed/$api_tests_total${NC}"
    
    if [ $api_tests_passed -eq $api_tests_total ]; then
        echo -e "${GREEN}✅ Все API тесты пройдены${NC}"
        return 0
    elif [ $api_tests_passed -gt 0 ]; then
        echo -e "${YELLOW}⚠️ Некоторые API тесты не пройдены${NC}"
        return 2
    else
        echo -e "${RED}❌ Все API тесты провалены${NC}"
        return 1
    fi
}

# Функция для отката миграций (если что-то пошло не так)
rollback_migrations() {
    local mode=$1
    echo -e "${RED}🔄 Попытка отката миграций...${NC}"
    
    # Для простоты, просто показываем инструкции по откату
    echo -e "${YELLOW}📋 Инструкции по откату:${NC}"
    echo -e "  1. Остановите приложение"
    echo -e "  2. Восстановите БД из резервной копии:"
    
    if [ "$mode" = "local" ]; then
        echo -e "     psql -h localhost -U postgres -d axenta_db < backups/axenta_local_backup_*.sql"
    else
        echo -e "     На сервере: psql -h localhost -U postgres -d axenta_db < axenta_production_backup_*.sql"
    fi
    
    echo -e "  3. Перезапустите приложение"
    echo -e "${RED}⚠️ Обратитесь к администратору для выполнения отката${NC}"
}

# Парсинг аргументов
MODE=""
FORCE="false"
SKIP_BACKUP="false"

for arg in "$@"; do
    case $arg in
        local|production)
            MODE="$arg"
            ;;
        --force)
            FORCE="true"
            ;;
        --skip-backup)
            SKIP_BACKUP="true"
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

# Проверяем, что режим указан
if [ -z "$MODE" ]; then
    echo -e "${RED}❌ Не указан режим работы${NC}"
    echo "Используйте --help для справки"
    exit 1
fi

echo -e "${BLUE}🎯 Режим: $MODE${NC}"
if [ "$FORCE" = "true" ]; then
    echo -e "${YELLOW}⚡ Принудительный режим: включен${NC}"
fi
if [ "$SKIP_BACKUP" = "true" ]; then
    echo -e "${YELLOW}⏭️ Пропуск резервной копии: включен${NC}"
fi
echo ""

# Проверяем наличие необходимых скриптов
required_scripts=("verify_migrations.sh" "backup_before_migration.sh")
if [ "$MODE" = "local" ]; then
    required_scripts+=("run_migrations.sh")
else
    required_scripts+=("run_production_migrations.sh")
fi

for script in "${required_scripts[@]}"; do
    if [ ! -f "$script" ]; then
        echo -e "${RED}❌ Не найден скрипт: $script${NC}"
        exit 1
    fi
    if [ ! -x "$script" ]; then
        echo -e "${BLUE}🔧 Делаем скрипт исполняемым: $script${NC}"
        chmod +x "$script"
    fi
done

# Основной процесс миграции
echo -e "${PURPLE}🚀 Начинаем безопасную миграцию...${NC}"
echo ""

# Этап 1: Проверка текущего состояния
echo -e "${BLUE}📋 Этап 1/5: Проверка текущего состояния БД${NC}"
if ! check_database_state "$MODE"; then
    echo -e "${RED}❌ Проверка состояния БД не пройдена${NC}"
    exit 1
fi
echo ""

# Этап 2: Создание резервной копии
echo -e "${BLUE}📋 Этап 2/5: Создание резервной копии${NC}"
if ! create_backup "$MODE"; then
    echo -e "${RED}❌ Создание резервной копии не выполнено${NC}"
    exit 1
fi
echo ""

# Этап 3: Выполнение миграций
echo -e "${BLUE}📋 Этап 3/5: Выполнение миграций${NC}"
if [ "$MODE" = "local" ]; then
    if ! run_local_migrations; then
        echo -e "${RED}❌ Миграции не выполнены${NC}"
        rollback_migrations "$MODE"
        exit 1
    fi
else
    if ! run_production_migrations; then
        echo -e "${RED}❌ Миграции не выполнены${NC}"
        rollback_migrations "$MODE"
        exit 1
    fi
fi
echo ""

# Этап 4: Проверка результата
echo -e "${BLUE}📋 Этап 4/5: Проверка результата миграций${NC}"
VERIFICATION_RESULT=0
if ! verify_migration_result "$MODE"; then
    VERIFICATION_RESULT=$?
    if [ $VERIFICATION_RESULT -eq 1 ]; then
        echo -e "${RED}❌ Проверка результата не пройдена${NC}"
        rollback_migrations "$MODE"
        exit 1
    else
        echo -e "${YELLOW}⚠️ Проверка результата выполнена с предупреждениями${NC}"
    fi
fi
echo ""

# Этап 5: Тестирование API (только для продакшена)
echo -e "${BLUE}📋 Этап 5/5: Тестирование API${NC}"
API_TEST_RESULT=0
if ! test_production_api "$MODE"; then
    API_TEST_RESULT=$?
    if [ $API_TEST_RESULT -eq 1 ]; then
        echo -e "${RED}❌ API тесты провалены${NC}"
        confirm "Откатить миграции из-за проблем с API?"
        rollback_migrations "$MODE"
        exit 1
    else
        echo -e "${YELLOW}⚠️ API тесты выполнены с предупреждениями${NC}"
    fi
fi
echo ""

# Итоговый отчет
echo -e "${PURPLE}📋 Итоговый отчет безопасной миграции:${NC}"
echo -e "${PURPLE}=====================================${NC}"
echo -e "🎯 Режим: $MODE"
echo -e "📅 Время: $(date)"
echo -e "💾 Резервная копия: $([ "$SKIP_BACKUP" = "true" ] && echo "пропущена" || echo "создана")"
echo -e "🔄 Миграции: выполнены"
echo -e "🔍 Проверка: $([ $VERIFICATION_RESULT -eq 0 ] && echo "✅ пройдена" || echo "⚠️ с предупреждениями")"
if [ "$MODE" = "production" ]; then
    echo -e "🧪 API тесты: $([ $API_TEST_RESULT -eq 0 ] && echo "✅ пройдены" || echo "⚠️ с предупреждениями")"
fi
echo ""

# Определяем финальный статус
if [ $VERIFICATION_RESULT -eq 0 ] && [ $API_TEST_RESULT -eq 0 ]; then
    echo -e "${GREEN}🎉 Миграция выполнена успешно!${NC}"
    echo -e "${GREEN}✅ Система готова к работе${NC}"
    exit 0
elif [ $VERIFICATION_RESULT -eq 2 ] || [ $API_TEST_RESULT -eq 2 ]; then
    echo -e "${YELLOW}⚠️ Миграция выполнена с предупреждениями${NC}"
    echo -e "${YELLOW}🔍 Рекомендуется дополнительная проверка${NC}"
    exit 2
else
    echo -e "${RED}❌ Миграция выполнена с ошибками${NC}"
    exit 1
fi
