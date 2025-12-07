# Новый подход к процессу загрузки снимков

## Описание

Реализован новый накопительный подход к загрузке снимков объектов из Axenta API, который оптимизирует процесс загрузки и хранения данных.

## Основные принципы

### 1. Разовая загрузка всех текущих объектов
При первом запуске система загружает все текущие объекты из Axenta в БД. Это выполняется один раз для инициализации системы.

**Endpoint:** `POST /api/auth/snapshots/load-all-current`

### 2. Определение стартовой даты биллинга
Система автоматически находит самый первый созданный объект в БД и использует дату его создания (`axenta_created_at`) как стартовую дату биллинга.

**Endpoint:** `GET /api/auth/snapshots/billing-start-date`

**Логика поиска:**
1. Ищет объекты с `axenta_created_at IS NOT NULL`
2. Исключает удаленные объекты (`axenta_deleted_at IS NULL OR axenta_deleted_at > axenta_created_at`)
3. Сортирует по `axenta_created_at ASC` и берет первый
4. Округляет дату до начала дня (00:00:00 UTC)
5. **Автоматически записывает результат в таблицу истории автоматических снимков** (`snapshot_jobs`) с типом `billing_start`

**Ответ:**
```json
{
  "status": "success",
  "start_date": "2025-01-01",
  "start_date_iso": "2025-01-01T00:00:00Z",
  "earliest_object": {
    "id": 12345,
    "name": "Объект 1",
    "created_at": "2025-01-01T10:30:00Z",
    "created_at_date": "2025-01-01"
  }
}
```

**Запись в историю:**
При определении стартовой даты система автоматически:
- Подсчитывает количество объектов на эту дату (всего и активных)
- Создает запись в таблице `snapshot_jobs` с типом `billing_start`
- Сохраняет дату (`date_from` и `date_to` одинаковые)
- Сохраняет количество объектов (`total_objects`, `active_objects`)
- Добавляет информацию о первом объекте в детали

Если объекты не найдены, возвращается текущая дата.

### 3. Ежедневное накопление
Каждый день система добавляет только новые объекты, созданные в этот день. Это значительно уменьшает объем загружаемых данных и ускоряет процесс.

**Endpoint:** `POST /api/auth/snapshots/daily-accumulation`

### 4. Автоматическое суммирование
Система автоматически вычисляет общее количество объектов на каждую дату, учитывая:
- Объекты, созданные до или в этот день (`axenta_created_at <= дата`)
- Объекты, которые не были удалены или были удалены после этой даты (`axenta_deleted_at IS NULL OR axenta_deleted_at > дата`)

**Endpoint:** `GET /api/auth/snapshots/objects-count?date=2025-01-01`

**Ответ:**
```json
{
  "status": "success",
  "date": "2025-01-01",
  "total_objects": 150,
  "active_objects": 120,
  "inactive_objects": 30
}
```

## Использование

### Инициализация системы

1. **Загрузите все текущие объекты:**
```bash
curl -X POST http://localhost:8080/api/auth/snapshots/load-all-current \
  -H "Authorization: Bearer YOUR_TOKEN"
```

2. **Получите стартовую дату биллинга:**
```bash
curl http://localhost:8080/api/auth/snapshots/billing-start-date \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Ежедневное использование

**Запуск ежедневного накопления:**
```bash
curl -X POST http://localhost:8080/api/auth/snapshots/daily-accumulation \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Этот endpoint можно вызывать ежедневно (например, через cron) для добавления новых объектов.

**Получение количества объектов на дату:**
```bash
curl "http://localhost:8080/api/auth/snapshots/objects-count?date=2025-01-15" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Технические детали

### Сервис: `SnapshotAccumulationService`

Основной сервис находится в `services/snapshot_accumulation_service.go` и предоставляет следующие методы:

- `LoadAllCurrentObjects(token string)` - разовая загрузка всех объектов
- `GetBillingStartDate(tenantDB *gorm.DB)` - определение стартовой даты
- `LoadNewObjectsForDate(date time.Time, token string, tenantDB *gorm.DB)` - загрузка новых объектов за день
- `CalculateObjectsCountForDate(date time.Time, tenantDB *gorm.DB)` - подсчет объектов на дату
- `ProcessDailyAccumulation(token string, tenantDB *gorm.DB)` - обработка ежедневного накопления

### Хранение данных

Объекты хранятся в таблице `axenta_object_snapshots` с полями:
- `axenta_created_at` - дата создания объекта в Axenta
- `axenta_deleted_at` - дата удаления объекта в Axenta (если удален)
- `is_active` - статус активности объекта

### Оптимизация

Новый подход обеспечивает:
- **Меньший объем данных:** загружаются только новые объекты каждый день
- **Быстрая работа:** не нужно загружать все объекты каждый раз
- **Точность:** автоматический подсчет объектов на любую дату
- **Масштабируемость:** система эффективно работает с большим количеством объектов

## Миграция со старого подхода

Если вы используете старый подход к загрузке снимков, рекомендуется:

1. Выполнить разовую загрузку всех текущих объектов
2. Определить стартовую дату биллинга
3. Настроить ежедневный запуск `daily-accumulation` (через cron или планировщик)
4. Использовать `objects-count` для получения количества объектов на нужную дату

## Автоматизация

Рекомендуется настроить автоматический запуск ежедневного накопления:

```bash
# Cron задача (каждый день в 00:00 UTC)
0 0 * * * curl -X POST http://localhost:8080/api/auth/snapshots/daily-accumulation -H "Authorization: Bearer YOUR_TOKEN"
```

Или использовать встроенный планировщик Go для автоматического запуска.
