# Правила для Go-бэкенда системы Axenta Cloud

**description**: Правила для Go-бэкенда системы Axenta Cloud  
**globs**: `/backend_axenta/**/*.go`  
**alwaysApply**: true

## Общие правила разработки

1. **Стиль кода**:

   - Используйте camelCase для переменных и функций
   - Следуйте стандартам `go fmt` и `golint`
   - Используйте осмысленные имена переменных и функций

2. **Структура проекта**:

   - Модели данных создавайте с GORM, используя теги `gorm` для PostgreSQL
   - API-роуты разрабатывайте с Gin, следуя RESTful подходу (GET, POST, PUT, DELETE)
   - Возвращайте JSON-ответы с полями `status`, `data`, `error`

3. **Константы**:
   - Определяйте константы в `const` блоках
   - Группируйте логически связанные константы

## Авторизация через Axenta Cloud API

### Эндпоинты авторизации:

- **Логин**: `POST https://axenta.cloud/api/auth/login/`
- **Текущий пользователь**: `GET https://axenta.cloud/api/current_user/`
- **Заголовок авторизации**: `Authorization: Bearer <token>`

### Структура запроса логина:

```go
type LoginRequest struct {
    Username string `json:"username" binding:"required,min=3,max=64"`
    Password string `json:"password" binding:"required,min=3,max=64"`
}
```

### Структура ответа авторизации:

```go
type AuthResponse struct {
    Status string `json:"status"`
    Data   struct {
        Token string `json:"token"`
        User  struct {
            AccountName string `json:"accountName"`
            AccountType string `json:"accountType"`
            CreatorName string `json:"creatorName"`
            ID          string `json:"id"`
            LastLogin   string `json:"lastLogin"`
            Name        string `json:"name"`
            Username    string `json:"username"`
        } `json:"user"`
    } `json:"data,omitempty"`
    Error string `json:"error,omitempty"`
}
```

## Шаблоны хендлеров

### Простой хендлер (package api)

Для быстрых хендлеров в пакете `api`:

```go
package api

import (
    "github.com/gin-gonic/gin"
)

func HandlerName(c *gin.Context) {
    // Логика хендлера
    c.JSON(200, gin.H{"status": "success", "data": nil})
}
```

### Расширенный хендлер (package handlers)

Для сложных хендлеров используйте шаблон из `@handler-template.go`:

```go
package handlers

import (
    "net/http"
    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

func HandlerName(c *gin.Context) {
    // 1. Валидация входных данных
    var req RequestStruct
    if err := c.ShouldBindJSON(&req); err != nil {
        ErrorResponse(c, http.StatusBadRequest, "Некорректные входные данные: "+err.Error())
        return
    }

    // 2. Бизнес-логика
    // ... обработка данных ...

    // 3. Возврат результата
    SuccessResponse(c, http.StatusOK, result)
}
```

### Рекомендации по выбору шаблона:

- **Простой шаблон (`api`)**: для простых операций, прокси к внешним API, быстрых хендлеров
- **Расширенный шаблон (`handlers`)**: для сложной бизнес-логики, работы с базой данных, CRUD операций

## Пример хендлера авторизации

```go
package api

import (
    "bytes"
    "encoding/json"
    "net/http"
    "github.com/gin-gonic/gin"
)

func Login(c *gin.Context) {
    var req struct {
        Username string `json:"username" binding:"required,min=3,max=64"`
        Password string `json:"password" binding:"required,min=3,max=64"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "status": "error",
            "error": "Invalid request"
        })
        return
    }

    // Подготовка запроса к Axenta Cloud API
    loginData := map[string]string{
        "username": req.Username,
        "password": req.Password,
    }

    jsonData, err := json.Marshal(loginData)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "status": "error",
            "error": "Internal server error"
        })
        return
    }

    // Отправка запроса к Axenta Cloud API
    resp, err := http.Post(
        "https://axenta.cloud/api/auth/login/",
        "application/json",
        bytes.NewBuffer(jsonData),
    )
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "status": "error",
            "error": "Authentication service unavailable"
        })
        return
    }
    defer resp.Body.Close()

    // Обработка ответа от Axenta Cloud API
    var authResp AuthResponse
    if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "status": "error",
            "error": "Invalid response from authentication service"
        })
        return
    }

    if authResp.Status != "success" {
        c.JSON(http.StatusUnauthorized, gin.H{
            "status": "error",
            "error": authResp.Error,
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "status": "success",
        "data": authResp.Data,
    })
}
```

## Middleware для авторизации

```go
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        if token == "" {
            c.JSON(http.StatusUnauthorized, gin.H{
                "status": "error",
                "error": "Authorization header required",
            })
            c.Abort()
            return
        }

        // Проверка токена через Axenta Cloud API
        req, err := http.NewRequest("GET", "https://axenta.cloud/api/current_user/", nil)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{
                "status": "error",
                "error": "Internal server error",
            })
            c.Abort()
            return
        }

        req.Header.Set("Authorization", token)

        client := &http.Client{}
        resp, err := client.Do(req)
        if err != nil || resp.StatusCode != http.StatusOK {
            c.JSON(http.StatusUnauthorized, gin.H{
                "status": "error",
                "error": "Invalid or expired token",
            })
            c.Abort()
            return
        }
        defer resp.Body.Close()

        c.Next()
    }
}
```

## Интеграции

- **Bitrix24, 1C, Telegram**: Используйте `net/http` или специализированные пакеты (например, `github.com/go-telegram-bot-api/telegram-bot-api`)
- **Асинхронные операции**: Используйте Goroutines для неблокирующих операций

## Безопасность

- Всегда валидируйте входные данные
- Используйте HTTPS для всех внешних API-запросов
- Не логируйте чувствительные данные (пароли, токены)
- Устанавливайте таймауты для HTTP-клиентов

## Логирование

Включайте в логи операций:

- `company_id`
- `device_id`
- `user_id`

## Контроль доступа

Роли пользователей:

- **manager/admin**: все операции с складом
- **tech**: резервирование, установка, удаление устройств только для назначенных заказов
- **accountant**: просмотр списков и истории

Всегда проверяйте `company_id` в операциях и включайте unit-тесты для предотвращения несанкционированного доступа к устройствам других компаний.
