# API для управления пользователями и ролями

## Обзор

Этот документ описывает API эндпоинты для управления пользователями, ролями, разрешениями и шаблонами пользователей в Axenta CRM.

## Аутентификация

Все эндпоинты требуют авторизации через JWT токен в заголовке `Authorization: Bearer <token>`.

## Эндпоинты пользователей

### GET /api/users

Получить список пользователей с фильтрацией и пагинацией.

**Параметры запроса:**

- `page` (int) - номер страницы (по умолчанию 1)
- `limit` (int) - количество записей на странице (по умолчанию 20, максимум 100)
- `role` (string) - фильтр по имени роли
- `active` (boolean) - фильтр по активности (true/false)
- `search` (string) - поиск по username, email, first_name, last_name

**Пример ответа:**

```json
{
  "status": "success",
  "data": {
    "items": [
      {
        "id": 1,
        "username": "admin",
        "email": "admin@example.com",
        "first_name": "Admin",
        "last_name": "User",
        "is_active": true,
        "role_id": 1,
        "role": {
          "id": 1,
          "name": "admin",
          "display_name": "Администратор"
        },
        "template_id": null,
        "login_count": 5,
        "created_at": "2025-01-27T10:00:00Z",
        "updated_at": "2025-01-27T10:00:00Z"
      }
    ],
    "total": 1,
    "page": 1,
    "limit": 20,
    "pages": 1
  }
}
```

### GET /api/users/:id

Получить данные конкретного пользователя.

### POST /api/users

Создать нового пользователя.

**Тело запроса:**

```json
{
  "username": "newuser",
  "email": "newuser@example.com",
  "password": "password123",
  "first_name": "New",
  "last_name": "User",
  "role_id": 1,
  "template_id": 1,
  "is_active": true
}
```

### PUT /api/users/:id

Обновить данные пользователя.

### DELETE /api/users/:id

Удалить пользователя (soft delete).

## Эндпоинты ролей

### GET /api/roles

Получить список ролей.

**Параметры запроса:**

- `active` (boolean) - фильтр по активности
- `search` (string) - поиск по имени или описанию
- `with_permissions` (boolean) - загрузить разрешения

### GET /api/roles/:id

Получить данные конкретной роли с разрешениями и пользователями.

### POST /api/roles

Создать новую роль.

**Тело запроса:**

```json
{
  "name": "manager",
  "display_name": "Менеджер",
  "description": "Роль менеджера",
  "color": "#0000ff",
  "priority": 50,
  "is_active": true
}
```

### PUT /api/roles/:id

Обновить данные роли.

### DELETE /api/roles/:id

Удалить роль (только если не системная и не используется).

### PUT /api/roles/:id/permissions

Обновить разрешения роли.

**Тело запроса:**

```json
{
  "permission_ids": [1, 2, 3]
}
```

## Эндпоинты разрешений

### GET /api/permissions

Получить список разрешений.

**Параметры запроса:**

- `resource` (string) - фильтр по ресурсу
- `action` (string) - фильтр по действию
- `category` (string) - фильтр по категории
- `active` (boolean) - фильтр по активности
- `search` (string) - поиск по имени или описанию

### POST /api/permissions

Создать новое разрешение.

**Тело запроса:**

```json
{
  "name": "reports.read",
  "display_name": "Чтение отчетов",
  "description": "Разрешение на просмотр отчетов",
  "resource": "reports",
  "action": "read",
  "category": "reporting",
  "is_active": true
}
```

## Эндпоинты шаблонов пользователей

### GET /api/user-templates

Получить список шаблонов пользователей.

**Параметры запроса:**

- `active` (boolean) - фильтр по активности
- `role_id` (int) - фильтр по роли
- `search` (string) - поиск по имени или описанию
- `with_users` (boolean) - загрузить связанных пользователей

### GET /api/user-templates/:id

Получить данные конкретного шаблона с ролью и пользователями.

### POST /api/user-templates

Создать новый шаблон пользователя.

**Тело запроса:**

```json
{
  "name": "Стандартный пользователь",
  "description": "Шаблон для обычных пользователей",
  "role_id": 2,
  "settings": "{\"theme\": \"light\", \"language\": \"ru\"}",
  "is_active": true
}
```

### PUT /api/user-templates/:id

Обновить данные шаблона.

### DELETE /api/user-templates/:id

Удалить шаблон (только если не используется).

## Коды ошибок

- `400 Bad Request` - неверные данные запроса
- `401 Unauthorized` - требуется авторизация
- `403 Forbidden` - недостаточно прав (например, попытка изменить системную роль)
- `404 Not Found` - ресурс не найден
- `409 Conflict` - конфликт данных (например, дублирование username/email)
- `500 Internal Server Error` - внутренняя ошибка сервера

## Валидация

### Пользователи

- `username`: 3-50 символов, уникальный
- `email`: валидный email, уникальный
- `password`: минимум 6 символов
- `first_name`, `last_name`: максимум 50 символов
- `role_id`: должна существовать активная роль

### Роли

- `name`: 2-100 символов, уникальное
- `display_name`: 2-100 символов
- `description`: максимум 500 символов
- `color`: HEX цвет (7 символов)
- `priority`: неотрицательное число

### Разрешения

- `name`: 2-100 символов, уникальное
- `display_name`: 2-100 символов
- `resource`, `action`: 2-50 символов
- `category`: максимум 50 символов

### Шаблоны пользователей

- `name`: 2-100 символов
- `description`: максимум 500 символов
- `role_id`: должна существовать активная роль
- `settings`: валидный JSON (если указан)

## Мультитенантность

Все эндпоинты работают в контексте текущего tenant (компании). Данные изолированы между разными компаниями через систему схем PostgreSQL.
