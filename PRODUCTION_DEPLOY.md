# 🚀 Инструкция по развертыванию бэкенда AxentaCRM в продакшене

## 📋 Содержание
- [Системные требования](#системные-требования)
- [Предварительная подготовка](#предварительная-подготовка)
- [Установка и настройка](#установка-и-настройка)
- [Переменные окружения](#переменные-окружения)
- [Сборка приложения](#сборка-приложения)
- [Настройка службы systemd](#настройка-службы-systemd)
- [Настройка Nginx](#настройка-nginx)
- [SSL сертификат](#ssl-сертификат)
- [Мониторинг и логи](#мониторинг-и-логи)
- [Резервное копирование](#резервное-копирование)
- [Обновление](#обновление)

## 🔧 Системные требования

### Минимальные требования:
- **OS:** Ubuntu 20.04+ / CentOS 8+ / Debian 11+
- **RAM:** 2GB (рекомендуется 4GB+)
- **CPU:** 2 ядра (рекомендуется 4+)
- **Диск:** 20GB свободного места
- **Go:** версия 1.19+
- **PostgreSQL:** версия 13+

### Рекомендуемые требования:
- **RAM:** 8GB+
- **CPU:** 4+ ядра
- **Диск:** SSD 50GB+
- **Сеть:** стабильное подключение к интернету

## 🛠️ Предварительная подготовка

### 1. Обновление системы
```bash
# Ubuntu/Debian
sudo apt update && sudo apt upgrade -y

# CentOS/RHEL
sudo yum update -y
```

### 2. Установка зависимостей
```bash
# Ubuntu/Debian
sudo apt install -y git curl wget nginx postgresql postgresql-contrib supervisor

# CentOS/RHEL
sudo yum install -y git curl wget nginx postgresql postgresql-server postgresql-contrib supervisor
```

### 3. Установка Go
```bash
# Скачивание и установка Go
cd /tmp
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz

# Добавление в PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Проверка установки
go version
```

## ⚙️ Установка и настройка

### 1. Создание пользователя для приложения
```bash
sudo useradd -m -s /bin/bash axenta
sudo mkdir -p /opt/axenta
sudo chown axenta:axenta /opt/axenta
```

### 2. Клонирование репозитория
```bash
sudo -u axenta git clone https://github.com/novaconnectkz/backend_axenta.git /opt/axenta/backend
cd /opt/axenta/backend
sudo -u axenta git checkout main
```

### 3. Настройка PostgreSQL
```bash
# Запуск PostgreSQL
sudo systemctl start postgresql
sudo systemctl enable postgresql

# Создание базы данных и пользователя
sudo -u postgres psql << EOF
CREATE DATABASE axenta_db;
CREATE USER axenta_user WITH PASSWORD 'secure_password_here';
GRANT ALL PRIVILEGES ON DATABASE axenta_db TO axenta_user;
ALTER USER axenta_user CREATEDB;
\q
EOF
```

## 🔒 Переменные окружения

### 1. Создание файла .env
```bash
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
DB_PASSWORD=secure_password_here
DB_SSLMODE=require

# JWT настройки
JWT_SECRET=your_super_secret_jwt_key_here_minimum_32_characters
JWT_EXPIRES_IN=24h

# Axenta Cloud API
AXENTA_API_URL=https://axenta.cloud/api
AXENTA_TIMEOUT=30s

# Логирование
LOG_LEVEL=info
LOG_FORMAT=json

# CORS настройки
CORS_ALLOWED_ORIGINS=https://yourdomain.com,https://www.yourdomain.com
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
```

### 2. Настройка прав доступа
```bash
sudo chmod 600 /opt/axenta/backend/.env
sudo chown axenta:axenta /opt/axenta/backend/.env
```

## 🏗️ Сборка приложения

### 1. Установка зависимостей
```bash
cd /opt/axenta/backend
sudo -u axenta go mod download
sudo -u axenta go mod verify
```

### 2. Сборка бинарного файла
```bash
sudo -u axenta go build -ldflags="-w -s" -o axenta_backend main.go
sudo chmod +x axenta_backend
```

### 3. Тестирование сборки
```bash
sudo -u axenta ./axenta_backend --version
```

## 🔄 Настройка службы systemd

### 1. Создание службы
```bash
sudo tee /etc/systemd/system/axenta-backend.service << EOF
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
```

### 2. Запуск службы
```bash
sudo systemctl daemon-reload
sudo systemctl enable axenta-backend
sudo systemctl start axenta-backend
sudo systemctl status axenta-backend
```

## 🌐 Настройка Nginx

### 1. Создание конфигурации
```bash
sudo tee /etc/nginx/sites-available/axenta-backend << EOF
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
    server_name api.yourdomain.com;
    
    # Redirect HTTP to HTTPS
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name api.yourdomain.com;
    
    # SSL Configuration
    ssl_certificate /etc/letsencrypt/live/api.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/api.yourdomain.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-RSA-AES128-GCM-SHA256:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;
    
    # Security Headers
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header X-XSS-Protection "1; mode=block";
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin";
    
    # Logging
    access_log /var/log/nginx/axenta-api.access.log;
    error_log /var/log/nginx/axenta-api.error.log;
    
    # General settings
    client_max_body_size 10M;
    client_body_timeout 60s;
    client_header_timeout 60s;
    keepalive_timeout 65s;
    
    # API endpoints
    location /api/ {
        limit_req zone=api burst=20 nodelay;
        
        proxy_pass http://axenta_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
        proxy_connect_timeout 30s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }
    
    # Auth endpoints with stricter limits
    location /api/auth/ {
        limit_req zone=auth burst=10 nodelay;
        
        proxy_pass http://axenta_backend;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_connect_timeout 10s;
        proxy_send_timeout 10s;
        proxy_read_timeout 10s;
    }
    
    # Health check
    location /health {
        proxy_pass http://axenta_backend;
        access_log off;
    }
    
    # Block unwanted requests
    location = /favicon.ico {
        log_not_found off;
        access_log off;
        return 204;
    }
    
    location = /robots.txt {
        log_not_found off;
        access_log off;
        return 204;
    }
}
EOF
```

### 2. Активация конфигурации
```bash
sudo ln -s /etc/nginx/sites-available/axenta-backend /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

## 🔐 SSL сертификат

### 1. Установка Certbot
```bash
# Ubuntu/Debian
sudo apt install -y certbot python3-certbot-nginx

# CentOS/RHEL
sudo yum install -y certbot python3-certbot-nginx
```

### 2. Получение сертификата
```bash
sudo certbot --nginx -d api.yourdomain.com
```

### 3. Автообновление сертификата
```bash
sudo crontab -e
# Добавить строку:
0 12 * * * /usr/bin/certbot renew --quiet
```

## 📊 Мониторинг и логи

### 1. Настройка логротации
```bash
sudo tee /etc/logrotate.d/axenta-backend << EOF
/var/log/nginx/axenta-*.log {
    daily
    missingok
    rotate 52
    compress
    delaycompress
    notifempty
    create 0644 www-data www-data
    postrotate
        systemctl reload nginx
    endscript
}
EOF
```

### 2. Мониторинг с помощью systemd
```bash
# Проверка статуса
sudo systemctl status axenta-backend

# Просмотр логов
sudo journalctl -u axenta-backend -f

# Просмотр логов за последний час
sudo journalctl -u axenta-backend --since "1 hour ago"
```

### 3. Основные команды мониторинга
```bash
# Проверка работоспособности
curl -f http://localhost:8080/health || echo "Service is down"

# Проверка использования ресурсов
ps aux | grep axenta_backend
netstat -tlnp | grep :8080

# Проверка подключений к БД
sudo -u postgres psql -c "SELECT count(*) FROM pg_stat_activity WHERE datname='axenta_db';"
```

## 💾 Резервное копирование

### 1. Скрипт резервного копирования БД
```bash
sudo tee /opt/axenta/backup.sh << EOF
#!/bin/bash

# Настройки
BACKUP_DIR="/opt/axenta/backups"
DB_NAME="axenta_db"
DB_USER="axenta_user"
DATE=\$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="\$BACKUP_DIR/axenta_db_\$DATE.sql"

# Создание директории для бэкапов
mkdir -p \$BACKUP_DIR

# Создание бэкапа
echo "Создание резервной копии БД..."
PGPASSWORD=secure_password_here pg_dump -h localhost -U \$DB_USER \$DB_NAME > \$BACKUP_FILE

# Сжатие
gzip \$BACKUP_FILE

# Удаление старых бэкапов (старше 30 дней)
find \$BACKUP_DIR -name "*.sql.gz" -mtime +30 -delete

echo "Резервная копия создана: \$BACKUP_FILE.gz"
EOF

sudo chmod +x /opt/axenta/backup.sh
sudo chown axenta:axenta /opt/axenta/backup.sh
```

### 2. Автоматическое резервное копирование
```bash
sudo -u axenta crontab -e
# Добавить строку для ежедневного бэкапа в 2:00
0 2 * * * /opt/axenta/backup.sh
```

## 🔄 Обновление

### 1. Скрипт обновления
```bash
sudo tee /opt/axenta/update.sh << EOF
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
sudo -u axenta go mod download

# Сборка новой версии
sudo -u axenta go build -ldflags="-w -s" -o axenta_backend.new main.go

# Остановка службы
sudo systemctl stop axenta-backend

# Замена исполняемого файла
sudo -u axenta mv axenta_backend.new axenta_backend
sudo -u axenta chmod +x axenta_backend

# Запуск службы
sudo systemctl start axenta-backend

# Проверка статуса
sleep 5
if sudo systemctl is-active --quiet axenta-backend; then
    echo "✅ Обновление прошло успешно!"
    sudo -u axenta rm -f axenta_backend.backup
else
    echo "❌ Ошибка при обновлении. Откат к предыдущей версии..."
    sudo systemctl stop axenta-backend
    sudo -u axenta mv axenta_backend.backup axenta_backend
    sudo systemctl start axenta-backend
    exit 1
fi
EOF

sudo chmod +x /opt/axenta/update.sh
```

### 2. Использование скрипта обновления
```bash
sudo /opt/axenta/update.sh
```

## 🛡️ Безопасность

### 1. Настройка файрвола
```bash
# UFW (Ubuntu)
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable

# firewalld (CentOS)
sudo firewall-cmd --permanent --add-service=ssh
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --reload
```

### 2. Дополнительные меры безопасности
```bash
# Отключение root доступа по SSH
sudo sed -i 's/#PermitRootLogin yes/PermitRootLogin no/' /etc/ssh/sshd_config
sudo systemctl restart sshd

# Настройка fail2ban
sudo apt install fail2ban -y
sudo systemctl enable fail2ban
sudo systemctl start fail2ban
```

## 📞 Поддержка и диагностика

### Основные команды диагностики:
```bash
# Статус службы
sudo systemctl status axenta-backend

# Логи приложения
sudo journalctl -u axenta-backend -n 100

# Проверка портов
sudo netstat -tlnp | grep axenta

# Проверка процессов
ps aux | grep axenta

# Тест API
curl -I https://api.yourdomain.com/health
```

### Контакты для поддержки:
- **Email:** support@profmonitor.com
- **GitHub:** https://github.com/novaconnectkz/backend_axenta
- **Документация:** [AUTH_README.md](./AUTH_README.md)

---

**🎯 Развертывание завершено! Ваш Axenta Backend готов к работе в продакшене.**
