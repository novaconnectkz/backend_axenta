# Диагностика проблемы с записью стартовой даты биллинга

## Проблема
Запись о стартовой дате биллинга (01/01/2025) не отображается в истории автоматических снимков.

## Шаги диагностики

### 1. Проверка создания записи

Вызовите endpoint для определения стартовой даты:
```bash
GET /api/auth/snapshots/billing-start-date
```

**Проверьте логи сервера** - должны быть сообщения:
- `📅 Определяем стартовую дату биллинга из БД...`
- `✅ Найден самый первый объект в БД:`
- `📝 Сохранение стартовой даты биллинга в историю снимков (схема public)...`
- `✅ Стартовая дата биллинга записана в историю снимков (ID: X, дата: ...)`

### 2. Проверка наличия записи в БД

Вызовите специальный endpoint для проверки:
```bash
GET /api/auth/snapshots/check-billing-start-in-history
```

**Ожидаемый ответ:**
```json
{
  "status": "found",
  "count": 1,
  "jobs": [
    {
      "id": 123,
      "job_type": "billing_start",
      "date_from": "2025-01-01",
      "date_to": "2025-01-01",
      "status": "completed",
      "total_objects": 150,
      "active_objects": 120
    }
  ]
}
```

### 3. Проверка через API истории снимков

```bash
GET /api/auth/snapshot-jobs?job_type=billing_start
```

Или без фильтра (должна быть видна среди всех записей):
```bash
GET /api/auth/snapshot-jobs
```

### 4. Возможные проблемы и решения

#### Проблема: Запись не создается

**Причины:**
- Ошибка при переключении на схему `public`
- Таблица `snapshot_jobs` не существует в схеме `public`
- Ошибка при создании записи (проверьте логи)

**Решение:**
1. Проверьте логи сервера на наличие ошибок
2. Убедитесь, что таблица `snapshot_jobs` существует:
   ```sql
   SELECT EXISTS (
     SELECT FROM information_schema.tables 
     WHERE table_schema = 'public' 
     AND table_name = 'snapshot_jobs'
   );
   ```

#### Проблема: Запись создается, но не отображается

**Причины:**
- Фронтенд фильтрует записи по типу (исключает `billing_start`)
- Пагинация скрывает запись
- Проблема с сортировкой (запись в конце списка)

**Решение:**
1. Проверьте через API напрямую: `GET /api/auth/snapshot-jobs?job_type=billing_start`
2. Проверьте фильтры на фронтенде
3. Увеличьте лимит пагинации: `GET /api/auth/snapshot-jobs?limit=100`

#### Проблема: Запись создается в неправильной схеме

**Признаки:**
- Запись не видна через API `GetSnapshotJobs`
- Логи показывают успешное создание, но запись не находится

**Решение:**
- Убедитесь, что используется `database.DB` с установкой `SET search_path TO public`
- Проверьте, что запись создается в правильной схеме:
  ```sql
  SELECT * FROM public.snapshot_jobs WHERE job_type = 'billing_start';
  ```

## Технические детали

### Где создается запись
- **Схема:** `public`
- **Таблица:** `snapshot_jobs`
- **Тип:** `billing_start`
- **Статус:** `completed`

### Структура записи
```go
{
  JobType: "billing_start",
  DateFrom: billingStartDate,  // 2025-01-01
  DateTo: billingStartDate,     // 2025-01-01 (одинаковые)
  Status: "completed",
  TotalObjects: totalObjects,
  ActiveObjects: activeObjects,
  SuccessCount: 1,
  TotalDaysProcessed: 1,
  TriggeredBy: "system"
}
```

### Логика создания
1. Определяется самый первый объект в БД по `axenta_created_at`
2. Подсчитывается количество объектов на эту дату
3. Создается запись в таблице `snapshot_jobs` в схеме `public`
4. Запись сразу помечается как `completed`

## Команды для проверки

### Проверка через SQL
```sql
-- Проверить наличие записи
SELECT id, job_type, date_from, date_to, status, total_objects, active_objects, started_at
FROM public.snapshot_jobs
WHERE job_type = 'billing_start'
ORDER BY started_at DESC;

-- Проверить все записи
SELECT id, job_type, date_from, status, started_at
FROM public.snapshot_jobs
ORDER BY started_at DESC
LIMIT 10;
```

### Проверка через API
```bash
# Проверка создания записи
curl -X GET "http://localhost:8080/api/auth/snapshots/billing-start-date" \
  -H "Authorization: Bearer YOUR_TOKEN"

# Проверка наличия записи
curl -X GET "http://localhost:8080/api/auth/snapshots/check-billing-start-in-history" \
  -H "Authorization: Bearer YOUR_TOKEN"

# Получение всех записей billing_start
curl -X GET "http://localhost:8080/api/auth/snapshot-jobs?job_type=billing_start" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Если проблема не решена

1. Проверьте все логи сервера при вызове `billing-start-date`
2. Проверьте, что таблица `snapshot_jobs` существует и доступна
3. Проверьте права доступа к схеме `public`
4. Убедитесь, что используется правильная БД (не тестовая)
