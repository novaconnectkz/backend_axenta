# Авторизация через Axenta Cloud API

## Описание

Система авторизации интегрирована с Axenta Cloud API для централизованного управления пользователями.

## Эндпоинты

### Публичные (без авторизации)

- `POST /api/auth/login` - Авторизация пользователя
- `GET /api/ping` - Проверка статуса сервера

### Защищенные (требуют токен)

- `GET /api/current_user` - Информация о текущем пользователе
- `GET /api/objects` - Список объектов
- `GET/POST/PUT/DELETE /api/billing/*` - Управление биллингом

## Использование

### 1. Авторизация

**Запрос:**

```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "your_username",
    "password": "your_password"
  }'
```

**Ответ при успехе:**

```json
{
  "status": "success",
  "data": {
    "token": "your_jwt_token",
    "user": {
      "accountName": "Company Name",
      "accountType": "admin",
      "creatorName": "Creator",
      "id": "123",
      "lastLogin": "2025-01-27",
      "name": "User Name",
      "username": "username"
    }
  }
}
```

### 2. Использование защищенных эндпоинтов

Добавляйте полученный токен в заголовок `Authorization`:

```bash
curl -X GET http://localhost:8080/api/current_user \
  -H "Authorization: Bearer your_jwt_token"
```

## Роли и права доступа

- **manager/admin**: Все операции с складом
- **tech**: Резервирование, установка, удаление устройств только для назначенных заказов
- **accountant**: Просмотр списков и истории

## Middleware

Система использует `AuthMiddleware()` для защиты роутов:

- Проверяет наличие заголовка `Authorization`
- Валидирует токен через Axenta Cloud API
- Сохраняет информацию о пользователе в контекст Gin

## Безопасность

- Все запросы к внешним API используют HTTPS
- HTTP клиенты имеют таймауты (30 сек для логина, 10 сек для проверки токена)
- Пароли и токены не логируются
- Проверяется `company_id` для разграничения доступа

## Обработка ошибок

Все ошибки возвращаются в стандартном формате:

```json
{
  "status": "error",
  "error": "Описание ошибки"
}
```

Коды ошибок:

- `400` - Неверный формат запроса
- `401` - Неавторизован (отсутствует или неверный токен)
- `500` - Внутренняя ошибка сервера

## Настройка

Убедитесь, что в `.env` файле указаны необходимые переменные:

```env
SERVER_PORT=8080
# Другие переменные для базы данных
```

## Логирование

В операциях включаются следующие поля:

- `company_id` - ID компании пользователя
- `device_id` - ID устройства (где применимо)
- `user_id` - ID пользователя

## Шаблоны разработки

### Простые хендлеры (package api)

Используйте для быстрых операций:

```go
package api

import (
    "github.com/gin-gonic/gin"
)

func HandlerName(c *gin.Context) {
    // Логика хендлера
    c.JSON(200, gin.H{"status": "success", "data": result})
}
```

### Сложные хендлеры (package handlers)

Для CRUD операций и сложной логики используйте расширенный шаблон из `handlers/handler-template.go` с методами `SuccessResponse()` и `ErrorResponse()`.

## Тестирование

Примеры тестовых запросов:

1. **Проверка сервера:**

```bash
curl http://localhost:8080/ping
```

2. **Авторизация:**

```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "test", "password": "test123"}'
```

3. **Проверка авторизации:**

```bash
curl -X GET http://localhost:8080/api/current_user \
  -H "Authorization: Bearer YOUR_TOKEN"
```
