# Инструкция по добавлению DaData API ключа на продакшен сервер

## DaData API ключ
**API-ключ (Token):** `9f89eacbff6b5a22581f4f3d36103470a5fede82`

## Способы добавления ключа на сервер

### Способ 1: Через файл DADATA_API_KEY (рекомендуется)

1. Подключитесь к продакшен серверу:
```bash
ssh user@your-production-server
```

2. Перейдите в директорию бэкенда:
```bash
cd /path/to/backend_axenta
```

3. Откройте или создайте файл `.env`:
```bash
nano .env
# или
vi .env
```

4. Добавьте переменную окружения:
```bash
DADATA_API_KEY=9f89eacbff6b5a22581f4f3d36103470a5fede82
```

5. Сохраните файл (Ctrl+O, затем Enter в nano, или :wq в vi)

6. Перезапустите бэкенд сервис:
```bash
# Если используется systemd
sudo systemctl restart backend-axenta

# Или если запускается через PM2
pm2 restart backend-axenta

# Или если запускается напрямую
# Остановите текущий процесс и запустите заново
```

### Способ 2: Через переменные окружения системы

1. Добавьте в `/etc/environment` или в файл конфигурации systemd service:
```bash
DADATA_API_KEY=9f89eacbff6b5a22581f4f3d36103470a5fede82
```

2. Если используете systemd, отредактируйте service файл:
```bash
sudo nano /etc/systemd/system/backend-axenta.service
```

3. Добавьте в секцию `[Service]`:
```ini
[Service]
Environment="DADATA_API_KEY=9f89eacbff6b5a22581f4f3d36103470a5fede82"
```

4. Перезагрузите конфигурацию и сервис:
```bash
sudo systemctl daemon-reload
sudo systemctl restart backend-axenta
```

### Способ 3: Через Docker (если используется)

1. Добавьте в `docker-compose.yml`:
```yaml
services:
  backend:
    environment:
      - DADATA_API_KEY=9f89eacbff6b5a22581f4f3d36103470a5fede82
```

2. Перезапустите контейнер:
```bash
docker-compose restart backend
```

## Проверка работы

После добавления ключа проверьте логи бэкенда:

```bash
# Если используется systemd
sudo journalctl -u backend-axenta -f

# Или если логи в файле
tail -f /path/to/backend_axenta/backend.log
```

Проверьте, что при запросе к `/api/auth/dadata/organization` не возникает ошибки:
```
"DaData API ключ не настроен на сервере"
```

## Тестирование

Можно протестировать через curl:

```bash
curl -X POST http://localhost:8080/api/auth/dadata/organization \
  -H "Content-Type: application/json" \
  -H "Authorization: Token YOUR_TOKEN" \
  -d '{"query": "6455051190", "branch_type": "MAIN"}'
```

Если всё работает, вы получите данные организации в ответе.

## Примечание

- Ключ хранится на бэкенде, фронтенд не имеет к нему доступа
- Ключ используется для всех пользователей через бэкенд API
- Безопасность: не коммитьте .env файл в Git
