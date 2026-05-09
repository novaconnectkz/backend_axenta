# Быстрое тестирование Axenta CRM

## 📚 API Документация

**OpenAPI спецификация**: `openapi.yaml` - полная документация API  
**Swagger UI**: Доступна в продакшене по адресу `/docs`

### 🔗 Основные эндпоинты API:

**Аутентификация:**
- `POST /api/auth/login/` - Вход в систему
- `GET /api/current_user/` - Текущий пользователь

**Объекты мониторинга:**
- `GET /api/objects/` - Список объектов (публичный)
- `GET /api/auth/objects/` - Список объектов (с авторизацией)
- `POST /api/auth/objects/` - Создание объекта
- `PUT /api/auth/objects/{id}/` - Обновление объекта
- `DELETE /api/auth/objects/{id}/` - Удаление объекта

**Устройства:**
- `GET /api/devices/` - Список устройств
- `POST /api/devices/` - Создание устройства
- `PUT /api/devices/{id}/` - Обновление устройства

**Сообщения и треки:**
- `GET /api/messages/get` - Получение сообщений
- `POST /api/tracks/create/` - Создание трека
- `GET /api/messages/stat` - Статистика сообщений

**Водители:**
- `GET /api/drivers/` - Список водителей
- `POST /api/drivers/` - Создание водителя
- `POST /api/driver/attach/` - Привязка водителя к объекту

**Геозоны:**
- `GET /api/geozone/` - Список геозон
- `POST /api/geozone/` - Создание геозоны
- `PUT /api/geozone/{id}/` - Обновление геозоны

**Уведомления:**
- `GET /api/notifications/` - Список уведомлений
- `POST /api/notifications/` - Создание уведомления

## 🔐 Основные данные

**Продакшен:**
- SSH: `ssh -i id_rsa.key root@194.87.143.169`
- API: `https://api.acrm.su`
- Frontend: `https://acrm.su`

**Локальный:**
- API: `http://localhost:8080`
- Frontend: `http://localhost:3001`

**Учетные данные:**
- Пользователь: `glomos`
- Пароль: `A51ewweB`
- Токен: `5e515a8f2874fc78f31c74af45260333f2c84c35`

## 🧪 Быстрый тест

### 1. Проверить статус сервера
```bash
ssh -i id_rsa.key root@194.87.143.169 "cd /var/www/app/backend_axenta && ps aux | grep 'go run main.go' | grep -v grep"
```

### 2. Тест авторизации
```bash
curl -X POST https://api.acrm.su/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"glomos","password":"A51ewweB"}'
```

### 3. Тест создания пользователя
```bash
curl -X POST https://api.acrm.su/api/cms/users/ \
  -H "Content-Type: application/json" \
  -H "Authorization: Token 5e515a8f2874fc78f31c74af45260333f2c84c35" \
  -d '{
    "name": "Test User",
    "username": "test_user_'$(date +%s)'",
    "email": "test_user_'$(date +%s)'@example.com",
    "password": "password123",
    "hasAdminAccess": false,
    "visibleTabsNames": ["monitoring"],
    "accesses": {"common": ["view"]}
  }'
```

### 4. Тест получения объектов
```bash
curl -X GET https://api.acrm.su/api/auth/objects/ \
  -H "Authorization: Token 5e515a8f2874fc78f31c74af45260333f2c84c35"
```

### 5. Тест получения устройств
```bash
curl -X GET https://api.acrm.su/api/devices/ \
  -H "Authorization: Token 5e515a8f2874fc78f31c74af45260333f2c84c35"
```

### 6. Тест получения водителей
```bash
curl -X GET https://api.acrm.su/api/drivers/ \
  -H "Authorization: Token 5e515a8f2874fc78f31c74af45260333f2c84c35"
```

### 7. Проверить результат
```bash
ssh -i id_rsa.key root@194.87.143.169 "sudo -u postgres psql -d axenta_db -c \"SET search_path TO tenant_default; SELECT id, name, username, email FROM users ORDER BY created_at DESC LIMIT 3;\""
```

## 🚨 Быстрые исправления

### Очистить токены
```bash
ssh -i id_rsa.key root@194.87.143.169 "sudo -u postgres psql -d axenta_db -c \"DELETE FROM user_tokens; ALTER SEQUENCE user_tokens_id_seq RESTART WITH 1;\""
```

### Перезапустить сервер
```bash
ssh -i id_rsa.key root@194.87.143.169 "cd /var/www/app/backend_axenta && pkill -f 'go run main.go' && sleep 2 && nohup go run main.go > server.log 2>&1 &"
```

### Проверить логи
```bash
ssh -i id_rsa.key root@194.87.143.169 "cd /var/www/app/backend_axenta && tail -20 server.log"
```

## 🏠 Локальное тестирование

### 1. Авторизация на локальном сервере
```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"glomos","password":"A51ewweB"}'
```

### 2. Получение объектов (локально)
```bash
curl -X GET http://localhost:8080/api/auth/objects/ \
  -H "Authorization: Token 5e515a8f2874fc78f31c74af45260333f2c84c35"
```

### 3. Получение устройств (локально)
```bash
curl -X GET http://localhost:8080/api/devices/ \
  -H "Authorization: Token 5e515a8f2874fc78f31c74af45260333f2c84c35"
```

### 4. Получение водителей (локально)
```bash
curl -X GET http://localhost:8080/api/drivers/ \
  -H "Authorization: Token 5e515a8f2874fc78f31c74af45260333f2c84c35"
```

### 5. Проверка здоровья API (локально)
```bash
curl -X GET http://localhost:8080/health
```

### 📝 Примечания по API:

- **Рабочие эндпоинты**: `/api/objects/`, `/api/auth/objects/` - возвращают данные объектов
- **Эндпоинты в разработке**: `/api/devices/`, `/api/drivers/` - могут возвращать 404
- **Авторизация**: Большинство эндпоинтов требуют заголовок `Authorization: Token <токен>`
- **Формат ответа**: JSON с полями `data`, `status`, `page`, `per_page`, `total`

### 🔍 Полезные команды для отладки:

```bash
# Проверить все доступные эндпоинты
curl -X GET http://localhost:8080/api/objects/ | jq .

# Получить только первый объект
curl -X GET http://localhost:8080/api/auth/objects/?per_page=1 \
  -H "Authorization: Token 5e515a8f2874fc78f31c74af45260333f2c84c35" | jq .
```
