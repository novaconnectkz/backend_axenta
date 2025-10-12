# Система ролей пользователей Axenta

## Обзор

Реализована система загрузки и управления ролями пользователей из Axenta API. Система поддерживает два типа пользователей из Axenta (`partner` и `client`) и локальных пользователей системы.

## Типы пользователей

### Пользователи Axenta
- **Partner (Партнер)** - пользователи с типом аккаунта "partner" в Axenta
- **Client (Клиент)** - пользователи с типом аккаунта "client" в Axenta

### Локальные пользователи
- **Local (Локальный)** - пользователи, созданные непосредственно в CRM системе

## Архитектура

### Модель User
Расширена модель `User` для поддержки ролей Axenta:

```go
type User struct {
    // ... существующие поля ...
    
    // Поля для Axenta интеграции
    AxentaUserType string `json:"axenta_user_type"` // partner, client, local
    AxentaUserID   string `json:"axenta_user_id"`   // ID пользователя в Axenta
    IsAxentaUser   bool   `json:"is_axenta_user"`   // Пользователь из Axenta или локальный
}
```

### Методы модели User
```go
func (u *User) IsPartner() bool        // Проверка роли партнера
func (u *User) IsClient() bool         // Проверка роли клиента  
func (u *User) IsLocalUser() bool      // Проверка локального пользователя
func (u *User) SetAxentaRole(userType, userID string) // Установка роли Axenta
func (u *User) ClearAxentaRole()       // Очистка роли Axenta
```

### AxentaUserService
Сервис для работы с пользователями Axenta:

```go
type AxentaUserService struct {
    db         *gorm.DB
    httpClient *http.Client
    baseURL    string
}
```

#### Основные методы:
- `GetUserFromAxenta(token string)` - получение данных пользователя из Axenta API
- `SyncUserWithAxenta(token, username string)` - синхронизация пользователя с Axenta
- `CreateLocalUser(username, email, password string, roleID uint)` - создание локального пользователя
- `GetUsersByType(userType string)` - получение пользователей по типу
- `EnsureDefaultRoles()` - создание ролей по умолчанию

## API Endpoints

### Управление пользователями Axenta
```
GET    /api/auth/axenta-users                 - Получить пользователей по типу (?type=partner|client|local|all)
GET    /api/auth/axenta-users/stats           - Статистика по типам пользователей
POST   /api/auth/axenta-users/local           - Создать локального пользователя
PUT    /api/auth/axenta-users/:id/role        - Обновить роль Axenta пользователя
POST   /api/auth/axenta-users/sync            - Синхронизировать пользователя с Axenta
POST   /api/auth/axenta-users/ensure-roles    - Создать роли по умолчанию
```

### Примеры использования

#### Получение пользователей по типу
```bash
# Получить всех партнеров
GET /api/auth/axenta-users?type=partner

# Получить всех клиентов
GET /api/auth/axenta-users?type=client

# Получить локальных пользователей
GET /api/auth/axenta-users?type=local

# Получить всех пользователей
GET /api/auth/axenta-users?type=all
```

#### Создание локального пользователя
```bash
POST /api/auth/axenta-users/local
Content-Type: application/json

{
  "username": "localuser",
  "email": "user@example.com",
  "password": "securepassword",
  "first_name": "Имя",
  "last_name": "Фамилия",
  "phone": "+7 900 123-45-67",
  "role_id": 1
}
```

#### Обновление роли Axenta пользователя
```bash
PUT /api/auth/axenta-users/123/role
Content-Type: application/json

{
  "axenta_user_type": "partner",
  "axenta_user_id": "456",
  "is_axenta_user": true
}
```

#### Синхронизация с Axenta
```bash
POST /api/auth/axenta-users/sync
Content-Type: application/json

{
  "token": "axenta_api_token",
  "username": "username"
}
```

#### Статистика пользователей
```bash
GET /api/auth/axenta-users/stats

# Ответ:
{
  "status": "success",
  "data": {
    "partners": {
      "count": 5,
      "users": [...]
    },
    "clients": {
      "count": 10,
      "users": [...]
    },
    "local": {
      "count": 3,
      "users": [...]
    },
    "total": 18
  }
}
```

## Процесс аутентификации

### Обновленный процесс Login
1. Аутентификация в Axenta API
2. Получение данных пользователя из Axenta
3. **Синхронизация пользователя с локальной базой данных**
4. **Автоматическое создание/обновление пользователя с ролью Axenta**
5. Возврат JWT токена с информацией о роли

### Ответ аутентификации
```json
{
  "status": "success",
  "data": {
    "token": "jwt_token",
    "user": {
      "id": "123",
      "username": "username",
      "name": "User Name",
      "email": "user@example.com",
      "accountType": "partner",
      "axentaUserType": "partner",
      "isAxentaUser": true,
      "roleId": 1,
      "role": {
        "id": 1,
        "name": "partner",
        "display_name": "Партнер"
      }
    }
  }
}
```

## Роли по умолчанию

Система автоматически создает следующие роли:

### Partner (Партнер)
- **Название**: `partner`
- **Отображаемое имя**: `Партнер`
- **Описание**: `Роль партнера из Axenta`
- **Цвет**: `#2196F3`
- **Приоритет**: `100`

### Client (Клиент)
- **Название**: `client`
- **Отображаемое имя**: `Клиент`
- **Описание**: `Роль клиента из Axenta`
- **Цвет**: `#4CAF50`
- **Приоритет**: `50`

### User (Пользователь)
- **Название**: `user`
- **Отображаемое имя**: `Пользователь`
- **Описание**: `Локальный пользователь системы`
- **Цвет**: `#FF9800`
- **Приоритет**: `25`

## База данных

### Миграция
Файл: `database/migrations/add_axenta_user_fields.sql`

Добавляет следующие поля в таблицу `users`:
- `axenta_user_type` VARCHAR(50) - тип пользователя Axenta
- `axenta_user_id` VARCHAR(100) - ID пользователя в Axenta
- `is_axenta_user` BOOLEAN - флаг пользователя Axenta

### Индексы
Созданы индексы для оптимизации поиска:
- `idx_users_axenta_user_type`
- `idx_users_axenta_user_id`
- `idx_users_is_axenta_user`
- `idx_users_axenta_composite`

## Тестирование

### Unit тесты
Файл: `services/axenta_user_service_test.go`

Покрывает:
- Создание ролей по умолчанию
- Создание локальных пользователей
- Получение пользователей по типу
- Методы модели User
- Маппинг типов аккаунтов
- Получение ролей по типу

### Запуск тестов
```bash
go test ./services -v -run TestAxentaUserService
```

## Безопасность

### Аутентификация
- Все API endpoints защищены аутентификацией
- Используется JWT токены для авторизации
- Поддержка мультитенантности

### Валидация данных
- Валидация входных данных на уровне API
- Проверка существования ролей при создании пользователей
- Хеширование паролей для локальных пользователей

## Логирование

Все операции с пользователями Axenta логируются:
- Успешная синхронизация пользователей
- Ошибки интеграции с Axenta API
- Создание новых пользователей
- Обновление ролей

## Мониторинг

### Метрики
- Количество пользователей по типам
- Статистика синхронизации с Axenta
- Ошибки интеграции

### Health Check
Проверка доступности Axenta API и корректности синхронизации пользователей.

## Развитие

### Планируемые улучшения
1. Автоматическая синхронизация пользователей по расписанию
2. Кэширование данных пользователей Axenta
3. Webhook уведомления об изменениях в Axenta
4. Расширенные права доступа для разных типов пользователей
5. Интеграция с системой уведомлений

### Конфигурация
Настройки интеграции с Axenta в файле конфигурации:
```yaml
axenta:
  api_url: "https://axenta.cloud/api"
  timeout: 30s
  retry_attempts: 3
  sync_interval: "1h"
```
