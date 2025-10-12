# 🚨 СРОЧНОЕ ИСПРАВЛЕНИЕ: Проблемы с tenant схемами на продакшене

## 📋 Обнаруженная проблема

На продакшене возникают ошибки:
```
ERROR: relation "user_templates" does not exist (SQLSTATE 42P01)
ERROR: relation "roles" does not exist (SQLSTATE 42P01)
```

**Причина**: Эндпоинты `/api/auth/roles` и `/api/auth/user-templates` используют мультитенантность, но tenant схемы не созданы.

## ✅ Решение

### Шаг 1: Загрузите исправленный код
Убедитесь, что на продакшене есть все исправления:
- `api/local_auth.go` - с методом `getPublicDB()`
- `services/jwt_service.go` - с методом `getPublicDB()`
- `database/migrations.go` - с LocalUser/RefreshToken в глобальных миграциях
- Новые утилиты в `cmd/`

### Шаг 2: Выполните миграции на продакшене

```bash
# 1. Миграция локальной авторизации (схема public)
go run cmd/migrate_local_auth/main.go

# 2. Создание компании по умолчанию
go run cmd/create_default_company/main.go

# 3. Создание tenant схемы с таблицами
go run cmd/create_missing_tables/main.go
```

**ИЛИ** используйте единый скрипт:
```bash
./deploy_with_tenant_fix.sh
```

### Шаг 3: Запустите сервер
```bash
go run main.go
```

## 🔍 Проверка исправлений

### Ожидаемые результаты:

1. **Эндпоинты работают без ошибок "table does not exist"**:
   ```bash
   curl "https://api.axenta.glonass-saratov.ru/api/auth/roles?page=1&limit=100" \
     -H "Authorization: Token YOUR_TOKEN"
   ```
   
   **Раньше**: `ERROR: relation "roles" does not exist`  
   **Теперь**: `{"error":"Invalid or expired token"}` (если токен неверный) или данные ролей

2. **User templates работают**:
   ```bash
   curl "https://api.axenta.glonass-saratov.ru/api/auth/user-templates?page=1&limit=100" \
     -H "Authorization: Token YOUR_TOKEN"
   ```
   
   **Раньше**: `ERROR: relation "user_templates" does not exist`  
   **Теперь**: Корректный ответ или ошибка авторизации

3. **Локальная авторизация работает**:
   ```bash
   curl -X POST "https://api.axenta.glonass-saratov.ru/api/local/login" \
     -H "Content-Type: application/json" \
     -d '{"username": "test", "password": "test"}'
   ```

## 📊 Архитектура после исправлений

### Схема `public` (глобальные таблицы):
- ✅ `companies` - информация о компаниях
- ✅ `local_users` - локальные пользователи
- ✅ `refresh_tokens` - токены обновления
- ✅ `billing_plans`, `subscriptions` - биллинг
- ✅ `integrations`, `integration_errors` - интеграции

### Схема `tenant_default` (данные компании по умолчанию):
- ✅ `roles` - роли пользователей
- ✅ `user_templates` - шаблоны пользователей
- ✅ `permissions` - разрешения
- ✅ `users` - пользователи компании
- ✅ `objects` - объекты мониторинга
- ✅ `contracts` - договоры

## 🔧 Техническая информация

### Что исправлено:

1. **LocalAuthAPI и JWTService** теперь используют `getPublicDB()` для принудительного переключения на схему public
2. **Система миграций** включает LocalUser/RefreshToken как глобальные таблицы
3. **Tenant схемы** создаются с помощью прямых SQL запросов для надежности
4. **Компания по умолчанию** создается автоматически для работы мультитенантности

### Изоляция схем:
```go
// В LocalAuthAPI и JWTService
func (api *LocalAuthAPI) getPublicDB() *gorm.DB {
    publicDB := api.db.Session(&gorm.Session{})
    if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
        log.Printf("⚠️ Не удалось переключиться на схему public: %v", err)
    }
    return publicDB
}
```

## 🚀 Быстрое исправление для продакшена

Если нужно быстро исправить продакшен:

```bash
# Подключитесь к серверу продакшена
ssh your-production-server

# Перейдите в директорию проекта
cd /path/to/backend_axenta

# Выполните исправления
go run cmd/create_default_company/main.go
go run cmd/create_missing_tables/main.go

# Перезапустите сервер
systemctl restart axenta-backend
# или
pm2 restart axenta-backend
```

## 📝 Логи для мониторинга

После исправлений в логах должны появиться:
- ✅ `Компания по умолчанию создана`
- ✅ `Схема tenant_default создана`
- ✅ `Все критические таблицы созданы успешно`
- ✅ `Default Axenta roles initialized successfully in public schema`

И **НЕ должно быть**:
- ❌ `relation "roles" does not exist`
- ❌ `relation "user_templates" does not exist`

## 🔗 Связанные файлы

- `cmd/migrate_local_auth/main.go` - миграция локальной авторизации
- `cmd/create_default_company/main.go` - создание компании по умолчанию  
- `cmd/create_missing_tables/main.go` - создание tenant таблиц
- `deploy_with_tenant_fix.sh` - полный скрипт развертывания
- `api/local_auth.go` - исправленная локальная авторизация
- `services/jwt_service.go` - исправленный JWT сервис
