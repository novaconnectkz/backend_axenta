# Быстрая проверка записи стартовой даты биллинга

## Проблема
Запись от 01/01/2025 не отображается в истории снимков на фронтенде.

## Быстрая диагностика

### Шаг 1: Создайте запись
```bash
GET /api/auth/snapshots/billing-start-date
```

**Проверьте логи сервера** - должны быть сообщения:
- `✅ Стартовая дата биллинга записана в историю снимков`
- `✅ Запись подтверждена в БД`

### Шаг 2: Проверьте наличие записи
```bash
GET /api/auth/snapshots/check-billing-start-in-history
```

**Ожидаемый ответ:**
```json
{
  "status": "found",
  "count": 1,
  "jobs": [{
    "id": 123,
    "job_type": "billing_start",
    "date_from": "2025-01-01",
    ...
  }]
}
```

### Шаг 3: Проверьте через основной API
```bash
GET /api/auth/snapshot-jobs
```

**Проверьте:**
- Есть ли запись с `job_type: "billing_start"` в ответе?
- Если нет - проверьте логи сервера на ошибки

### Шаг 4: Проверьте с фильтром
```bash
GET /api/auth/snapshot-jobs?job_type=billing_start
```

**Если запись есть в API, но не видна на фронтенде:**
- Возможно, фронтенд фильтрует записи по типу
- Проверьте фильтры на фронтенде
- Убедитесь, что фронтенд показывает все типы задач

## Возможные причины

1. **Запись не создается:**
   - Проверьте логи на ошибки
   - Убедитесь, что таблица `snapshot_jobs` существует в схеме `public`

2. **Запись создается, но не видна в API:**
   - Проверьте, что используется правильная схема (`public`)
   - Проверьте фильтры в API

3. **Запись видна в API, но не на фронтенде:**
   - Фронтенд может фильтровать по типу `job_type`
   - Проверьте фильтры на фронтенде
   - Убедитесь, что фронтенд поддерживает тип `billing_start`

## SQL проверка

Если нужно проверить напрямую в БД:
```sql
-- Проверить все записи billing_start
SELECT id, job_type, date_from, date_to, status, total_objects, active_objects, started_at
FROM public.snapshot_jobs
WHERE job_type = 'billing_start'
ORDER BY started_at DESC;

-- Проверить все записи (последние 10)
SELECT id, job_type, date_from, status, started_at
FROM public.snapshot_jobs
ORDER BY started_at DESC
LIMIT 10;
```
