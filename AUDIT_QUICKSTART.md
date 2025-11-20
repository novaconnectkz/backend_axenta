# Быстрый старт системы аудит-логирования

## Запуск

### 1. Настройка переменных окружения

Добавьте в `.env`:

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

### 2. Запуск backend

```bash
cd backend_axenta
go run main.go
```

При запуске вы увидите:
```
✅ Audit logs table created/verified
✅ Audit logging with database support initialized
✅ Audit middleware enabled for all routes
✅ Audit API endpoints registered at /api/auth/audit/*
```

### 3. Запуск frontend

```bash
cd frontend_axenta
npm run dev
```

### 4. Доступ к интерфейсу

Откройте в браузере: `http://localhost:3000/settings/audit-logs`

## Примеры использования

### Backend - Добавление логирования в код

```go
import "backend_axenta/audit"

// В любом handler
func CreateObject(c *gin.Context) {
    // ... ваш код создания объекта ...
    
    if err != nil {
        // Логируем ошибку
        audit.LogError(c, "object.create.failed", err, gin.H{
            "object_name": req.Name,
            "reason": "validation_error",
        })
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // Логируем успех
    audit.LogSuccess(c, "object.created", gin.H{
        "object_id": object.ID,
        "object_name": object.Name,
    })
    
    c.JSON(200, gin.H{"data": object})
}
```

### Просмотр логов через API

```bash
# Получить последние 10 логов
curl -X GET "http://localhost:8080/api/auth/audit/logs?per_page=10" \
  -H "Authorization: Token YOUR_TOKEN"

# Получить только ошибки за последние 3 дня
curl -X GET "http://localhost:8080/api/auth/audit/logs?level=error&start_date=2025-11-17T00:00:00Z" \
  -H "Authorization: Token YOUR_TOKEN"

# Поиск по действию
curl -X GET "http://localhost:8080/api/auth/audit/logs?search=login" \
  -H "Authorization: Token YOUR_TOKEN"

# Статистика за 30 дней
curl -X GET "http://localhost:8080/api/auth/audit/stats?days=30" \
  -H "Authorization: Token YOUR_TOKEN"
```

### Просмотр логов в файле

```bash
# Последние 20 записей
tail -n 20 logs/audit.jsonl

# В реальном времени
tail -f logs/audit.jsonl

# С форматированием
tail -f logs/audit.jsonl | jq '.'
```

### Просмотр логов в БД

```sql
-- Последние 10 записей
SELECT * FROM audit_logs 
ORDER BY timestamp DESC 
LIMIT 10;

-- Все ошибки за сегодня
SELECT * FROM audit_logs 
WHERE DATE(timestamp) = CURRENT_DATE 
  AND success = false;

-- Действия конкретного пользователя
SELECT action, timestamp, success 
FROM audit_logs 
WHERE user_id = '123'
ORDER BY timestamp DESC;

-- Статистика по действиям
SELECT action, COUNT(*) as count 
FROM audit_logs 
WHERE timestamp > NOW() - INTERVAL '7 days'
GROUP BY action 
ORDER BY count DESC;
```

## Проверка работы

1. Войдите в систему через frontend
2. Выполните несколько действий (создание объекта, редактирование)
3. Перейдите в **Настройки** → **Аудит действий пользователей**
4. Увидите все свои действия в таблице

## Структура логов

Каждая запись содержит:

```json
{
  "timestamp": "2025-11-20T10:30:45Z",
  "user_id": "123",
  "username": "ivan.ivanov",
  "role": "partner",
  "tenant_id": "1",
  "ip": "192.168.1.100",
  "user_agent": "Mozilla/5.0...",
  "action": "object.created",
  "method": "POST",
  "path": "/api/auth/objects",
  "status_code": 200,
  "details": {
    "object_id": 456,
    "object_name": "Новый объект",
    "object_type": "vehicle"
  },
  "success": true,
  "level": "success",
  "duration_ms": 45
}
```

## Основные действия в логах

| Действие | Описание |
|----------|----------|
| `auth.login.attempt` | Попытка входа |
| `auth.login.success` | Успешный вход |
| `auth.login.failed` | Неудачная попытка входа |
| `auth.logout` | Выход из системы |
| `object.created` | Создание объекта |
| `object.updated` | Обновление объекта |
| `object.deleted` | Удаление объекта |
| `user.created` | Создание пользователя |
| `user.updated` | Обновление пользователя |
| `user.deleted` | Удаление пользователя |
| `settings.updated` | Изменение настроек |
| `data.export` | Экспорт данных |

## Уровни логов

- **info** - Информационные сообщения
- **success** - Успешные операции
- **warning** - Предупреждения
- **error** - Ошибки

## Устранение проблем

### Логи не появляются в интерфейсе

1. Проверьте авторизацию
2. Проверьте что `AUDIT_ENABLED=true`
3. Откройте консоль браузера на наличие ошибок
4. Проверьте что backend запущен

### Ошибка при запуске backend

1. Проверьте подключение к БД
2. Проверьте наличие директории `logs/`
3. Проверьте права на запись

### Пустая таблица в БД

1. Выполните несколько действий в системе
2. Проверьте что миграция применена:
   ```sql
   SELECT * FROM information_schema.tables WHERE table_name = 'audit_logs';
   ```

## Дополнительная информация

См. полную документацию в `AUDIT_SYSTEM.md`

