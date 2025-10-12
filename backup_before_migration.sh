#!/bin/bash

# 💾 Скрипт для создания резервной копии базы данных перед миграциями
# Использование: ./backup_before_migration.sh [local|production] [--compress]

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
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_DIR="./backups"

echo -e "${BLUE}💾 Создание резервной копии базы данных Axenta CRM${NC}"
echo -e "${BLUE}===================================================${NC}"
echo ""

# Функция для показа справки
show_help() {
    echo "Использование: $0 [режим] [опции]"
    echo ""
    echo "Режимы:"
    echo "  local       Создать резервную копию локальной БД"
    echo "  production  Создать резервную копию продакшен БД"
    echo ""
    echo "Опции:"
    echo "  --compress  Сжать резервную копию с помощью gzip"
    echo "  --help, -h  Показать эту справку"
    echo ""
    echo "Примеры:"
    echo "  $0 local"
    echo "  $0 production --compress"
    exit 0
}

# Функция для загрузки переменных окружения
load_env_vars() {
    local env_file=".env"
    if [ "$1" = "production" ]; then
        env_file=".env.production"
    fi
    
    if [ -f "$env_file" ]; then
        echo -e "${BLUE}📄 Загрузка переменных из $env_file${NC}"
        export $(grep -v '^#' "$env_file" | xargs)
    else
        echo -e "${YELLOW}⚠️ Файл $env_file не найден, используем переменные по умолчанию${NC}"
    fi
}

# Функция для создания резервной копии локальной БД
backup_local_database() {
    echo -e "${YELLOW}🏠 Создание резервной копии локальной базы данных...${NC}"
    
    # Загружаем переменные окружения
    load_env_vars "local"
    
    # Устанавливаем значения по умолчанию
    DB_HOST=${DB_HOST:-"localhost"}
    DB_PORT=${DB_PORT:-"5432"}
    DB_USER=${DB_USER:-"postgres"}
    DB_NAME=${DB_NAME:-"axenta_db"}
    
    echo -e "${BLUE}🗄️ База данных: $DB_USER@$DB_HOST:$DB_PORT/$DB_NAME${NC}"
    
    # Создаем директорию для резервных копий
    mkdir -p "$BACKUP_DIR"
    
    # Имя файла резервной копии
    local backup_file="$BACKUP_DIR/axenta_local_backup_${TIMESTAMP}.sql"
    
    # Проверяем доступность базы данных
    if ! pg_isready -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" &>/dev/null; then
        echo -e "${RED}❌ База данных недоступна${NC}"
        echo -e "${YELLOW}💡 Убедитесь, что PostgreSQL запущен и настроен корректно${NC}"
        return 1
    fi
    
    echo -e "${BLUE}🔄 Создание резервной копии...${NC}"
    
    # Создаем резервную копию
    if PGPASSWORD="$DB_PASSWORD" pg_dump \
        -h "$DB_HOST" \
        -p "$DB_PORT" \
        -U "$DB_USER" \
        -d "$DB_NAME" \
        --verbose \
        --no-password \
        --format=plain \
        --no-owner \
        --no-privileges \
        > "$backup_file"; then
        
        echo -e "${GREEN}✅ Резервная копия создана: $backup_file${NC}"
        
        # Сжимаем если нужно
        if [ "$COMPRESS" = "true" ]; then
            echo -e "${BLUE}🗜️ Сжатие резервной копии...${NC}"
            gzip "$backup_file"
            backup_file="${backup_file}.gz"
            echo -e "${GREEN}✅ Резервная копия сжата: $backup_file${NC}"
        fi
        
        # Показываем размер файла
        local file_size=$(du -h "$backup_file" | cut -f1)
        echo -e "${BLUE}📊 Размер резервной копии: $file_size${NC}"
        
        return 0
    else
        echo -e "${RED}❌ Ошибка создания резервной копии${NC}"
        return 1
    fi
}

# Функция для создания резервной копии продакшен БД
backup_production_database() {
    echo -e "${YELLOW}🌐 Создание резервной копии продакшен базы данных...${NC}"
    
    # Проверяем доступность сервера
    if ! ssh -o ConnectTimeout=10 ${PRODUCTION_USER}@${PRODUCTION_SERVER} "echo 'Сервер доступен'" &>/dev/null; then
        echo -e "${RED}❌ Продакшен сервер недоступен${NC}"
        echo -e "${YELLOW}💡 Проверьте SSH ключи и доступ к серверу${NC}"
        return 1
    fi
    
    echo -e "${GREEN}✅ Соединение с продакшен сервером установлено${NC}"
    
    # Создаем директорию для резервных копий
    mkdir -p "$BACKUP_DIR"
    
    # Имя файла резервной копии
    local backup_file="axenta_production_backup_${TIMESTAMP}.sql"
    local local_backup_file="$BACKUP_DIR/$backup_file"
    
    # Создаем скрипт резервного копирования на сервере
    echo -e "${BLUE}📝 Создание скрипта резервного копирования на сервере...${NC}"
    
    ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "cat > ${PRODUCTION_PATH}/backup_script.sh << 'EOF'
#!/bin/bash
set -e

# Загружаем переменные окружения
if [ -f .env.production ]; then
    export \$(grep -v '^#' .env.production | xargs)
elif [ -f .env ]; then
    export \$(grep -v '^#' .env | xargs)
fi

# Устанавливаем значения по умолчанию
DB_HOST=\${DB_HOST:-\"localhost\"}
DB_PORT=\${DB_PORT:-\"5432\"}
DB_USER=\${DB_USER:-\"postgres\"}
DB_NAME=\${DB_NAME:-\"axenta_db\"}

echo \"🗄️ База данных: \$DB_USER@\$DB_HOST:\$DB_PORT/\$DB_NAME\"

# Проверяем доступность базы данных
if ! pg_isready -h \"\$DB_HOST\" -p \"\$DB_PORT\" -U \"\$DB_USER\" -d \"\$DB_NAME\" &>/dev/null; then
    echo \"❌ База данных недоступна\"
    exit 1
fi

echo \"🔄 Создание резервной копии...\"

# Создаем резервную копию
PGPASSWORD=\"\$DB_PASSWORD\" pg_dump \\
    -h \"\$DB_HOST\" \\
    -p \"\$DB_PORT\" \\
    -U \"\$DB_USER\" \\
    -d \"\$DB_NAME\" \\
    --verbose \\
    --no-password \\
    --format=plain \\
    --no-owner \\
    --no-privileges \\
    > \"$backup_file\"

if [ \$? -eq 0 ]; then
    echo \"✅ Резервная копия создана: $backup_file\"
    
    # Сжимаем если нужно
    if [ \"$COMPRESS\" = \"true\" ]; then
        echo \"🗜️ Сжатие резервной копии...\"
        gzip \"$backup_file\"
        echo \"✅ Резервная копия сжата: ${backup_file}.gz\"
    fi
    
    # Показываем размер файла
    if [ \"$COMPRESS\" = \"true\" ]; then
        file_size=\$(du -h \"${backup_file}.gz\" | cut -f1)
        echo \"📊 Размер резервной копии: \$file_size\"
    else
        file_size=\$(du -h \"$backup_file\" | cut -f1)
        echo \"📊 Размер резервной копии: \$file_size\"
    fi
else
    echo \"❌ Ошибка создания резервной копии\"
    exit 1
fi
EOF"

    # Делаем скрипт исполняемым и запускаем
    ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "chmod +x ${PRODUCTION_PATH}/backup_script.sh"
    
    echo -e "${BLUE}🔄 Выполнение резервного копирования на сервере...${NC}"
    if ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "cd ${PRODUCTION_PATH} && ./backup_script.sh"; then
        echo -e "${GREEN}✅ Резервная копия создана на сервере${NC}"
        
        # Определяем имя файла для скачивания
        local remote_file="$backup_file"
        if [ "$COMPRESS" = "true" ]; then
            remote_file="${backup_file}.gz"
        fi
        
        # Скачиваем резервную копию
        echo -e "${BLUE}📥 Скачивание резервной копии...${NC}"
        if scp ${PRODUCTION_USER}@${PRODUCTION_SERVER}:${PRODUCTION_PATH}/$remote_file "$local_backup_file"; then
            echo -e "${GREEN}✅ Резервная копия скачана: $local_backup_file${NC}"
            
            # Показываем размер локального файла
            local file_size=$(du -h "$local_backup_file" | cut -f1)
            echo -e "${BLUE}📊 Размер локального файла: $file_size${NC}"
            
            # Удаляем файлы на сервере
            ssh ${PRODUCTION_USER}@${PRODUCTION_SERVER} "rm -f ${PRODUCTION_PATH}/$remote_file ${PRODUCTION_PATH}/backup_script.sh"
            
            return 0
        else
            echo -e "${RED}❌ Ошибка скачивания резервной копии${NC}"
            return 1
        fi
    else
        echo -e "${RED}❌ Ошибка создания резервной копии на сервере${NC}"
        return 1
    fi
}

# Функция для проверки инструментов
check_tools() {
    echo -e "${BLUE}🔧 Проверка необходимых инструментов...${NC}"
    
    local missing_tools=()
    
    # Проверяем pg_dump
    if ! command -v pg_dump &> /dev/null; then
        missing_tools+=("pg_dump (PostgreSQL client tools)")
    fi
    
    # Проверяем pg_isready
    if ! command -v pg_isready &> /dev/null; then
        missing_tools+=("pg_isready (PostgreSQL client tools)")
    fi
    
    # Проверяем gzip если нужно сжатие
    if [ "$COMPRESS" = "true" ] && ! command -v gzip &> /dev/null; then
        missing_tools+=("gzip")
    fi
    
    if [ ${#missing_tools[@]} -gt 0 ]; then
        echo -e "${RED}❌ Отсутствуют необходимые инструменты:${NC}"
        for tool in "${missing_tools[@]}"; do
            echo -e "  - $tool"
        done
        echo -e "${YELLOW}💡 Установите PostgreSQL client tools${NC}"
        return 1
    fi
    
    echo -e "${GREEN}✅ Все необходимые инструменты доступны${NC}"
    return 0
}

# Парсинг аргументов
MODE=""
COMPRESS="false"

for arg in "$@"; do
    case $arg in
        local|production)
            MODE="$arg"
            ;;
        --compress)
            COMPRESS="true"
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
if [ "$COMPRESS" = "true" ]; then
    echo -e "${BLUE}🗜️ Сжатие: включено${NC}"
fi
echo ""

# Проверяем инструменты
if ! check_tools; then
    exit 1
fi
echo ""

# Выполняем резервное копирование
case $MODE in
    "local")
        if backup_local_database; then
            echo -e "${GREEN}🎉 Резервная копия локальной БД создана успешно${NC}"
            exit 0
        else
            echo -e "${RED}❌ Ошибка создания резервной копии локальной БД${NC}"
            exit 1
        fi
        ;;
    "production")
        if backup_production_database; then
            echo -e "${GREEN}🎉 Резервная копия продакшен БД создана успешно${NC}"
            exit 0
        else
            echo -e "${RED}❌ Ошибка создания резервной копии продакшен БД${NC}"
            exit 1
        fi
        ;;
esac
