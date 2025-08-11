# ⚡ Быстрый старт для продакшен развертывания Axenta Backend

## 🚀 Автоматическое развертывание (Рекомендуется)

### 1. Скачайте скрипт развертывания
```bash
wget https://raw.githubusercontent.com/novaconnectkz/backend_axenta/main/deploy_production.sh
chmod +x deploy_production.sh
```

### 2. Запустите автоматическое развертывание
```bash
sudo ./deploy_production.sh
```

Скрипт автоматически:
- ✅ Установит все зависимости
- ✅ Настроит PostgreSQL
- ✅ Создаст пользователя приложения
- ✅ Соберет приложение
- ✅ Настроит systemd службу
- ✅ Настроит Nginx
- ✅ Настроит файрвол и безопасность
- ✅ Создаст скрипты обслуживания

**Время развертывания: 5-10 минут** ⏱️

---

## 🛠️ Ручное развертывание

### Требования
- Ubuntu 20.04+ / CentOS 8+ / Debian 11+
- 2GB+ RAM, 2+ CPU ядра
- Root доступ

### 1. Установка зависимостей
```bash
# Ubuntu/Debian
sudo apt update && sudo apt install -y git curl wget nginx postgresql postgresql-contrib

# CentOS/RHEL  
sudo yum update -y && sudo yum install -y git curl wget nginx postgresql postgresql-server
```

### 2. Установка Go
```bash
cd /tmp
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### 3. Создание пользователя и клонирование
```bash
sudo useradd -m -s /bin/bash axenta
sudo mkdir -p /opt/axenta
sudo chown axenta:axenta /opt/axenta
sudo -u axenta git clone https://github.com/novaconnectkz/backend_axenta.git /opt/axenta/backend
```

### 4. Настройка PostgreSQL
```bash
sudo systemctl start postgresql
sudo systemctl enable postgresql

sudo -u postgres psql << EOF
CREATE DATABASE axenta_db;
CREATE USER axenta_user WITH PASSWORD 'your_password_here';
GRANT ALL PRIVILEGES ON DATABASE axenta_db TO axenta_user;
\q
EOF
```

### 5. Создание .env файла
```bash
sudo -u axenta cp /opt/axenta/backend/env.production.example /opt/axenta/backend/.env
sudo -u axenta nano /opt/axenta/backend/.env
# Измените DB_PASSWORD и JWT_SECRET!
```

### 6. Сборка и запуск
```bash
cd /opt/axenta/backend
sudo -u axenta go build -o axenta_backend main.go

# Создание systemd службы
sudo tee /etc/systemd/system/axenta-backend.service << EOF
[Unit]
Description=Axenta CRM Backend
After=network.target postgresql.service

[Service]
Type=simple
User=axenta
Group=axenta
WorkingDirectory=/opt/axenta/backend
ExecStart=/opt/axenta/backend/axenta_backend
Restart=always

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable axenta-backend
sudo systemctl start axenta-backend
```

---

## 🔧 После установки

### Проверка работоспособности
```bash
# Статус службы
sudo systemctl status axenta-backend

# Проверка API
curl http://localhost:8080/health

# Просмотр логов
sudo journalctl -u axenta-backend -f
```

### Настройка SSL (Let's Encrypt)
```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d api.yourdomain.com
```

### Основные команды управления
```bash
# Перезапуск службы
sudo systemctl restart axenta-backend

# Обновление приложения
sudo /opt/axenta/update.sh

# Резервное копирование
sudo /opt/axenta/backup.sh

# Просмотр конфигурации
cat /opt/axenta/backend/.env
```

---

## 🔗 Полезные ссылки

- 📖 **Полная документация**: [PRODUCTION_DEPLOY.md](./PRODUCTION_DEPLOY.md)
- 🔐 **Настройка авторизации**: [AUTH_README.md](./AUTH_README.md)
- 🌐 **GitHub репозиторий**: https://github.com/novaconnectkz/backend_axenta
- 💬 **Поддержка**: support@profmonitor.com

---

## 🆘 Решение проблем

### Служба не запускается
```bash
sudo journalctl -u axenta-backend -n 50
sudo systemctl status axenta-backend
```

### Ошибки подключения к БД
```bash
sudo -u postgres psql -c "\l"  # Список БД
sudo systemctl status postgresql
```

### Проблемы с портами
```bash
sudo netstat -tlnp | grep :8080
sudo lsof -i :8080
```

### API не отвечает
```bash
curl -v http://localhost:8080/health
sudo nginx -t  # Проверка конфигурации Nginx
```

---

**🎯 Готово! Ваш Axenta Backend работает в продакшене!**

Следующий шаг: настройте фронтенд для работы с вашим API endpoint.
