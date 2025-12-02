# Обновление полей объектов Axenta Cloud

## 📋 Описание изменений

Добавлены новые поля в таблицу `axenta_object_snapshots` для хранения дополнительных данных, которые предоставляет API Axenta Cloud.

### Дата обновления
2 декабря 2025

### Затронутые файлы
- `models/axenta_snapshot.go` - обновлена модель `AxentaObjectSnapshot`
- `services/axenta_sync_service.go` - обновлен код синхронизации для парсинга и сохранения новых полей
- `scripts/migrate_axenta_objects.go` - скрипт для миграции БД

---

## ✅ Новые поля в AxentaObjectSnapshot

| Поле | Тип | Описание | Источник в API |
|------|-----|----------|----------------|
| `creator_name` | `*string` | Имя создателя объекта (ФИО) | `creatorName` |
| `creator_id` | `*int` | ID создателя в Axenta Cloud | `creatorId` |
| `creator_is_active` | `*bool` | Активен ли создатель | `creatorIsActive` |
| `account_is_active` | `*bool` | Активен ли аккаунт | `accountIsActive` |
| `phone_numbers` | `*string` (JSONB) | Массив номеров телефонов | `phoneNumbers` |
| `axenta_created_at` | `*time.Time` | Дата создания объекта в Axenta | `createdAt` |
| `axenta_deleted_at` | `*time.Time` | Дата удаления объекта в Axenta | `deletedAt` |

**Важно:** Все новые поля nullable (указатели) для обратной совместимости с существующими данными.

---

## 🔄 Что изменилось

### 1. Модель данных (`models/axenta_snapshot.go`)

**Было:**
```go
type AxentaObjectSnapshot struct {
    // ... базовые поля
    UniqueID            string     `json:"unique_id"`
    IsActive            bool       `json:"is_active"`
    LastCommunicationAt *time.Time `json:"last_communication_at"`
    RawPayload          string     `json:"raw_payload" gorm:"type:jsonb"`
}
```

**Стало:**
```go
type AxentaObjectSnapshot struct {
    // ... базовые поля (без изменений)
    
    // Новые поля из Axenta Cloud API
    CreatorName     *string    `json:"creator_name" gorm:"type:varchar(200)"`
    CreatorID       *int       `json:"creator_id"`
    CreatorIsActive *bool      `json:"creator_is_active"`
    AccountIsActive *bool      `json:"account_is_active"`
    PhoneNumbers    *string    `json:"phone_numbers" gorm:"type:jsonb"`
    AxentaCreatedAt *time.Time `json:"axenta_created_at"`
    AxentaDeletedAt *time.Time `json:"axenta_deleted_at"`
    
    RawPayload      string     `json:"raw_payload" gorm:"type:jsonb"`
}
```

### 2. Парсинг данных (`services/axenta_sync_service.go`)

**Структура `axentaObject` дополнена:**
```go
type axentaObject struct {
    // ... существующие поля
    
    // Новые поля из API
    CreatorName     string   `json:"creatorName"`
    CreatorID       int      `json:"creatorId"`
    CreatorIsActive bool     `json:"creatorIsActive"`
    AccountIsActive bool     `json:"accountIsActive"`
    PhoneNumbers    []string `json:"phoneNumbers"`
    CreatedAt       string   `json:"createdAt"`
    DeletedAt       string   `json:"deletedAt"`
}
```

**Код сохранения дополнен:**
- Парсинг и сохранение имени создателя
- Парсинг и сохранение ID создателя
- Сохранение флагов активности (создателя и аккаунта)
- Конвертация массива телефонов в JSON
- Парсинг дат создания и удаления из Axenta

---

## 📦 Миграция базы данных

### Автоматическая миграция

Миграция произойдет **автоматически** при следующем запуске приложения благодаря GORM AutoMigrate.

Таблица `axenta_object_snapshots` включена в список миграций в `database/migrations.go`:
```go
{
    TableName:   "axenta_object_snapshots",
    Model:       &models.AxentaObjectSnapshot{},
    Description: "Снимки объектов Axenta",
    IsGlobal:    false, // Тенантная таблица
}
```

### Ручная миграция (опционально)

Для явного запуска миграции используйте скрипт:

```bash
cd /Users/com/backend_axenta
go run scripts/migrate_axenta_objects.go
```

Скрипт выполнит:
1. Добавление новых колонок во все схемы компаний
2. Проверку успешности миграции
3. Вывод отчета о добавленных колонках

---

## 🔐 Безопасность изменений

### ✅ Обратная совместимость гарантирована:

1. **Все новые поля nullable** - не требуют значений в существующих записях
2. **Существующие данные сохранены** - миграция только добавляет колонки
3. **Никакие поля не удалены** - старый код продолжит работать
4. **Индексы не изменены** - производительность не пострадает
5. **Raw payload сохранен** - полные данные API по-прежнему доступны

### ✅ Тестирование:

- Линтер: ✅ Ошибок нет
- Backward compatibility: ✅ Гарантирована
- Data integrity: ✅ Сохранена

---

## 📊 Использование новых данных

### Примеры запросов

**Поиск объектов по создателю:**
```go
var snapshots []models.AxentaObjectSnapshot
db.Where("creator_name LIKE ?", "%Иванов%").Find(&snapshots)
```

**Фильтр по дате создания в Axenta:**
```go
var snapshots []models.AxentaObjectSnapshot
db.Where("axenta_created_at >= ?", time.Now().AddDate(0, -1, 0)).Find(&snapshots)
```

**Получение объектов с телефонами:**
```go
var snapshots []models.AxentaObjectSnapshot
db.Where("phone_numbers IS NOT NULL").Find(&snapshots)

// Парсинг JSON массива телефонов
if snapshot.PhoneNumbers != nil {
    var phones []string
    json.Unmarshal([]byte(*snapshot.PhoneNumbers), &phones)
    // phones = ["79991234567", "79997654321"]
}
```

**Поиск удаленных объектов:**
```go
var snapshots []models.AxentaObjectSnapshot
db.Where("axenta_deleted_at IS NOT NULL").Find(&snapshots)
```

---

## 🎯 Что дальше?

После внедрения изменений:

1. **Следующая синхронизация** автоматически заполнит новые поля для всех объектов
2. **Старые записи** останутся с NULL значениями до следующей синхронизации
3. **Новые записи** будут содержать полные данные из API

### Мониторинг

Проверить заполнение новых полей:
```sql
-- Сколько объектов имеют данные о создателе
SELECT COUNT(*) FROM axenta_object_snapshots WHERE creator_name IS NOT NULL;

-- Сколько объектов имеют дату создания в Axenta
SELECT COUNT(*) FROM axenta_object_snapshots WHERE axenta_created_at IS NOT NULL;

-- Сколько объектов имеют телефоны
SELECT COUNT(*) FROM axenta_object_snapshots WHERE phone_numbers IS NOT NULL;
```

---

## 📝 Итоговая таблица доступности данных

| Данные | API предоставляет | Парсим | Сохраняем отдельно | В raw_payload |
|--------|-------------------|--------|-------------------|---------------|
| **ID объекта** | ✅ | ✅ | ✅ | ✅ |
| **Название** | ✅ | ✅ | ✅ | ✅ |
| **Уникальный ID (IMEI)** | ✅ | ✅ | ✅ | ✅ |
| **Создатель (ФИО)** | ✅ | ✅ | ✅ NEW | ✅ |
| **ID создателя** | ✅ | ✅ | ✅ NEW | ✅ |
| **Дата создания в Axenta** | ✅ | ✅ | ✅ NEW | ✅ |
| **Дата удаления в Axenta** | ✅ | ✅ | ✅ NEW | ✅ |
| **Номера телефонов** | ✅ | ✅ | ✅ NEW | ✅ |
| **Активность создателя** | ✅ | ✅ | ✅ NEW | ✅ |
| **Активность аккаунта** | ✅ | ✅ | ✅ NEW | ✅ |
| **Дата последней связи** | ✅ | ✅ | ✅ | ✅ |
| **Статус активности** | ✅ | ✅ | ✅ | ✅ |

---

## 👤 Автор
Обновление выполнено в рамках улучшения интеграции с Axenta Cloud API.

## 📅 История изменений
- **02.12.2025** - Добавлены новые поля из Axenta Cloud API

