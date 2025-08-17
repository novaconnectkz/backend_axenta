# Система управления складом

## Обзор

Система управления складом обеспечивает полный контроль над оборудованием, включая отслеживание остатков, управление перемещениями, мониторинг состояния и автоматические уведомления.

## Основные возможности

### 1. Управление оборудованием

- ✅ CRUD операции для оборудования
- ✅ Отслеживание серийных номеров, IMEI, телефонных номеров
- ✅ Поддержка QR кодов для быстрого поиска
- ✅ Категоризация оборудования
- ✅ Отслеживание местоположения на складе
- ✅ Контроль статусов (в наличии, установлено, на обслуживании, сломано)

### 2. Складские операции

- ✅ Поступление оборудования
- ✅ Выдача оборудования
- ✅ Перемещение между локациями
- ✅ Резервирование оборудования
- ✅ Инвентаризация
- ✅ Списание оборудования

### 3. Интеграция с монтажами

- ✅ Автоматическая привязка оборудования к объектам
- ✅ Отслеживание установленного оборудования
- ✅ Обработка возврата оборудования с объектов

### 4. Система уведомлений

- ✅ Уведомления о низких остатках
- ✅ Уведомления об истекших гарантиях
- ✅ Уведомления о необходимости обслуживания
- ✅ Уведомления о движении оборудования

### 5. Аналитика и отчетность

- ✅ Статистика по оборудованию
- ✅ Анализ складских операций
- ✅ Мониторинг уведомлений
- ✅ История операций с оборудованием

## Модели данных

### Equipment (Оборудование)

```go
type Equipment struct {
    ID                uint
    Type              string    // GPS-tracker, sensor, camera
    Model             string    // Модель устройства
    Brand             string    // Производитель
    SerialNumber      string    // Серийный номер (уникальный)
    IMEI              string    // IMEI для GSM устройств (уникальный)
    PhoneNumber       string    // Номер телефона SIM-карты
    MACAddress        string    // MAC адрес для WiFi устройств
    QRCode            string    // QR код для быстрого поиска (уникальный)
    Status            string    // in_stock, reserved, installed, maintenance, broken, disposed
    Condition         string    // new, used, refurbished, damaged
    ObjectID          *uint     // Связь с объектом (если установлено)
    CategoryID        *uint     // Категория оборудования
    WarehouseLocation string    // Местоположение на складе
    PurchasePrice     decimal   // Закупочная цена
    PurchaseDate      *time.Time // Дата закупки
    WarrantyUntil     *time.Time // Гарантия до
    Specifications    string    // Технические характеристики (JSON)
    Notes             string    // Заметки
    LastMaintenanceAt *time.Time // Последнее обслуживание
}
```

### EquipmentCategory (Категория оборудования)

```go
type EquipmentCategory struct {
    ID            uint
    Name          string  // Название категории (уникальное)
    Description   string  // Описание
    Code          string  // Код категории (уникальный)
    MinStockLevel int     // Минимальный остаток для уведомлений
    IsActive      bool    // Активна ли категория
}
```

### WarehouseOperation (Складская операция)

```go
type WarehouseOperation struct {
    ID             uint
    Type           string  // receive, issue, transfer, inventory, maintenance, disposal
    Description    string  // Описание операции
    Status         string  // pending, completed, cancelled
    EquipmentID    uint    // Связанное оборудование
    Quantity       int     // Количество (для групповых операций)
    FromLocation   string  // Откуда
    ToLocation     string  // Куда
    UserID         uint    // Ответственное лицо
    DocumentNumber string  // Номер документа
    Notes          string  // Заметки
    InstallationID *uint   // Связанная установка (если есть)
}
```

### StockAlert (Складское уведомление)

```go
type StockAlert struct {
    ID                  uint
    Type                string    // low_stock, expired_warranty, maintenance_due
    Title               string    // Заголовок уведомления
    Description         string    // Описание
    Severity            string    // low, medium, high, critical
    EquipmentID         *uint     // Связанное оборудование
    EquipmentCategoryID *uint     // Связанная категория
    Status              string    // active, acknowledged, resolved
    ReadAt              *time.Time // Время прочтения
    ResolvedAt          *time.Time // Время разрешения
    AssignedUserID      *uint     // Ответственное лицо
    Metadata            string    // Дополнительные данные (JSON)
}
```

## API Endpoints

### Оборудование

#### GET /api/equipment

Получение списка оборудования с фильтрацией и пагинацией.

**Параметры запроса:**

- `type` - тип оборудования
- `status` - статус оборудования
- `condition` - состояние оборудования
- `manufacturer` - производитель
- `model` - модель
- `search` - поиск по серийному номеру, IMEI, телефону, модели, QR коду
- `available` - только доступное оборудование (true)
- `needs_maintenance` - требует обслуживания (true)
- `sort_by` - поле сортировки (по умолчанию: created_at)
- `sort_order` - порядок сортировки (asc/desc, по умолчанию: desc)
- `page` - номер страницы (по умолчанию: 1)
- `limit` - количество записей на странице (по умолчанию: 20)

**Пример ответа:**

```json
{
  "data": [
    {
      "id": 1,
      "type": "GPS-tracker",
      "model": "GT06N",
      "brand": "Concox",
      "serial_number": "GT06N001",
      "imei": "123456789012345",
      "phone_number": "+79001234567",
      "qr_code": "EQ-1-GT06N001",
      "status": "in_stock",
      "condition": "new",
      "category": {
        "id": 1,
        "name": "GPS Trackers"
      },
      "warehouse_location": "A1-01",
      "purchase_price": "2500.00",
      "warranty_until": "2025-12-31T00:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 50,
    "pages": 3
  }
}
```

#### GET /api/equipment/:id

Получение информации о конкретном оборудовании.

#### POST /api/equipment

Создание нового оборудования.

**Тело запроса:**

```json
{
  "type": "GPS-tracker",
  "model": "GT06N",
  "brand": "Concox",
  "serial_number": "GT06N001",
  "imei": "123456789012345",
  "phone_number": "+79001234567",
  "qr_code": "EQ-1-GT06N001",
  "category_id": 1,
  "warehouse_location": "A1-01",
  "purchase_price": "2500.00"
}
```

#### PUT /api/equipment/:id

Обновление информации об оборудовании.

#### DELETE /api/equipment/:id

Удаление оборудования (мягкое удаление).

#### PUT /api/equipment/:id/install

Установка оборудования на объект.

**Тело запроса:**

```json
{
  "object_id": 123
}
```

#### PUT /api/equipment/:id/uninstall

Снятие оборудования с объекта.

#### GET /api/equipment/statistics

Получение статистики по оборудованию.

#### GET /api/equipment/low-stock

Получение списка оборудования с низкими остатками.

**Параметры запроса:**

- `threshold` - пороговое значение (по умолчанию: 5)

#### GET /api/equipment/qr/:qr_code

Поиск оборудования по QR коду.

### Категории оборудования

#### GET /api/equipment/categories

Получение списка категорий оборудования.

**Параметры запроса:**

- `active` - только активные категории (true)

#### POST /api/equipment/categories

Создание новой категории оборудования.

**Тело запроса:**

```json
{
  "name": "GPS Trackers",
  "description": "GPS трекеры для мониторинга транспорта",
  "code": "GPS",
  "min_stock_level": 10,
  "is_active": true
}
```

#### PUT /api/equipment/categories/:id

Обновление категории оборудования.

#### DELETE /api/equipment/categories/:id

Удаление категории оборудования.

### Складские операции

#### POST /api/warehouse/operations

Создание складской операции.

**Тело запроса:**

```json
{
  "type": "receive",
  "description": "Поступление нового оборудования",
  "equipment_id": 1,
  "user_id": 1,
  "quantity": 1,
  "to_location": "A1-01",
  "document_number": "DOC-001"
}
```

#### GET /api/warehouse/operations

Получение списка складских операций.

**Параметры запроса:**

- `type` - тип операции
- `equipment_id` - ID оборудования
- `status` - статус операции
- `date_from` - дата начала периода
- `date_to` - дата окончания периода
- `sort_by` - поле сортировки
- `sort_order` - порядок сортировки
- `page` - номер страницы
- `limit` - количество записей

#### POST /api/warehouse/transfer

Перемещение оборудования между локациями.

**Тело запроса:**

```json
{
  "equipment_id": 1,
  "from_location": "A1-01",
  "to_location": "B2-05",
  "user_id": 1,
  "notes": "Перемещение для удобства доступа"
}
```

### Складские уведомления

#### GET /api/warehouse/alerts

Получение списка складских уведомлений.

**Параметры запроса:**

- `type` - тип уведомления
- `status` - статус уведомления
- `severity` - уровень важности
- `include_resolved` - включить разрешенные уведомления (true)

#### POST /api/warehouse/alerts

Создание нового уведомления.

#### PUT /api/warehouse/alerts/:id/acknowledge

Отметка уведомления как прочитанного.

#### PUT /api/warehouse/alerts/:id/resolve

Разрешение уведомления.

### Статистика склада

#### GET /api/warehouse/statistics

Получение статистики склада.

**Пример ответа:**

```json
{
  "data": {
    "total_equipment": 150,
    "in_stock": 120,
    "installed": 25,
    "reserved": 3,
    "maintenance": 2,
    "broken": 0,
    "low_stock_categories": 2,
    "active_alerts": 5,
    "recent_operations": 15,
    "by_category": {
      "GPS Trackers": 100,
      "Sensors": 30,
      "Cameras": 20
    },
    "operations_by_type": {
      "receive": 8,
      "issue": 5,
      "transfer": 2
    },
    "alerts_by_severity": {
      "high": 2,
      "medium": 3
    }
  }
}
```

## Бизнес-логика (WarehouseService)

### Основные методы

#### CheckLowStockLevels()

Проверяет остатки по всем активным категориям и создает уведомления о низких остатках.

#### CheckExpiredWarranties()

Проверяет оборудование с истекшими гарантиями и создает соответствующие уведомления.

#### CheckMaintenanceDue()

Проверяет оборудование, требующее планового обслуживания (не обслуживалось более 6 месяцев).

#### ProcessEquipmentInstallation(equipmentID, objectID, installerID)

Обрабатывает установку оборудования на объект:

- Проверяет доступность оборудования
- Обновляет статус на "installed"
- Привязывает к объекту
- Создает операцию "issue"

#### ProcessEquipmentReturn(equipmentID, userID, warehouseLocation)

Обрабатывает возврат оборудования с объекта:

- Обновляет статус на "in_stock"
- Отвязывает от объекта
- Устанавливает новое местоположение
- Создает операцию "receive"

#### GenerateQRCode(equipmentID)

Генерирует уникальный QR код для оборудования в формате "EQ-{ID}-{SerialNumber}".

#### ReserveEquipment(equipmentID, userID, notes)

Резервирует оборудование для будущей установки.

#### UnreserveEquipment(equipmentID, userID)

Снимает резервирование с оборудования.

#### GetEquipmentHistory(equipmentID)

Возвращает полную историю операций с оборудованием.

### Периодические задачи

Метод `RunPeriodicChecks()` должен запускаться по расписанию для:

- Проверки низких остатков
- Проверки истекших гарантий
- Проверки необходимости обслуживания

## Система уведомлений

Складские уведомления автоматически отправляются соответствующим пользователям через:

- Telegram (если указан telegram_id)
- Email (если указан email)

### Типы уведомлений

1. **low_stock** - Низкий остаток в категории
2. **expired_warranty** - Истекла гарантия на оборудование
3. **maintenance_due** - Требуется плановое обслуживание
4. **equipment_movement** - Движение оборудования

### Роли для уведомлений

- **Низкие остатки**: admin, warehouse_manager
- **Истекшие гарантии**: admin, warehouse_manager, technician
- **Обслуживание**: admin, technician, maintenance_manager
- **Движение оборудования**: admin, warehouse_manager

## Тестирование

Система включает полный набор тестов:

### Модельные тесты (models/warehouse_test.go)

- Тестирование методов моделей
- Проверка валидации данных
- Тестирование связей между моделями
- Проверка уникальности полей

### API тесты (api/warehouse_test.go)

- Тестирование всех endpoints
- Проверка валидации входных данных
- Тестирование фильтрации и пагинации
- Проверка обработки ошибок

### Сервисные тесты (services/warehouse_service_test.go)

- Тестирование бизнес-логики
- Проверка транзакций
- Тестирование уведомлений
- Проверка периодических задач

## Запуск тестов

```bash
# Тесты моделей
go test ./models -v -run TestWarehouse

# Тесты API
go test ./api -v -run TestWarehouse

# Тесты сервисов
go test ./services -v -run TestWarehouseService

# Все тесты
go test ./... -v
```

## Миграции базы данных

При первом запуске система автоматически создаст необходимые таблицы:

- `equipment` - оборудование
- `equipment_categories` - категории оборудования
- `warehouse_operations` - складские операции
- `stock_alerts` - уведомления

## Безопасность

- Все операции требуют аутентификации
- Поддержка мультитенантности
- Валидация входных данных
- Проверка прав доступа
- Логирование всех операций

## Производительность

- Индексы на часто используемые поля
- Пагинация для больших списков
- Кэширование статистики
- Оптимизированные запросы с Preload

## Интеграция с другими модулями

- **Объекты**: Автоматическая привязка оборудования при монтаже
- **Монтажи**: Отслеживание оборудования в процессе установки
- **Пользователи**: Контроль доступа и ответственности
- **Уведомления**: Автоматические алерты по различным событиям

## Заключение

Система управления складом обеспечивает полный контроль над оборудованием компании, автоматизирует рутинные операции и предоставляет детальную аналитику для принятия управленческих решений.
