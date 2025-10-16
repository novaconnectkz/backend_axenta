# Быстрое тестирование Axenta CRM

## 🔐 Основные данные

**Продакшен:**
- SSH: `ssh -i id_rsa.key root@194.87.143.169`
- API: `https://api.axenta.glonass-saratov.ru`
- Frontend: `https://axenta.glonass-saratov.ru`

**Локальный:**
- API: `http://localhost:8080`
- Frontend: `http://localhost:3000`

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
curl -X POST https://api.axenta.glonass-saratov.ru/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"glomos","password":"A51ewweB"}'
```

### 3. Тест создания пользователя
```bash
curl -X POST https://api.axenta.glonass-saratov.ru/api/cms/users/ \
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

### 4. Проверить результат
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
