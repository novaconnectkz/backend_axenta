# Система аудит-логирования Axenta CRM

## Описание

Полнофункциональная система аудит-логирования действий пользователей в Axenta CRM. Система записывает все важные действия пользователей в формате JSON, сохраняет в файл, stdout и базу данных PostgreSQL.

## Возможности

### Backend

1. **Многоуровневое логирование**
   - Файл (JSONL формат)
   - Stdout (для production мониторинга)
   - База данных PostgreSQL (для анализа и поиска)

2. **Автоматическая запись событий**
   - Авторизация и аутентификация
   - CRUD операции над всеми сущностями
   - Изменения настроек системы
   - API вызовы
   - Ошибки и исключения
   - Системные события

3. **Богатый контекст логов**
   - Временная метка (timestamp)
   - ID и имя пользователя
   - Роль пользователя
   - IP адрес
   - User Agent
   - HTTP метод и путь
   - Код ответа
   - Длительность операции
   - Дополнительные детали (JSONB)
   - Статус успеха/ошибки

4. **REST API для доступа к логам**
   - Получение списка с фильтрацией
   - Просмотр деталей конкретного лога
   - Статистика по логам
   - Экспорт в JSON

### Frontend

1. **Интерфейс просмотра логов**
   - Таблица с пагинацией и сортировкой
   - Фильтры по уровню, статусу, дате, пользователю
   - Полнотекстовый поиск
   - Просмотр деталей каждой записи

2. **Аналитика и статистика**
   - Общая статистика за период
   - Топ действий
   - Распределение по уровням
   - Активность по дням

3. **Экспорт данных**
   - Экспорт в JSON формат
   - С применением текущих фильтров

## Установка и настройка

### Backend

#### 1. Конфигурация

Добавьте переменные окружения в `.env`:

```env
# Аудит-логирование
AUDIT_ENABLED=true
AUDIT_LOG_FILE=logs/audit.jsonl
AUDIT_LOG_TO_STDOUT=false
AUDIT_LOG_TO_FILE=true
AUDIT_LOG_TO_DB=true
AUDIT_MAX_FILE_SIZE=100
AUDIT_MAX_BACKUPS=10
```

#### 2. Миграция базы данных

Система автоматически применит миграцию при запуске. Миграция создаст таблицу `audit_logs` с необходимыми индексами.

Для ручного применения:

```bash
cd backend_axenta
# Применить миграцию
psql -U postgres -d axenta_db -f database/migrations/000022_create_audit_logs.up.sql

# Откатить миграцию (если необходимо)
psql -U postgres -d axenta_db -f database/migrations/000022_create_audit_logs.down.sql
```

#### 3. Инициализация

Система аудита инициализируется автоматически в `main.go` при запуске приложения:

```go
// Инициализация происходит автоматически
// Логи начинают записываться сразу после старта
```

### Frontend

Роут автоматически добавлен в router:

```
/settings/audit-logs
```

Доступ через раздел "Настройки" → "Аудит действий пользователей"

## Использование

### Backend - Логирование событий

#### 1. Автоматическое логирование через middleware

Все HTTP запросы автоматически логируются:

```go
// Middleware подключен глобально в main.go
r.Use(audit.Middleware())
```

#### 2. Ручное логирование в handlers

```go
import "backend_axenta/audit"

// Успешная операция
audit.LogSuccess(c, "user.created", gin.H{
    "user_id": user.ID,
    "username": user.Username,
})

// Ошибка
audit.LogError(c, "user.create.failed", err, gin.H{
    "username": req.Username,
    "reason": "duplicate_username",
})

// Простое логирование
audit.LogFromContext(c, "settings.updated", gin.H{
    "setting": "billing_enabled",
    "old_value": false,
    "new_value": true,
})

// Логирование без gin.Context
audit.Log(userID, "system.backup.started", map[string]interface{}{
    "backup_type": "full",
    "destination": "/backups/",
})
```

#### 3. Примеры логирования для разных операций

**Авторизация:**
```go
// Успешный вход
audit.LogSuccess(c, "auth.login.success", gin.H{
    "user_id": user.ID,
    "username": user.Username,
})

// Неудачная попытка входа
audit.LogError(c, "auth.login.failed", err, gin.H{
    "username": req.Username,
    "reason": "invalid_credentials",
})
```

**CRUD операции:**
```go
// Создание
audit.LogSuccess(c, "object.created", gin.H{
    "object_id": object.ID,
    "object_name": object.Name,
    "object_type": object.Type,
})

// Обновление
audit.LogSuccess(c, "object.updated", gin.H{
    "object_id": id,
    "changed_fields": []string{"name", "status"},
})

// Удаление
audit.LogSuccess(c, "object.deleted", gin.H{
    "object_id": id,
    "object_name": object.Name,
})
```

**Системные операции:**
```go
// Изменение настроек
audit.LogSuccess(c, "settings.billing.updated", gin.H{
    "settings": changedSettings,
})

// Экспорт данных
audit.LogSuccess(c, "data.export.completed", gin.H{
    "export_type": "objects",
    "format": "xlsx",
    "records_count": count,
})
```

### Frontend - Просмотр логов

1. Перейдите в раздел **Настройки** → **Аудит действий пользователей**
2. Используйте фильтры для поиска нужных логов:
   - Поиск по тексту
   - Фильтр по уровню (Инфо, Успех, Предупреждение, Ошибка)
   - Фильтр по статусу (Успешно/Ошибка)
   - Фильтр по дате
3. Нажмите на иконку глаза чтобы посмотреть детали записи
4. Используйте кнопку "Экспорт в JSON" для выгрузки данных

## API Endpoints

### GET /api/auth/audit/logs

Получить список логов с фильтрацией

**Параметры запроса:**
- `page` - номер страницы (по умолчанию: 1)
- `per_page` - количество на странице (по умолчанию: 50, макс: 1000)
- `user_id` - фильтр по ID пользователя
- `action` - фильтр по действию (поддерживает LIKE)
- `level` - фильтр по уровню (info, success, warning, error)
- `success` - фильтр по статусу (true/false)
- `start_date` - фильтр по начальной дате (ISO 8601)
- `end_date` - фильтр по конечной дате (ISO 8601)
- `search` - полнотекстовый поиск
- `sort_by` - поле для сортировки (по умолчанию: timestamp)
- `sort_order` - порядок сортировки (asc/desc, по умолчанию: desc)

**Пример:**
```bash
curl -X GET "http://localhost:8080/api/auth/audit/logs?level=error&page=1&per_page=25" \
  -H "Authorization: Token YOUR_TOKEN"
```

**Ответ:**
```json
{
  "status": "success",
  "data": {
    "items": [...],
    "total": 150,
    "page": 1,
    "per_page": 25,
    "total_pages": 6
  }
}
```

### GET /api/auth/audit/logs/:id

Получить детали конкретного лога

**Пример:**
```bash
curl -X GET "http://localhost:8080/api/auth/audit/logs/123" \
  -H "Authorization: Token YOUR_TOKEN"
```

### GET /api/auth/audit/stats

Получить статистику по логам

**Параметры запроса:**
- `days` - количество дней для статистики (по умолчанию: 7)

**Пример:**
```bash
curl -X GET "http://localhost:8080/api/auth/audit/stats?days=30" \
  -H "Authorization: Token YOUR_TOKEN"
```

**Ответ:**
```json
{
  "status": "success",
  "data": {
    "period": 30,
    "start_date": "2025-10-20T00:00:00Z",
    "stats": {
      "total_logs": 15420,
      "success_count": 14890,
      "error_count": 530,
      "unique_users": 45,
      "unique_actions": 125
    },
    "top_actions": [...],
    "level_stats": [...],
    "daily_activity": [...]
  }
}
```

### GET /api/auth/audit/export

Экспортировать логи в JSON

Принимает те же параметры что и `/api/auth/audit/logs`, но без пагинации. Максимум 10,000 записей.

**Пример:**
```bash
curl -X GET "http://localhost:8080/api/auth/audit/export?level=error" \
  -H "Authorization: Token YOUR_TOKEN" \
  -o audit_logs.json
```

## Структура данных

### AuditLog (модель БД)

```go
type AuditLog struct {
    ID         uint                   // ID записи
    Timestamp  time.Time              // Время события
    UserID     string                 // ID пользователя
    Username   string                 // Имя пользователя
    Role       string                 // Роль пользователя
    TenantID   string                 // ID компании
    IP         string                 // IP адрес
    UserAgent  string                 // User Agent
    Action     string                 // Действие
    Resource   string                 // Ресурс
    Method     string                 // HTTP метод
    Path       string                 // Путь запроса
    StatusCode int                    // HTTP код
    Details    map[string]interface{} // Дополнительные детали
    Success    bool                   // Успех операции
    Level      string                 // Уровень (info/success/warning/error)
    Error      string                 // Текст ошибки
    Duration   int64                  // Длительность в мс
}
```

## Производительность

Система аудита спроектирована для минимального влияния на производительность:

1. **Асинхронная запись в БД** - запись в базу данных происходит в отдельной горутине
2. **Буферизованная запись в файл** - использует буферизованный writer
3. **Индексы БД** - все важные поля проиндексированы
4. **JSONB для details** - эффективное хранение дополнительных данных

## Безопасность

1. **Доступ только для авторизованных пользователей** - все API endpoints требуют токен
2. **Не логируются чувствительные данные** - пароли и токены никогда не попадают в логи
3. **Изоляция по компаниям** - через tenant_id
4. **Ротация логов** - старые файлы автоматически архивируются

## Мониторинг

### Логи приложения

Система сама записывает события своей работы:

```
✅ Audit logging initialized: file=logs/audit.jsonl, stdout=false, file_logging=true
✅ Audit logs table created/verified
✅ Audit logging with database support initialized
✅ Audit middleware enabled for all routes
✅ Audit API endpoints registered at /api/auth/audit/*
```

### Проверка работы

1. Проверьте файл логов:
```bash
tail -f logs/audit.jsonl
```

2. Проверьте БД:
```sql
SELECT COUNT(*) FROM audit_logs;
SELECT * FROM audit_logs ORDER BY timestamp DESC LIMIT 10;
```

3. Проверьте API:
```bash
curl -X GET "http://localhost:8080/api/auth/audit/logs" \
  -H "Authorization: Token YOUR_TOKEN"
```

## Устранение неполадок

### Логи не записываются

1. Проверьте конфигурацию в `.env`
2. Проверьте права на запись в директорию `logs/`
3. Проверьте логи приложения на наличие ошибок инициализации

### Ошибки при записи в БД

1. Проверьте наличие таблицы `audit_logs`
2. Проверьте права пользователя БД на INSERT
3. Проверьте логи приложения

### Медленная работа

1. Проверьте индексы БД
2. Увеличьте размер буфера для записи
3. Рассмотрите отключение `AUDIT_LOG_TO_STDOUT=false`

## Примеры интеграции

### Логирование в сервисах

```go
type UserService struct {
    db *gorm.DB
}

func (s *UserService) CreateUser(c *gin.Context, req *CreateUserRequest) error {
    user := &models.User{...}
    
    if err := s.db.Create(user).Error; err != nil {
        // Логируем ошибку
        audit.LogError(c, "user.create.failed", err, gin.H{
            "username": req.Username,
            "email": req.Email,
        })
        return err
    }
    
    // Логируем успех
    audit.LogSuccess(c, "user.created", gin.H{
        "user_id": user.ID,
        "username": user.Username,
        "role": user.Role.Name,
    })
    
    return nil
}
```

### Логирование системных задач

```go
func RunBackupJob() {
    startTime := time.Now()
    
    // Логируем начало
    audit.Log("system", "backup.started", map[string]interface{}{
        "backup_type": "full",
    })
    
    if err := performBackup(); err != nil {
        // Логируем ошибку с деталями
        audit.LogWithContext(context.Background(), "system", "backup.failed", false, map[string]interface{}{
            "error": err.Error(),
            "duration": time.Since(startTime).Seconds(),
        })
        return
    }
    
    // Логируем успех
    audit.LogWithContext(context.Background(), "system", "backup.completed", true, map[string]interface{}{
        "duration": time.Since(startTime).Seconds(),
    })
}
```

## Дальнейшее развитие

Возможные улучшения:

1. Добавить уровни доступа к логам (только админы могут видеть все логи)
2. Добавить алерты на критические события
3. Добавить экспорт в другие форматы (CSV, Excel)
4. Добавить дашборд с графиками активности
5. Интеграция с внешними системами мониторинга (ELK, Grafana)
6. Добавить автоматическую архивацию старых логов
7. Реализовать поиск по JSONB деталям

## Лицензия

Часть Axenta CRM системы

