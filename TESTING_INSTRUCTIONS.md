# Инструкция по тестированию Axenta CRM

## 🔐 Доступы и учетные данные

### Продакшен сервер
- **IP адрес**: `194.87.143.169`
- **SSH ключ**: `id_rsa.key` (файл в корне проекта)
- **Пользователь**: `root`
- **База данных**: PostgreSQL
  - Хост: `localhost:5432`
  - База: `axenta_db`
  - Пользователь: `axenta_user`
  - Пароль: `your_secure_database_password_here`

### Локальный сервер
- **Хост**: `localhost:8080`
- **База данных**: PostgreSQL
  - Хост: `localhost:5432`
  - База: `axenta_db`
  - Пользователь: `postgres`
  - Пароль: `postgres`

### Учетные данные для авторизации
- **Пользователь**: `glomos`
- **Пароль**: `A51ewweB`
- **Ожидаемый токен**: `5e515a8f2874fc78f31c74af45260333f2c84c35`

## 🌐 Ссылки для тестирования

### Продакшен
- **Frontend**: `https://axenta.glonass-saratov.ru`
- **API**: `https://api.axenta.glonass-saratov.ru`

### Локальный
- **Frontend**: `http://localhost:3000`
- **API**: `http://localhost:8080`

## 🧪 Тестовые данные для создания пользователей

### Успешное создание пользователя
```json
{
  "name": "Test User Production",
  "username": "test_user_$(date +%s)",
  "email": "test_user_$(date +%s)@example.com",
  "password": "password123",
  "hasAdminAccess": false,
  "visibleTabsNames": ["monitoring"],
  "accesses": {
    "common": ["view"]
  }
}
```

### Пользователь с админ правами
```json
{
  "name": "Admin Test User",
  "username": "admin_test_$(date +%s)",
  "email": "admin_test_$(date +%s)@example.com",
  "password": "password123",
  "hasAdminAccess": true,
  "visibleTabsNames": ["monitoring", "users", "settings"],
  "accesses": {
    "common": ["view", "create", "edit", "delete"],
    "users": ["view", "create", "edit"],
    "settings": ["view", "edit"]
  }
}
```

## 🔧 Команды для тестирования

### 1. Подключение к продакшен серверу
```bash
ssh -i id_rsa.key -o StrictHostKeyChecking=no root@194.87.143.169
```

### 2. Проверка статуса сервера
```bash
# На продакшене
cd /var/www/app/backend_axenta
ps aux | grep "go run main.go" | grep -v grep
tail -20 server.log

# На локальном
ps aux | grep "go run main.go" | grep -v grep
```

### 3. Перезапуск сервера
```bash
# На продакшене
cd /var/www/app/backend_axenta
pkill -f "go run main.go"
sleep 2
nohup go run main.go > server.log 2>&1 &

# На локальном
pkill -f "go run main.go"
sleep 2
go run main.go
```

### 4. Проверка базы данных
```bash
# Продакшен
sudo -u postgres psql -d axenta_db -c "SELECT COUNT(*) FROM user_tokens WHERE is_active = true;"

# Локальный
PGPASSWORD=postgres psql -h localhost -p 5432 -U postgres -d axenta_db -c "SELECT COUNT(*) FROM user_tokens WHERE is_active = true;"
```

## 🧪 Тестирование API

### 1. Тест авторизации
```bash
# Продакшен
curl -X POST https://api.axenta.glonass-saratov.ru/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "glomos",
    "password": "A51ewweB"
  }'

# Локальный
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "glomos",
    "password": "A51ewweB"
  }'
```

### 2. Тест создания пользователя
```bash
# Продакшен
curl -X POST https://api.axenta.glonass-saratov.ru/api/cms/users/ \
  -H "Content-Type: application/json" \
  -H "Authorization: Token 5e515a8f2874fc78f31c74af45260333f2c84c35" \
  -d '{
    "name": "Test User $(date +%s)",
    "username": "test_user_$(date +%s)",
    "email": "test_user_$(date +%s)@example.com",
    "password": "password123",
    "hasAdminAccess": false,
    "visibleTabsNames": ["monitoring"],
    "accesses": {
      "common": ["view"]
    }
  }'

# Локальный
curl -X POST http://localhost:8080/api/cms/users/ \
  -H "Content-Type: application/json" \
  -H "Authorization: Token 5e515a8f2874fc78f31c74af45260333f2c84c35" \
  -d '{
    "name": "Test User $(date +%s)",
    "username": "test_user_$(date +%s)",
    "email": "test_user_$(date +%s)@example.com",
    "password": "password123",
    "hasAdminAccess": false,
    "visibleTabsNames": ["monitoring"],
    "accesses": {
      "common": ["view"]
    }
  }'
```

### 3. Проверка созданного пользователя
```bash
# Продакшен
sudo -u postgres psql -d axenta_db -c "SET search_path TO tenant_default; SELECT id, name, username, email, is_active FROM users ORDER BY created_at DESC LIMIT 5;"

# Локальный
PGPASSWORD=postgres psql -h localhost -p 5432 -U postgres -d axenta_db -c "SET search_path TO tenant_default; SELECT id, name, username, email, is_active FROM users ORDER BY created_at DESC LIMIT 5;"
```

## 🗂️ Структура базы данных

### Основные таблицы
- `users` - пользователи
- `user_tokens` - токены пользователей
- `user_tabs` - видимые вкладки пользователей
- `user_accesses` - права доступа пользователей

### Схемы
- `public` - глобальные таблицы
- `tenant_default` - тенантные таблицы

## 🚨 Типичные проблемы и решения

### 1. Ошибка 401 Unauthorized
```bash
# Проверить токены в базе
sudo -u postgres psql -d axenta_db -c "SELECT id, user_id, username, is_active FROM user_tokens WHERE is_active = true;"

# Очистить токены при необходимости
sudo -u postgres psql -d axenta_db -c "DELETE FROM user_tokens; ALTER SEQUENCE user_tokens_id_seq RESTART WITH 1;"
```

### 2. Ошибка "relation does not exist"
```bash
# Проверить существование таблиц
sudo -u postgres psql -d axenta_db -c "SELECT schemaname, tablename FROM pg_tables WHERE tablename IN ('user_tokens', 'user_tabs', 'user_accesses');"

# Создать недостающие таблицы
sudo -u postgres psql -d axenta_db -c "CREATE TABLE IF NOT EXISTS public.user_tokens (...);"
```

### 3. Ошибка "malformed array literal"
```bash
# Проверить структуру таблицы user_accesses
sudo -u postgres psql -d axenta_db -c "SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'user_accesses' AND column_name = 'perms';"

# Исправить тип поля
sudo -u postgres psql -d axenta_db -c "ALTER TABLE user_accesses ALTER COLUMN perms TYPE TEXT;"
```

## 📋 Чек-лист для тестирования

- [ ] Сервер запущен и отвечает на ping
- [ ] База данных доступна
- [ ] Авторизация работает
- [ ] Токен сохраняется в user_tokens
- [ ] Создание пользователя работает
- [ ] Пользователь сохраняется в базе
- [ ] user_tabs создаются корректно
- [ ] user_accesses создаются корректно
- [ ] Frontend может создать пользователя через форму

## 🔄 Команды для быстрого тестирования

### Полный цикл тестирования
```bash
# 1. Проверить статус
ssh -i id_rsa.key root@194.87.143.169 "cd /var/www/app/backend_axenta && ps aux | grep 'go run main.go' | grep -v grep"

# 2. Авторизация
curl -X POST https://api.axenta.glonass-saratov.ru/api/auth/login -H "Content-Type: application/json" -d '{"username":"glomos","password":"A51ewweB"}'

# 3. Создание пользователя
curl -X POST https://api.axenta.glonass-saratov.ru/api/cms/users/ -H "Content-Type: application/json" -H "Authorization: Token 5e515a8f2874fc78f31c74af45260333f2c84c35" -d '{"name":"Test User","username":"test_user_'$(date +%s)'","email":"test_user_'$(date +%s)'@example.com","password":"password123","hasAdminAccess":false,"visibleTabsNames":["monitoring"],"accesses":{"common":["view"]}}'

# 4. Проверка результата
ssh -i id_rsa.key root@194.87.143.169 "sudo -u postgres psql -d axenta_db -c \"SET search_path TO tenant_default; SELECT id, name, username, email FROM users ORDER BY created_at DESC LIMIT 3;\""
```

---
*Документ создан: $(date)*
*Последнее обновление: $(date)*
