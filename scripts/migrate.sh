#!/bin/bash

# Скрипт для запуска миграций базы данных Axenta CRM
# Использование: ./scripts/migrate.sh [опции]

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Функция для вывода сообщений
log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

# Функция для показа справки
show_help() {
    echo "Скрипт миграции базы данных Axenta CRM"
    echo ""
    echo "Использование:"
    echo "  ./scripts/migrate.sh [опции]"
    echo ""
    echo "Опции:"
    echo "  --help, -h           Показать эту справку"
    echo "  --dry-run           Показать план миграций без выполнения"
    echo "  --global            Выполнить только глобальные миграции"
    echo "  --build             Собрать утилиту миграции перед запуском"
    echo "  --create-schema NAME --company-id ID  Создать схему для компании"
    echo ""
    echo "Примеры:"
    echo "  ./scripts/migrate.sh                    # Выполнить все миграции"
    echo "  ./scripts/migrate.sh --dry-run          # Показать план"
    echo "  ./scripts/migrate.sh --global           # Только глобальные таблицы"
    echo "  ./scripts/migrate.sh --build            # Пересобрать и запустить"
    echo ""
}

# Переменные по умолчанию
DRY_RUN=false
GLOBAL_ONLY=false
BUILD=false
CREATE_SCHEMA=""
COMPANY_ID=""

# Парсинг аргументов
while [[ $# -gt 0 ]]; do
    case $1 in
        --help|-h)
            show_help
            exit 0
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --global)
            GLOBAL_ONLY=true
            shift
            ;;
        --build)
            BUILD=true
            shift
            ;;
        --create-schema)
            CREATE_SCHEMA="$2"
            shift 2
            ;;
        --company-id)
            COMPANY_ID="$2"
            shift 2
            ;;
        *)
            log_error "Неизвестная опция: $1"
            show_help
            exit 1
            ;;
    esac
done

# Проверяем, что мы в корневой директории проекта
if [[ ! -f "go.mod" ]]; then
    log_error "Скрипт должен запускаться из корневой директории проекта"
    exit 1
fi

# Проверяем наличие файла .env
if [[ ! -f ".env" ]]; then
    log_warning "Файл .env не найден. Убедитесь, что переменные окружения настроены"
fi

# Загружаем переменные окружения если есть .env
if [[ -f ".env" ]]; then
    log_info "Загружаем переменные окружения из .env"
    export $(grep -v '^#' .env | xargs)
fi

# Собираем утилиту миграции если нужно
MIGRATE_BINARY="./migrate"
if [[ "$BUILD" == "true" ]] || [[ ! -f "$MIGRATE_BINARY" ]]; then
    log_info "Собираем утилиту миграции..."
    go build -o "$MIGRATE_BINARY" ./cmd/migrate/
    if [[ $? -eq 0 ]]; then
        log_success "Утилита миграции собрана"
    else
        log_error "Ошибка сборки утилиты миграции"
        exit 1
    fi
fi

# Проверяем подключение к базе данных
log_info "Проверяем подключение к базе данных..."

# Формируем аргументы для утилиты миграции
MIGRATE_ARGS=""

if [[ "$DRY_RUN" == "true" ]]; then
    MIGRATE_ARGS="$MIGRATE_ARGS -dry-run"
fi

if [[ "$GLOBAL_ONLY" == "true" ]]; then
    MIGRATE_ARGS="$MIGRATE_ARGS -global"
fi

if [[ -n "$CREATE_SCHEMA" ]]; then
    if [[ -z "$COMPANY_ID" ]]; then
        log_error "Для создания схемы необходимо указать ID компании с --company-id"
        exit 1
    fi
    MIGRATE_ARGS="$MIGRATE_ARGS -create-schema $CREATE_SCHEMA -company-id $COMPANY_ID"
fi

# Запускаем миграции
log_info "Запускаем миграции с аргументами: $MIGRATE_ARGS"
echo ""

if $MIGRATE_BINARY $MIGRATE_ARGS; then
    echo ""
    log_success "Миграции завершены успешно!"
    
    # Показываем статистику если это не dry-run
    if [[ "$DRY_RUN" != "true" ]] && [[ -z "$CREATE_SCHEMA" ]]; then
        echo ""
        log_info "Для проверки состояния базы данных используйте:"
        echo "  ./scripts/migrate.sh --dry-run"
    fi
else
    echo ""
    log_error "Ошибка выполнения миграций!"
    exit 1
fi

# Очистка
if [[ "$BUILD" == "true" ]]; then
    log_info "Удаляем временный файл утилиты миграции"
    rm -f "$MIGRATE_BINARY"
fi
