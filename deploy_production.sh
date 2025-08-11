#!/bin/bash

# 🚀 Автоматический скрипт развертывания Axenta Backend в продакшене
# Автор: ProfMonitor Team
# Версия: 1.0

set -e  # Остановка при любой ошибке

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Функции для вывода
print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_header() {
    echo -e "${BLUE}"
    echo "================================================================"
    echo "🚀 AXENTA BACKEND PRODUCTION DEPLOYMENT"
    echo "================================================================"
    echo -e "${NC}"
}

# Проверка прав root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "Этот скрипт должен запускаться с правами root (sudo)"
        exit 1
    fi
}

# Проверка операционной системы
check_os() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        OS=$NAME
        VER=$VERSION_ID
        print_info "Обнаружена ОС: $OS $VER"
    else
        print_error "Не удалось определить операционную систему"
        exit 1
    fi
}

# Установка зависимостей
install_dependencies() {
    print_info "Установка системных зависимостей..."
    
    if [[ "$OS" == *"Ubuntu"* ]] || [[ "$OS" == *"Debian"* ]]; then
        apt update
        apt install -y git curl wget nginx postgresql postgresql-contrib supervisor ufw fail2ban
    elif [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
        yum update -y
        yum install -y git curl wget nginx postgresql postgresql-server postgresql-contrib supervisor firewalld
    else
        print_error "Неподдерживаемая операционная система: $OS"
        exit 1
    fi
    
    print_success "Системные зависимости установлены"
}

# Установка Go
install_go() {
    print_info "Установка Go..."
    
    # Проверка, установлен ли Go
    if command -v go &> /dev/null; then
        GO_VERSION=$(go version | awk '{print $3}')
        print_info "Go уже установлен: $GO_VERSION"
        return
    fi
    
    cd /tmp
    wget -q https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
    tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
    
    # Добавление в PATH для всех пользователей
    echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    export PATH=$PATH:/usr/local/go/bin
    
    print_success "Go установлен: $(go version)"
}

# Создание пользователя приложения
create_app_user() {
    print_info "Создание пользователя приложения..."
    
    if id "axenta" &>/dev/null; then
        print_info "Пользователь axenta уже существует"
    else
        useradd -m -s /bin/bash axenta
        print_success "Пользователь axenta создан"
    fi
    
    mkdir -p /opt/axenta
    chown axenta:axenta /opt/axenta
}

# Настройка PostgreSQL
setup_postgresql() {
    print_info "Настройка PostgreSQL..."
    
    # Инициализация БД (для CentOS)
    if [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
        if [[ ! -f /var/lib/pgsql/data/postgresql.conf ]]; then
            postgresql-setup initdb
        fi
    fi
    
    systemctl start postgresql
    systemctl enable postgresql
    
    # Запрос пароля для БД
    read -s -p "Введите пароль для пользователя БД axenta_user: " DB_PASSWORD
    echo
    
    # Создание БД и пользователя
    sudo -u postgres psql << EOF
CREATE DATABASE axenta_db;
CREATE USER axenta_user WITH PASSWORD '$DB_PASSWORD';
GRANT ALL PRIVILEGES ON DATABASE axenta_db TO axenta_user;
ALTER USER axenta_user CREATEDB;
\q
EOF
    
    print_success "PostgreSQL настроен"
    
    # Сохранение пароля в переменной для дальнейшего использования
    echo "$DB_PASSWORD" > /tmp/db_password
}

# Клонирование репозитория
clone_repository() {
    print_info "Клонирование репозитория..."
    
    if [[ -d /opt/axenta/backend ]]; then
        print_warning "Директория /opt/axenta/backend уже существует. Обновляем..."
        cd /opt/axenta/backend
        sudo -u axenta git pull origin main
    else
        sudo -u axenta git clone https://github.com/novaconnectkz/backend_axenta.git /opt/axenta/backend
    fi
    
    print_success "Репозиторий клонирован/обновлен"
}

# Создание файла окружения
create_env_file() {
    print_info "Создание файла переменных окружения..."
    
    DB_PASSWORD=$(cat /tmp/db_password)
    
    # Генерация случайного JWT секрета
    JWT_SECRET=$(openssl rand -base64 32)
    
    # Запрос доменного имени
    read -p "Введите доменное имя для CORS (например: yourdomain.com): " DOMAIN_NAME
    
    sudo -u axenta tee /opt/axenta/backend/.env << EOF
# Основные настройки
APP_ENV=production
APP_PORT=8080
APP_HOST=0.0.0.0

# База данных
DB_HOST=localhost
DB_PORT=5432
DB_NAME=axenta_db
DB_USER=axenta_user
DB_PASSWORD=$DB_PASSWORD
DB_SSLMODE=require

# JWT настройки
JWT_SECRET=$JWT_SECRET
JWT_EXPIRES_IN=24h

# Axenta Cloud API
AXENTA_API_URL=https://axenta.cloud/api
AXENTA_TIMEOUT=30s

# Логирование
LOG_LEVEL=info
LOG_FORMAT=json

# CORS настройки
CORS_ALLOWED_ORIGINS=https://$DOMAIN_NAME,https://www.$DOMAIN_NAME
CORS_ALLOWED_METHODS=GET,POST,PUT,DELETE,OPTIONS
CORS_ALLOWED_HEADERS=Content-Type,Authorization

# Безопасность
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_WINDOW=1m
MAX_REQUEST_SIZE=10MB

# Мониторинг
HEALTH_CHECK_PATH=/health
METRICS_PATH=/metrics
EOF
    
    chmod 600 /opt/axenta/backend/.env
    chown axenta:axenta /opt/axenta/backend/.env
    
    # Удаление временного файла с паролем
    rm -f /tmp/db_password
    
    print_success "Файл .env создан"
}

# Сборка приложения
build_application() {
    print_info "Сборка приложения..."
    
    cd /opt/axenta/backend
    sudo -u axenta /usr/local/go/bin/go mod download
    sudo -u axenta /usr/local/go/bin/go mod verify
    sudo -u axenta /usr/local/go/bin/go build -ldflags="-w -s" -o axenta_backend main.go
    chmod +x axenta_backend
    
    print_success "Приложение собрано"
}

# Создание systemd службы
create_systemd_service() {
    print_info "Создание systemd службы..."
    
    tee /etc/systemd/system/axenta-backend.service << EOF
[Unit]
Description=Axenta CRM Backend Service
After=network.target postgresql.service
Wants=postgresql.service

[Service]
Type=simple
User=axenta
Group=axenta
WorkingDirectory=/opt/axenta/backend
ExecStart=/opt/axenta/backend/axenta_backend
ExecReload=/bin/kill -HUP \$MAINPID
KillMode=mixed
KillSignal=SIGINT
TimeoutStopSec=5
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/axenta
NoNewPrivileges=true

# Перезапуск при сбое
Restart=always
RestartSec=5
StartLimitInterval=60
StartLimitBurst=3

# Логирование
StandardOutput=journal
StandardError=journal
SyslogIdentifier=axenta-backend

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    systemctl enable axenta-backend
    
    print_success "Systemd служба создана и включена"
}

# Настройка Nginx
setup_nginx() {
    print_info "Настройка Nginx..."
    
    # Запрос поддомена для API
    read -p "Введите поддомен для API (например: api.yourdomain.com): " API_DOMAIN
    
    tee /etc/nginx/sites-available/axenta-backend << EOF
# Rate limiting
limit_req_zone \$binary_remote_addr zone=api:10m rate=10r/s;
limit_req_zone \$binary_remote_addr zone=auth:10m rate=5r/s;

# Upstream backend
upstream axenta_backend {
    server 127.0.0.1:8080 max_fails=3 fail_timeout=30s;
    keepalive 32;
}

server {
    listen 80;
    server_name $API_DOMAIN;
    
    # Redirect HTTP to HTTPS (будет активировано после получения SSL)
    # return 301 https://\$server_name\$request_uri;
    
    # Временная конфигурация для HTTP (до получения SSL)
    location / {
        proxy_pass http://axenta_backend;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF
    
    # Активация конфигурации
    ln -sf /etc/nginx/sites-available/axenta-backend /etc/nginx/sites-enabled/
    
    # Удаление дефолтной конфигурации
    rm -f /etc/nginx/sites-enabled/default
    
    # Проверка конфигурации
    nginx -t
    systemctl enable nginx
    systemctl restart nginx
    
    print_success "Nginx настроен"
    echo "API_DOMAIN=$API_DOMAIN" > /tmp/api_domain
}

# Настройка файрвола
setup_firewall() {
    print_info "Настройка файрвола..."
    
    if [[ "$OS" == *"Ubuntu"* ]] || [[ "$OS" == *"Debian"* ]]; then
        ufw allow 22/tcp
        ufw allow 80/tcp
        ufw allow 443/tcp
        ufw --force enable
    elif [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
        systemctl enable firewalld
        systemctl start firewalld
        firewall-cmd --permanent --add-service=ssh
        firewall-cmd --permanent --add-service=http
        firewall-cmd --permanent --add-service=https
        firewall-cmd --reload
    fi
    
    print_success "Файрвол настроен"
}

# Настройка fail2ban
setup_fail2ban() {
    print_info "Настройка fail2ban..."
    
    systemctl enable fail2ban
    systemctl start fail2ban
    
    print_success "Fail2ban настроен"
}

# Создание скриптов обслуживания
create_maintenance_scripts() {
    print_info "Создание скриптов обслуживания..."
    
    # Скрипт резервного копирования
    tee /opt/axenta/backup.sh << 'EOF'
#!/bin/bash

# Настройки
BACKUP_DIR="/opt/axenta/backups"
DB_NAME="axenta_db"
DB_USER="axenta_user"
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="$BACKUP_DIR/axenta_db_$DATE.sql"

# Создание директории для бэкапов
mkdir -p $BACKUP_DIR

# Чтение пароля из .env файла
DB_PASSWORD=$(grep "DB_PASSWORD=" /opt/axenta/backend/.env | cut -d'=' -f2)

# Создание бэкапа
echo "Создание резервной копии БД..."
PGPASSWORD=$DB_PASSWORD pg_dump -h localhost -U $DB_USER $DB_NAME > $BACKUP_FILE

# Сжатие
gzip $BACKUP_FILE

# Удаление старых бэкапов (старше 30 дней)
find $BACKUP_DIR -name "*.sql.gz" -mtime +30 -delete

echo "Резервная копия создана: $BACKUP_FILE.gz"
EOF
    
    # Скрипт обновления
    tee /opt/axenta/update.sh << 'EOF'
#!/bin/bash

echo "Начало обновления Axenta Backend..."

# Переход в директорию приложения
cd /opt/axenta/backend

# Создание бэкапа текущей версии
sudo -u axenta cp axenta_backend axenta_backend.backup

# Получение обновлений
sudo -u axenta git fetch origin
sudo -u axenta git checkout main
sudo -u axenta git pull origin main

# Обновление зависимостей
sudo -u axenta /usr/local/go/bin/go mod download

# Сборка новой версии
sudo -u axenta /usr/local/go/bin/go build -ldflags="-w -s" -o axenta_backend.new main.go

# Остановка службы
systemctl stop axenta-backend

# Замена исполняемого файла
sudo -u axenta mv axenta_backend.new axenta_backend
sudo -u axenta chmod +x axenta_backend

# Запуск службы
systemctl start axenta-backend

# Проверка статуса
sleep 5
if systemctl is-active --quiet axenta-backend; then
    echo "✅ Обновление прошло успешно!"
    sudo -u axenta rm -f axenta_backend.backup
else
    echo "❌ Ошибка при обновлении. Откат к предыдущей версии..."
    systemctl stop axenta-backend
    sudo -u axenta mv axenta_backend.backup axenta_backend
    systemctl start axenta-backend
    exit 1
fi
EOF
    
    chmod +x /opt/axenta/backup.sh
    chmod +x /opt/axenta/update.sh
    chown axenta:axenta /opt/axenta/backup.sh
    chown axenta:axenta /opt/axenta/update.sh
    
    print_success "Скрипты обслуживания созданы"
}

# Настройка cron задач
setup_cron() {
    print_info "Настройка автоматических задач..."
    
    # Добавление задач в crontab для пользователя axenta
    sudo -u axenta crontab << 'EOF'
# Ежедневное резервное копирование в 2:00
0 2 * * * /opt/axenta/backup.sh

# Еженедельная очистка логов в 3:00 воскресенья
0 3 * * 0 find /var/log/nginx -name "*.log" -mtime +7 -delete
EOF
    
    print_success "Cron задачи настроены"
}

# Запуск служб
start_services() {
    print_info "Запуск служб..."
    
    systemctl start axenta-backend
    
    # Проверка статуса
    sleep 3
    if systemctl is-active --quiet axenta-backend; then
        print_success "Axenta Backend запущен"
    else
        print_error "Ошибка запуска Axenta Backend"
        journalctl -u axenta-backend -n 20
        exit 1
    fi
}

# Финальная проверка
final_check() {
    print_info "Выполнение финальной проверки..."
    
    # Проверка HTTP ответа
    if curl -f http://localhost:8080/health &>/dev/null; then
        print_success "API отвечает на запросы"
    else
        print_error "API не отвечает"
        exit 1
    fi
    
    # Проверка подключения к БД
    if sudo -u axenta psql -h localhost -U axenta_user -d axenta_db -c "SELECT 1;" &>/dev/null; then
        print_success "Подключение к БД работает"
    else
        print_warning "Проверьте подключение к БД"
    fi
}

# Вывод финальной информации
show_final_info() {
    API_DOMAIN=$(cat /tmp/api_domain | cut -d'=' -f2)
    rm -f /tmp/api_domain
    
    print_header
    print_success "🎉 РАЗВЕРТЫВАНИЕ ЗАВЕРШЕНО УСПЕШНО!"
    echo -e "${NC}"
    
    echo "📋 Информация о развертывании:"
    echo "   🌐 API URL: http://$API_DOMAIN"
    echo "   🗄️  База данных: PostgreSQL (axenta_db)"
    echo "   👤 Пользователь приложения: axenta"
    echo "   📁 Директория приложения: /opt/axenta/backend"
    echo ""
    
    echo "🔧 Управление службой:"
    echo "   sudo systemctl status axenta-backend"
    echo "   sudo systemctl restart axenta-backend"
    echo "   sudo journalctl -u axenta-backend -f"
    echo ""
    
    echo "📊 Обслуживание:"
    echo "   Резервное копирование: /opt/axenta/backup.sh"
    echo "   Обновление: /opt/axenta/update.sh"
    echo ""
    
    echo "🔐 Следующие шаги:"
    echo "   1. Настройте DNS для домена $API_DOMAIN"
    echo "   2. Получите SSL сертификат: sudo certbot --nginx -d $API_DOMAIN"
    echo "   3. Раскомментируйте HTTPS redirect в Nginx конфигурации"
    echo "   4. Протестируйте API endpoints"
    echo ""
    
    print_success "Документация: /opt/axenta/backend/PRODUCTION_DEPLOY.md"
    echo -e "${NC}"
}

# Основная функция
main() {
    print_header
    
    check_root
    check_os
    install_dependencies
    install_go
    create_app_user
    setup_postgresql
    clone_repository
    create_env_file
    build_application
    create_systemd_service
    setup_nginx
    setup_firewall
    setup_fail2ban
    create_maintenance_scripts
    setup_cron
    start_services
    final_check
    show_final_info
}

# Запуск скрипта
main "$@"
