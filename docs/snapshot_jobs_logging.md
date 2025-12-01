# Логирование автоматических снимков партнерских объектов

## Обзор

Система автоматически создает снимки партнерских объектов каждый день в 00:00 UTC и записывает полную информацию о каждом запуске в базу данных.

## Таблица `snapshot_jobs`

Хранит историю всех запусков создания снимков.

### Основные поля

| Поле | Тип | Описание |
|------|-----|----------|
| `id` | BIGSERIAL | Уникальный ID задачи |
| `job_type` | VARCHAR(50) | Тип задачи: `daily_auto`, `manual`, `scheduled` |
| `status` | VARCHAR(20) | Статус: `running`, `completed`, `failed`, `partial` |
| `started_at` | TIMESTAMP | Дата и время начала |
| `finished_at` | TIMESTAMP | Дата и время окончания |
| `duration_seconds` | INTEGER | Длительность выполнения в секундах |

### Статистика

| Поле | Описание |
|------|----------|
| `total_companies` | Количество обработанных компаний (тенантов) |
| `total_contracts` | Количество обработанных договоров |
| `total_days_processed` | Количество успешно обработанных дней |
| `success_count` | Количество успешно созданных снимков |
| `error_count` | Количество ошибок |

### Детали (JSONB)

Поле `details` содержит подробную информацию:

```json
{
  "companies": [
    {
      "company_id": 1,
      "company_name": "tenant_186",
      "contracts_count": 5,
      "success_count": 4,
      "error_count": 1,
      "processing_time_s": 12
    }
  ],
  "contracts": [
    {
      "contract_id": 96,
      "contract_number": "ДП-2024-001",
      "company_id": 1,
      "days_processed": 1,
      "success_count": 1,
      "error_count": 0,
      "processing_time_s": 2
    }
  ],
  "errors": [
    {
      "timestamp": "2024-12-01T10:30:00Z",
      "company_id": 1,
      "contract_id": 97,
      "date": "2024-11-30",
      "message": "Не удалось получить токен",
      "error_type": "api_error",
      "recoverable": true
    }
  ]
}
```

## API Endpoints

### 1. Получить список задач

```
GET /api/auth/snapshot-jobs?limit=50&offset=0&status=completed
```

**Параметры:**
- `limit` (опционально) - количество записей (по умолчанию 50)
- `offset` (опционально) - смещение для пагинации
- `status` (опционально) - фильтр по статусу
- `job_type` (опционально) - фильтр по типу задачи

**Ответ:**
```json
{
  "jobs": [...],
  "total": 100,
  "limit": 50,
  "offset": 0
}
```

### 2. Получить детали задачи

```
GET /api/auth/snapshot-jobs/:id
```

**Ответ:** Полная информация о задаче со всеми деталями.

### 3. Получить статистику

```
GET /api/auth/snapshot-jobs/stats
```

**Ответ:**
```json
{
  "total_jobs": 100,
  "completed_jobs": 95,
  "failed_jobs": 2,
  "partial_jobs": 3,
  "running_jobs": 0,
  "total_snapshots": 5432,
  "total_errors": 23,
  "avg_duration_s": 45.2,
  "last_job_started_at": "2024-12-01 00:00:00"
}
```

### 4. Получить последнюю задачу

```
GET /api/auth/snapshot-jobs/latest
```

### 5. Очистить старые записи

```
DELETE /api/auth/snapshot-jobs/cleanup?days=90
```

Удаляет записи старше указанного количества дней (по умолчанию 90).

## Просмотр в UI

Создан компонент `SnapshotJobsHistory.vue` для просмотра истории в админке.

### Возможности:

1. **Статистика в карточках** - общая информация о задачах
2. **Таблица задач** с пагинацией и сортировкой
3. **Детальный просмотр** каждой задачи в диалоге
4. **Список ошибок** для проблемных задач
5. **Обновление в реальном времени**

### Использование:

```vue
<template>
  <SnapshotJobsHistory />
</template>

<script setup>
import SnapshotJobsHistory from '@/components/Admin/SnapshotJobsHistory.vue';
</script>
```

## Типы статусов

| Статус | Описание | Когда устанавливается |
|--------|----------|----------------------|
| `running` | Выполняется | При старте задачи |
| `completed` | Успешно завершено | Когда `error_count == 0` |
| `partial` | Частично выполнено | Когда `error_count > 0` и `success_count > 0` |
| `failed` | Ошибка | Когда `success_count == 0` |

## Типы задач

| Тип | Описание | Кто запускает |
|-----|----------|---------------|
| `daily_auto` | Автоматическое ежедневное создание | Cron планировщик в 00:00 UTC |
| `manual` | Ручное создание через UI | Пользователь |
| `scheduled` | По расписанию | Планировщик (будущая функция) |

## Типы ошибок

| Тип | Описание | Recoverable |
|-----|----------|-------------|
| `api_error` | Ошибка API Axenta | ✅ true |
| `db_error` | Ошибка БД | ❌ false |
| `validation_error` | Ошибка валидации данных | ✅ true |

## Мониторинг

### Проверка последней задачи

```bash
curl -X GET "http://localhost:8080/api/auth/snapshot-jobs/latest" \
  -H "Authorization: Token YOUR_TOKEN" \
  -H "X-Tenant-ID: 1"
```

### Проверка статистики

```sql
SELECT 
  status,
  COUNT(*) as count,
  AVG(duration_seconds) as avg_duration,
  SUM(success_count) as total_snapshots,
  SUM(error_count) as total_errors
FROM snapshot_jobs
WHERE started_at > NOW() - INTERVAL '30 days'
GROUP BY status;
```

### Поиск проблемных задач

```sql
SELECT 
  id,
  started_at,
  status,
  error_count,
  error_message
FROM snapshot_jobs
WHERE error_count > 0
ORDER BY started_at DESC
LIMIT 10;
```

## Очистка старых логов

Рекомендуется периодически удалять старые записи для оптимизации БД:

```bash
# Удалить записи старше 90 дней
curl -X DELETE "http://localhost:8080/api/auth/snapshot-jobs/cleanup?days=90" \
  -H "Authorization: Token YOUR_TOKEN" \
  -H "X-Tenant-ID: 1"
```

Или настроить автоматическую очистку через cron:

```sql
-- Создать функцию для очистки
CREATE OR REPLACE FUNCTION cleanup_old_snapshot_jobs()
RETURNS void AS $$
BEGIN
  DELETE FROM snapshot_jobs
  WHERE started_at < NOW() - INTERVAL '90 days';
END;
$$ LANGUAGE plpgsql;

-- Создать cron задачу (если используется pg_cron)
SELECT cron.schedule('cleanup-snapshot-jobs', '0 2 * * 0', 'SELECT cleanup_old_snapshot_jobs()');
```

## Примеры использования

### Получить все неудачные задачи за последние 7 дней

```javascript
const response = await axios.get(`${config.apiBaseUrl}/auth/snapshot-jobs`, {
  params: {
    status: 'failed',
    limit: 100
  },
  headers: {
    'Authorization': `Token ${token}`,
    'X-Tenant-ID': tenantId
  }
});
```

### Проверить детали конкретной задачи

```javascript
const jobId = 123;
const response = await axios.get(`${config.apiBaseUrl}/auth/snapshot-jobs/${jobId}`, {
  headers: {
    'Authorization': `Token ${token}`,
    'X-Tenant-ID': tenantId
  }
});

console.log('Ошибки:', response.data.details.errors);
console.log('Статистика:', {
  успешно: response.data.success_count,
  ошибок: response.data.error_count,
  длительность: response.data.duration_seconds
});
```

## Troubleshooting

### Задача зависла в статусе "running"

Если задача долго находится в статусе `running`, возможно:
1. Процесс был прерван
2. Произошел сбой сервера
3. Очень большое количество договоров

**Решение:** Проверить логи сервера и при необходимости вручную обновить статус:

```sql
UPDATE snapshot_jobs
SET status = 'failed',
    error_message = 'Прервано вручную',
    finished_at = NOW()
WHERE id = :job_id AND status = 'running';
```

### Высокий процент ошибок

Проверить типы ошибок:

```sql
SELECT 
  (details->'errors'->0->>'error_type') as error_type,
  COUNT(*) as count
FROM snapshot_jobs
WHERE error_count > 0
GROUP BY error_type;
```

## Архитектура

```
┌─────────────────┐
│  Cron Scheduler │ (00:00 UTC daily)
└────────┬────────┘
         │
         ▼
┌─────────────────────────┐
│ PartnerSnapshotScheduler│
│  - createDailySnapshots()│
└────────┬────────────────┘
         │
         ▼
┌─────────────────────┐
│  SnapshotJob (DB)   │ ← Создается запись
│  status: running    │
└────────┬────────────┘
         │
         ▼ (для каждой компании)
┌─────────────────────┐
│  GetTenantDBByID()  │
└────────┬────────────┘
         │
         ▼ (для каждого договора)
┌─────────────────────────────┐
│ CreateSnapshotForContract() │
└────────┬────────────────────┘
         │
         ▼
┌─────────────────────┐
│  SnapshotJob (DB)   │ ← Обновляется статистика
│  - AddError()       │
│  - AddContractDetail│
└────────┬────────────┘
         │
         ▼ (по завершении)
┌─────────────────────┐
│  SnapshotJob (DB)   │ ← Финализация
│  status: completed  │
│  duration_seconds   │
└─────────────────────┘
```

## Миграция

Для применения изменений выполните миграцию:

```bash
psql -h localhost -U postgres -d axenta_db -f migrations/add_snapshot_jobs_table.sql
```

