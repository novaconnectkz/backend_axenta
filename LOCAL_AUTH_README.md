# Локальная авторизация Axenta CRM

## 🎯 Обзор

Система локальной авторизации предоставляет альтернативный способ аутентификации пользователей в Axenta CRM, работающий параллельно с существующей интеграцией Axenta Cloud API.

## 🏗️ Архитектура

### Backend (Go + Gin)

```
backend_axenta/
├── models/
│   └── local_user.go          # Модели локальных пользователей
├── services/
│   └── jwt_service.go         # JWT сервис
├── middleware/
│   └── local_auth.go          # Middleware для локальной авторизации
├── api/
│   ├── local_auth.go          # API endpoints
│   └── websocket_auth.go      # WebSocket с авторизацией
├── cmd/server/
│   └── main_local_auth.go     # Альтернативный main.go
└── scripts/
    └── create_test_users.go   # Создание тестовых пользователей
```

### Frontend (Vue 3 + TypeScript)

```
frontend_axenta/src/
├── composables/
│   └── useLocalAuth.js        # Composable для локальной авторизации
├── views/
│   ├── LocalLogin.vue         # Страница входа
│   └── LocalProfile.vue       # Профиль пользователя
└── router/
    └── index.ts               # Обновленные маршруты
```

## 🔐 Безопасность

### JWT Токены
- **Access Token**: Срок жизни 1 час (настраивается через `JWT_ACCESS_TTL`)
- **Refresh Token**: Срок жизни 7 дней (настраивается через `JWT_REFRESH_TTL`)
- **Алгоритм**: HS256
- **Секрет**: Настраивается через `JWT_SECRET`

### Роли пользователей
- `admin` - Полный доступ ко всем функциям
- `manager` - Управление объектами и пользователями
- `tech` - Технические операции
- `accountant` - Просмотр отчетов и финансов
- `user` - Базовый доступ

### Хеширование паролей
- Использует `bcrypt` с cost factor 12
- Пароли никогда не хранятся в открытом виде

## 🚀 Быстрый старт

### 1. Настройка Backend

#### Переменные окружения
```bash
# .env
JWT_SECRET=your-super-secret-key-change-in-production
JWT_ACCESS_TTL=1    # часы
JWT_REFRESH_TTL=168 # часы (7 дней)
```

#### Запуск с локальной авторизацией
```bash
# Переименуйте файл для использования
mv cmd/server/main_local_auth.go main.go

# Установите зависимости
go mod tidy

# Создайте тестовых пользователей
go run scripts/create_test_users.go

# Запустите сервер
go run main.go
```

### 2. Настройка Frontend

#### Установка зависимостей
```bash
npm install
```

#### Использование в компонентах
```vue
<script setup>
import { useLocalAuth } from '@/composables/useLocalAuth'

const localAuth = useLocalAuth()

const handleLogin = async () => {
  try {
    await localAuth.login('admin', 'admin123')
    console.log('Logged in:', localAuth.user.value)
  } catch (error) {
    console.error('Login failed:', error)
  }
}
</script>
```

## 📡 API Endpoints

### Публичные endpoints

#### POST `/api/local/login`
Локальная авторизация пользователя.

**Запрос:**
```json
{
  "username": "admin",
  "password": "admin123"
}
```

**Ответ:**
```json
{
  "status": "success",
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "a1b2c3d4e5f6...",
    "token_type": "Bearer",
    "expires_in": 3600,
    "user": {
      "id": 1,
      "username": "admin",
      "name": "Администратор",
      "role": "admin",
      "company_id": "4e12b3c9-529c-4fe7-98e1-025eed8cb258"
    }
  }
}
```

#### POST `/api/local/refresh`
Обновление access токена.

**Запрос:**
```json
{
  "refresh_token": "a1b2c3d4e5f6..."
}
```

### Защищенные endpoints

#### GET `/api/local/current_user`
Получение данных текущего пользователя.

**Заголовки:**
```
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

#### POST `/api/local/logout`
Выход из системы (отзыв refresh токена).

#### POST `/api/local/register` (только для админов)
Регистрация нового пользователя.

**Запрос:**
```json
{
  "username": "newuser",
  "password": "password123",
  "email": "user@example.com",
  "name": "Новый пользователь",
  "company_id": "4e12b3c9-529c-4fe7-98e1-025eed8cb258",
  "role": "user"
}
```

## 🌐 WebSocket с авторизацией

### Подключение
```
WS /ws/live-data/:company_id?token=your_jwt_token
```

Или через заголовок:
```
Authorization: Bearer your_jwt_token
```

### Пример использования
```javascript
const token = localStorage.getItem('local_access_token')
const companyId = '4e12b3c9-529c-4fe7-98e1-025eed8cb258'
const ws = new WebSocket(`ws://localhost:8080/ws/live-data/${companyId}?token=${token}`)

ws.onmessage = (event) => {
  const message = JSON.parse(event.data)
  console.log('Received:', message)
}

// Подписка на канал
ws.send(JSON.stringify({
  type: 'subscribe',
  data: { channel: 'objects' }
}))
```

## 🧪 Тестирование

### Тестовые пользователи
После запуска `create_test_users.go` будут созданы:

| Логин | Пароль | Роль | Описание |
|-------|--------|------|----------|
| admin | admin123 | admin | Администратор |
| manager | manager123 | manager | Менеджер |
| tech | tech123 | tech | Техник |
| accountant | accountant123 | accountant | Бухгалтер |
| user | user123 | user | Пользователь |

### Тестирование API
```bash
# Вход в систему
curl -X POST http://localhost:8080/api/local/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "admin123"}'

# Получение текущего пользователя
curl -X GET http://localhost:8080/api/local/current_user \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"

# Обновление токена
curl -X POST http://localhost:8080/api/local/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token": "YOUR_REFRESH_TOKEN"}'
```

## 🔧 Интеграция с существующей системой

### Гибридный режим
Система может работать в гибридном режиме:

1. **Axenta Cloud API** - основная авторизация
2. **Локальная авторизация** - резервная или для специальных пользователей

### Middleware
```go
// Использование локальной авторизации
localAuth := middleware.NewLocalAuthMiddleware(jwtService)
router.Use(localAuth.RequireAuth())

// Проверка ролей
router.Use(localAuth.RequireRole("admin", "manager"))
```

### Frontend
```javascript
// Проверка типа авторизации
const { useAuth } = require('@/context/auth')           // Axenta Cloud
const { useLocalAuth } = require('@/composables/useLocalAuth') // Локальная

const auth = useAuth()
const localAuth = useLocalAuth()

// Используйте нужный тип авторизации
if (localAuth.isAuthenticated.value) {
  // Локальная авторизация активна
} else if (auth.isAuthenticated.value) {
  // Axenta Cloud авторизация активна
}
```

## 📊 Мониторинг и логирование

### Структурированные логи
```json
{
  "timestamp": "2025-01-27T12:00:00Z",
  "operation": "login_success",
  "username": "admin",
  "user_id": "1",
  "company_id": "4e12b3c9-529c-4fe7-98e1-025eed8cb258",
  "auth_type": "local",
  "status": "success",
  "role": "admin"
}
```

### Очистка токенов
```go
// Автоматическая очистка истекших токенов
jwtService.CleanupExpiredTokens()
```

## 🛡️ Безопасность в продакшене

### Обязательные настройки
1. **Смените JWT_SECRET** на криптографически стойкий ключ
2. **Настройте HTTPS** для всех соединений
3. **Ограничьте CORS** только для доверенных доменов
4. **Настройте rate limiting** для endpoints авторизации
5. **Включите логирование** всех операций авторизации

### Рекомендации
- Используйте короткие сроки жизни для access токенов (15-60 минут)
- Регулярно ротируйте refresh токены
- Мониторьте подозрительную активность
- Реализуйте блокировку аккаунтов при множественных неудачных попытках входа

## 🔄 Миграция

### От Axenta Cloud к локальной авторизации
1. Создайте локальных пользователей с теми же ролями
2. Обновите фронтенд для использования `useLocalAuth`
3. Переключите middleware на `LocalAuthMiddleware`

### Обратная совместимость
Система спроектирована для работы параллельно с существующей авторизацией Axenta Cloud без конфликтов.

## 📞 Поддержка

При возникновении проблем:
1. Проверьте логи сервера
2. Убедитесь в правильности переменных окружения
3. Проверьте подключение к базе данных
4. Убедитесь в корректности JWT токенов

## 🎉 Заключение

Локальная авторизация предоставляет полнофункциональную альтернативу Axenta Cloud API с поддержкой:
- JWT токенов с автоматическим обновлением
- Ролевой модели доступа
- WebSocket соединений с авторизацией
- Современного Vue 3 фронтенда
- Полной интеграции с существующей системой
