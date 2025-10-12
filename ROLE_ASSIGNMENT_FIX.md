# Исправление назначения ролей пользователям из Axenta

## 🎯 Проблема
В интерфейсе управления пользователями в колонке "Роль" отображалось "Не назначена" для всех пользователей, загруженных из Axenta API.

## ✅ Решение
Обновлена функция `GetUsersFromAxentaCloud` для автоматического назначения ролей на основе параметра `accountType` из Axenta API.

## 🔧 Что изменено

### 1. Обновлена функция GetUsersFromAxentaCloud
**Файл**: `api/axenta_proxy.go`

**Было**:
```go
"role_id": 0, // Роли в Axenta Cloud работают по-другому
```

**Стало**:
```go
// Определяем роль на основе accountType
var roleInfo gin.H
var roleID interface{} = 0

if db != nil {
    accountType, ok := user["accountType"].(string)
    if ok {
        role, roleData := getRoleByAxentaType(db, accountType)
        if role != nil {
            roleID = role.ID
            roleInfo = roleData
        }
    }
}

// В ответе:
"role_id": roleID,
"role": roleInfo, // Полная информация о роли
```

### 2. Добавлены вспомогательные функции

#### getRoleByAxentaType()
Получает роль из базы данных на основе типа аккаунта Axenta:
- `"partner"` → роль `"partner"` (Партнер)
- `"client"` → роль `"client"` (Клиент)  
- другие → роль `"user"` (Пользователь)

#### mapAccountTypeToAxentaType()
Преобразует тип аккаунта Axenta в тип пользователя системы с безопасной обработкой null значений.

### 3. Автоматическая инициализация ролей
**Файл**: `main.go`

При запуске сервера автоматически создаются роли по умолчанию:
```go
// Инициализируем роли по умолчанию для Axenta пользователей
axentaUserService := services.NewAxentaUserService(database.DB)
if err := axentaUserService.EnsureDefaultRoles(); err != nil {
    log.Printf("Warning: Failed to ensure default Axenta roles: %v", err)
} else {
    log.Println("✅ Default Axenta roles initialized successfully")
}
```

### 4. Расширенный ответ API
Теперь каждый пользователь из Axenta возвращается с полной информацией о роли:

```json
{
  "id": 123,
  "username": "partner_user",
  "email": "partner@example.com",
  "account_type": "partner",
  "role_id": 1,
  "role": {
    "id": 1,
    "name": "partner",
    "display_name": "Партнер",
    "description": "Роль партнера из Axenta",
    "color": "#2196F3",
    "is_active": true
  },
  "axenta_user_type": "partner",
  "is_axenta_user": true,
  "external_source": "axenta"
}
```

## 📋 Автоматическое назначение ролей

| accountType в Axenta | Роль в CRM | Отображение |
|---------------------|------------|-------------|
| `"partner"` | `partner` | **Партнер** 🔵 |
| `"client"` | `client` | **Клиент** 🟢 |
| другие/null | `user` | **Пользователь** 🟠 |

## 🚀 Результат

### До исправления:
```
| Пользователь | Роль |
|--------------|------|
| partner1     | Не назначена |
| client1      | Не назначена |
```

### После исправления:
```
| Пользователь | Роль |
|--------------|------|
| partner1     | Партнер |
| client1      | Клиент |
```

## 🧪 Тестирование

1. **Запустите сервер**:
   ```bash
   cd /Users/com/backend_axenta
   go run main.go
   ```

2. **Проверьте создание ролей** в логах:
   ```
   🔧 Initializing default Axenta roles...
   Created default role: partner
   Created default role: client  
   Created default role: user
   ✅ Default Axenta roles initialized successfully
   ```

3. **Обновите страницу пользователей** - роли теперь должны отображаться корректно!

## 🎉 Заключение

Проблема решена! Теперь при загрузке пользователей из Axenta API:
- ✅ Автоматически назначаются роли на основе `accountType`
- ✅ Роли отображаются в интерфейсе
- ✅ Поддерживаются все типы: партнеры, клиенты, пользователи
- ✅ Роли создаются автоматически при запуске сервера
