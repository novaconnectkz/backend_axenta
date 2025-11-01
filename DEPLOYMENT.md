# 🚀 Процесс развертывания Axenta Backend

## Инфраструктура

Проект использует классический подход без контейнеризации:

### Компоненты

1. **Systemd** (`axenta-backend.service`)
   - Управление процессом приложения
   - Автозапуск при загрузке системы
   - Автоматический перезапуск при сбоях
   - Логирование в journald

2. **Nginx** (reverse proxy)
   - Проксирование запросов на `127.0.0.1:8080`
   - Rate limiting
   - SSL/TLS терминация
   - Доступ из интернета через порты 80/443

3. **PostgreSQL**
   - Установлен на том же сервере
   - База данных: `axenta_db`
   - Пользователь: `axenta_user`
   - Локальное подключение через Unix socket или localhost:5432

4. **Go Binary**
   - Собирается на сервере: `go build -ldflags="-w -s" -o axenta_backend main.go`
   - Расположение: `/opt/axenta/backend/axenta_backend`
   - Запускается от пользователя `axenta`

5. **Git**
   - Клонирование репозитория: `/opt/axenta/backend`
   - Обновление через `git pull`

### Процесс деплоя

1. **Автоматический** (через GitHub Actions)
   - Push в `main` → запускается workflow `.github/workflows/main.yml`
   - SSH подключение к серверу
   - `git pull` последних изменений
   - `go build` нового бинарника
   - `systemctl restart axenta-backend`

2. **Вручную** (через скрипт)
   ```bash
   ./deploy_production.sh
   ```
   Или через SSH:
   ```bash
   cd /opt/axenta/backend
   git pull
   go build -ldflags="-w -s" -o axenta_backend main.go
   sudo systemctl restart axenta-backend
   ```

### Управление сервисом

```bash
# Статус
sudo systemctl status axenta-backend

# Перезапуск
sudo systemctl restart axenta-backend

# Логи
sudo journalctl -u axenta-backend -f

# Остановка
sudo systemctl stop axenta-backend

# Запуск
sudo systemctl start axenta-backend
```

### Скрипты обслуживания

- `/opt/axenta/backup.sh` - резервное копирование БД (автоматически через cron в 2:00)
- `/opt/axenta/update.sh` - обновление приложения с откатом при ошибке

### Безопасность

- **UFW/FirewallD** - файрвол (открыты только 22, 80, 443)
- **Fail2ban** - защита от брутфорса
- **Nginx rate limiting** - ограничение частоты запросов
- **Systemd security** - изоляция процесса (ProtectSystem, NoNewPrivileges)

### Мониторинг

- Логи: `journalctl -u axenta-backend`
- Health check: `curl http://localhost:8080/ping`
- Метрики: `/metrics` endpoint (если настроен)

---

**Примечание**: Docker файлы (Dockerfile, docker-compose.yml) присутствуют только для экспериментальных целей и не используются в продакшене.

