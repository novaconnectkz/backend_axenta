# Настройка CMS пользователей

## Обзор

Реализован функционал создания пользователей через CMS API с токен-авторизацией.

## Backend (Go + Gin + GORM)

### Новые файлы:
- `middleware/axenta_api_tokens.go` - middleware для проверки API токенов
- `models/user_tab.go` - модель вкладок пользователя
- `models/user_access.go` - модель доступов пользователя  
- `api/cms_users.go` - API endpoints для CMS пользователей

### Новые endpoints:
- `POST /api/cms/users/` - создание пользователя (201)

### Авторизация:
- Заголовок: `Authorization: Token <TOKEN>`
- Токены берутся из переменной `AXENTA_API_TOKENS` (CSV)
- Account ID берется из `AXENTA_DEFAULT_ACCOUNT_ID`

### Валидация:
- `name`, `username`, `email`, `password` - обязательные поля
- Пароль минимум 6 символов
- Email и username должны быть уникальными
- Пароль хешируется через bcrypt

### База данных:
- Создается пользователь в таблице `users`
- Создаются записи в `user_tabs` для видимых вкладок
- Создаются записи в `user_accesses` для доступов
- Все операции выполняются в транзакции

## Frontend (Vue 3 + TS + Vuetify 3)

### Новые файлы:
- `src/views/CreateUser.vue` - страница создания пользователя

### Новые маршруты:
- `/users/create` - страница создания пользователя

### Обновленные файлы:
- `src/router/index.ts` - добавлен маршрут
- `src/views/AccountsPage.vue` - добавлена кнопка "Создать пользователя"

### Функционал:
- Форма с полями: name, username, email, password, hasAdminAccess, visibleTabsNames, accesses
- Валидация полей (required, email format, min length)
- Axios запросы с авторизацией через токен
- Уведомления об успехе/ошибках через snackbar
- Кнопка дизейблится при невалидной форме или во время запроса

## Настройка

### 1. Переменные окружения

Добавьте в `.env` файл:

```bash
# API токены для авторизации (CSV список)
AXENTA_API_TOKENS=your_token_here,another_token_here

# ID аккаунта по умолчанию
AXENTA_DEFAULT_ACCOUNT_ID=1
```

### 2. Миграции базы данных

Новые таблицы будут созданы автоматически при запуске сервера:
- `user_tabs` - вкладки пользователей
- `user_accesses` - доступы пользователей

### 3. Тестирование

Запустите тестовый скрипт:

```bash
./test_cms_user_creation.sh
```

## Использование

### 1. Доступ к разделу "Учетные записи"

В навигации перейдите в "Учетные записи" (`/accounts`).

### 2. Создание пользователя

Нажмите кнопку "Создать пользователя" в разделе действий.

### 3. Заполнение формы

- **Основная информация**: имя, username, email, пароль
- **Права доступа**: административный доступ (переключатель)
- **Видимые вкладки**: выбор доступных разделов
- **Настройки доступа**: права по областям (view, edit, create, delete, full)

### 4. Отправка

После заполнения формы и прохождения валидации нажмите "Создать пользователя".

## API Документация

### POST /api/cms/users/

**Заголовки:**
```
Content-Type: application/json
Authorization: Token <TOKEN>
```

**Тело запроса:**
```json
{
  "name": "Полное имя",
  "username": "username",
  "email": "user@example.com", 
  "password": "password123",
  "hasAdminAccess": false,
  "visibleTabsNames": ["monitoring", "reports"],
  "accesses": {
    "objects": {"perms": ["view", "edit"]},
    "users": {"perms": ["view"]}
  }
}
```

**Ответ 201:**
```json
{
  "id": 1,
  "email": "user@example.com",
  "name": "Полное имя",
  "username": "username",
  "creatorName": "Создатель",
  "lastLogin": null,
  "creationDatetime": "2025-01-27T10:00:00Z",
  "accountId": "1",
  "accountName": "Default Account",
  "accountType": "partner",
  "accountIsActive": true,
  "accountBlockingDatetime": null,
  "isActive": true,
  "language": "ru",
  "timezone": 3,
  "isAdmin": false,
  "hasAdminAccess": false,
  "visibleTabsNames": ["monitoring", "reports"],
  "currentUserAccess": ["view", "edit"],
  "addressFormat": [],
  "objectCardSettings": {},
  "monitoringItemSetup": {},
  "visibleObjectsIds": [],
  "visibleObjectsCount": 0,
  "visibleGeozoneIds": [],
  "commonAccesses": ["view", "edit"]
}
```

**Ошибки:**
- `400` - ошибки валидации
- `401` - неверный токен
- `409` - дублирующиеся email/username
- `500` - внутренняя ошибка сервера
