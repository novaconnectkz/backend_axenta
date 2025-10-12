# Исправление ошибки 404 для API пользователей

## Проблема

Фронтенд получал ошибки 404 при обращении к следующим эндпоинтам:
- `GET /api/auth/roles`
- `GET /api/auth/user-templates` 
- `GET /api/auth/users/stats`
- `GET /api/auth/users`

## Причина

Фронтенд обращался к эндпоинтам с префиксом `/auth/`, но в бэкенде эти эндпоинты были зарегистрированы без этого префикса:
- `/api/roles` вместо `/api/auth/roles`
- `/api/user-templates` вместо `/api/auth/user-templates`
- `/api/users` вместо `/api/auth/users`
- `/api/users/stats` вместо `/api/auth/users/stats`

## Решение

Добавлены дублирующие эндпоинты с префиксом `/auth/` в файл `main.go`:

```go
// === ЭНДПОИНТЫ С ПРЕФИКСОМ /auth/ ДЛЯ ФРОНТЕНДА ===
log.Println("🔧 Registering /auth/ prefixed endpoints for frontend compatibility...")

// Пользователи с префиксом /auth/
apiGroup.GET("/auth/users", api.GetUsersFromAxentaCloud)
apiGroup.GET("/auth/users/", api.GetUsersFromAxentaCloud)
apiGroup.GET("/auth/users/stats", api.GetUsersStatsFromAxentaCloud)
apiGroup.GET("/auth/users/stats/", api.GetUsersStatsFromAxentaCloud)
apiGroup.GET("/auth/users/:id", api.GetUser)
apiGroup.POST("/auth/users", api.CreateUser)
apiGroup.PUT("/auth/users/:id", api.UpdateUser)
apiGroup.DELETE("/auth/users/:id", api.DeleteUser)
apiGroup.POST("/auth/users/bulk-delete", api.BulkDeleteUsers)

// Роли с префиксом /auth/
apiGroup.GET("/auth/roles", api.GetRoles)
apiGroup.GET("/auth/roles/:id", api.GetRole)
apiGroup.POST("/auth/roles", api.CreateRole)
apiGroup.PUT("/auth/roles/:id", api.UpdateRole)
apiGroup.DELETE("/auth/roles/:id", api.DeleteRole)
apiGroup.PUT("/auth/roles/:id/permissions", api.UpdateRolePermissions)

// Разрешения с префиксом /auth/
apiGroup.GET("/auth/permissions", api.GetPermissions)
apiGroup.POST("/auth/permissions", api.CreatePermission)

// Шаблоны пользователей с префиксом /auth/
apiGroup.GET("/auth/user-templates", api.GetUserTemplates)
apiGroup.GET("/auth/user-templates/:id", api.GetUserTemplate)
apiGroup.POST("/auth/user-templates", api.CreateUserTemplate)
apiGroup.PUT("/auth/user-templates/:id", api.UpdateUserTemplate)
apiGroup.DELETE("/auth/user-templates/:id", api.DeleteUserTemplate)
```

## Развертывание

### Автоматическое развертывание
Изменения были закоммичены в ветку `main`, что должно запустить GitHub Actions для автоматического развертывания.

### Ручное развертывание
Если автоматическое развертывание не работает, выполните следующие команды на сервере:

```bash
# Подключиться к серверу
ssh root@api.axenta.glonass-saratov.ru

# Перейти в рабочую директорию
cd /var/www/backend_axenta

# Остановить сервис
sudo systemctl stop axenta-backend

# Обновить код
git fetch --all --prune
git pull origin main

# Собрать новую версию
/usr/local/go/bin/go build -ldflags="-w -s" -o axenta_backend main.go

# Запустить сервис
sudo systemctl start axenta-backend

# Проверить статус
sudo systemctl status axenta-backend
```

## Проверка

После развертывания проверьте доступность эндпоинтов:

```bash
# Должны возвращать 401 (Unauthorized) вместо 404
curl -s -w "HTTP Status: %{http_code}\n" https://api.axenta.glonass-saratov.ru/api/auth/roles
curl -s -w "HTTP Status: %{http_code}\n" https://api.axenta.glonass-saratov.ru/api/auth/users
curl -s -w "HTTP Status: %{http_code}\n" https://api.axenta.glonass-saratov.ru/api/auth/user-templates
curl -s -w "HTTP Status: %{http_code}\n" https://api.axenta.glonass-saratov.ru/api/auth/users/stats
```

## Коммит

Изменения были закоммичены с сообщением:
```
Добавлены недостающие эндпоинты /auth/ для совместимости с фронтендом

- Добавлены эндпоинты /auth/users, /auth/users/stats
- Добавлены эндпоинты /auth/roles, /auth/user-templates  
- Добавлены эндпоинты /auth/permissions
- Исправлена проблема с 404 ошибками на продакшене
```

Коммит: `e6d3e5b`
