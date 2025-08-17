# Система планирования монтажей

Модуль системы планирования монтажей предоставляет полный набор функций для управления монтажами, монтажниками, локациями и оборудованием.

## Возможности

### 🔧 Управление монтажами

- **CRUD операции** для монтажей и диагностики
- **Планирование** с учетом доступности монтажников
- **Отслеживание статуса** выполнения работ (запланирован, выполняется, завершен, отменен)
- **Система приоритетов** (низкий, обычный, высокий, срочный)
- **Автоматическая проверка конфликтов** в расписании
- **Контроль максимального количества** монтажей в день для каждого монтажника

### 👷 Управление монтажниками

- **Полная информация** о монтажниках (ФИО, контакты, специализация)
- **Типы монтажников**: штатные, наемные, партнеры
- **Рабочие параметры**: график работы, максимальное количество монтажей в день
- **Специализация и навыки**: GPS-трекеры, сигнализации, видеонаблюдение и др.
- **Географические ограничения**: города и регионы работы
- **Статус доступности**: доступен, занят, отпуск, больничный

### 📍 Управление локациями

- **Справочник городов** и регионов
- **Географические координаты** для точного позиционирования
- **Временные зоны** для корректного планирования
- **Группировка по регионам** для оптимизации маршрутов
- **Поиск локаций** по различным критериям

### 📦 Управление оборудованием

- **Складской учет** оборудования
- **Отслеживание серийных номеров**, IMEI, номеров телефонов
- **Статусы оборудования**: на складе, зарезервировано, установлено, на обслуживании
- **Состояние оборудования**: новое, б/у, восстановленное, поврежденное
- **Связь с монтажами** через many-to-many отношения
- **Контроль остатков** и уведомления о низких запасах

### 🔔 Система уведомлений

- **Напоминания о предстоящих монтажах** (за день до выполнения)
- **Уведомления монтажников** о новых/измененных заданиях
- **Уведомления клиентов** о статусе работ
- **Поддержка каналов**: SMS, Email, Telegram (расширяемо)
- **Логирование всех уведомлений** с отслеживанием статуса доставки

## API Endpoints

### Монтажи (`/api/installations`)

```http
GET    /api/installations              # Список монтажей с фильтрацией
GET    /api/installations/:id          # Информация о монтаже
POST   /api/installations              # Создание нового монтажа
PUT    /api/installations/:id          # Обновление монтажа
DELETE /api/installations/:id          # Удаление монтажа

PUT    /api/installations/:id/start    # Начало выполнения
PUT    /api/installations/:id/complete # Завершение монтажа
PUT    /api/installations/:id/cancel   # Отмена монтажа

GET    /api/installations/statistics   # Статистика по монтажам
```

**Параметры фильтрации:**

- `status` - статус монтажа
- `installer_id` - ID монтажника
- `object_id` - ID объекта
- `type` - тип работы
- `date_from`, `date_to` - период
- `page`, `limit` - пагинация

### Монтажники (`/api/installers`)

```http
GET    /api/installers                 # Список монтажников
GET    /api/installers/:id             # Информация о монтажнике
POST   /api/installers                 # Создание монтажника
PUT    /api/installers/:id             # Обновление информации
DELETE /api/installers/:id             # Удаление монтажника

PUT    /api/installers/:id/activate    # Активация
PUT    /api/installers/:id/deactivate  # Деактивация

GET    /api/installers/:installer_id/schedule  # Расписание монтажника
GET    /api/installers/:id/workload            # Загруженность
GET    /api/installers/available              # Доступные монтажники
GET    /api/installers/statistics            # Статистика
```

**Параметры для поиска доступных монтажников:**

- `date` - дата монтажа (обязательно)
- `location_id` - ID локации
- `specialization` - требуемая специализация
- `duration` - продолжительность работы в минутах

### Локации (`/api/locations`)

```http
GET    /api/locations                  # Список локаций
GET    /api/locations/:id              # Информация о локации
POST   /api/locations                  # Создание локации
PUT    /api/locations/:id              # Обновление локации
DELETE /api/locations/:id              # Удаление локации

GET    /api/locations/statistics       # Статистика по локациям
GET    /api/locations/by-region        # Группировка по регионам
GET    /api/locations/search           # Поиск локаций
```

### Оборудование (`/api/equipment`)

```http
GET    /api/equipment                  # Список оборудования
GET    /api/equipment/:id              # Информация об оборудовании
POST   /api/equipment                  # Добавление оборудования
PUT    /api/equipment/:id              # Обновление информации
DELETE /api/equipment/:id              # Удаление оборудования

PUT    /api/equipment/:id/install      # Установка на объект
PUT    /api/equipment/:id/uninstall    # Снятие с объекта

GET    /api/equipment/statistics       # Статистика по оборудованию
GET    /api/equipment/low-stock        # Оборудование с низкими остатками
GET    /api/equipment/qr/:qr_code      # Поиск по QR коду
```

## Модели данных

### Installation (Монтаж)

```go
type Installation struct {
    ID                uint      `json:"id"`
    Type              string    `json:"type"`              // монтаж, диагностика, демонтаж, обслуживание
    Status            string    `json:"status"`            // planned, in_progress, completed, cancelled
    Priority          string    `json:"priority"`          // low, normal, high, urgent
    ScheduledAt       time.Time `json:"scheduled_at"`      // Запланированное время
    EstimatedDuration int       `json:"estimated_duration"` // Оценочное время в минутах
    ObjectID          uint      `json:"object_id"`
    InstallerID       uint      `json:"installer_id"`
    LocationID        *uint     `json:"location_id"`
    ClientContact     string    `json:"client_contact"`
    Address           string    `json:"address"`
    Notes             string    `json:"notes"`
    Result            string    `json:"result"`
    // ... другие поля
}
```

### Installer (Монтажник)

```go
type Installer struct {
    ID                    uint     `json:"id"`
    FirstName             string   `json:"first_name"`
    LastName              string   `json:"last_name"`
    Type                  string   `json:"type"`                    // staff, contractor, partner
    Phone                 string   `json:"phone"`
    Email                 string   `json:"email"`
    Specialization        []string `json:"specialization"`
    MaxDailyInstallations int      `json:"max_daily_installations"`
    WorkingDays           []int    `json:"working_days"`           // 1-7 (пн-вс)
    WorkingHoursStart     string   `json:"working_hours_start"`
    WorkingHoursEnd       string   `json:"working_hours_end"`
    LocationIDs           []uint   `json:"location_ids"`
    IsActive              bool     `json:"is_active"`
    Status                string   `json:"status"`                 // available, busy, vacation, sick
    // ... другие поля
}
```

### Location (Локация)

```go
type Location struct {
    ID        uint     `json:"id"`
    City      string   `json:"city"`
    Region    string   `json:"region"`
    Country   string   `json:"country"`
    Latitude  *float64 `json:"latitude"`
    Longitude *float64 `json:"longitude"`
    Timezone  string   `json:"timezone"`
    IsActive  bool     `json:"is_active"`
}
```

### Equipment (Оборудование)

```go
type Equipment struct {
    ID           uint   `json:"id"`
    Type         string `json:"type"`         // GPS-tracker, sensor, camera, etc.
    Model        string `json:"model"`
    Brand        string `json:"brand"`
    SerialNumber string `json:"serial_number"`
    IMEI         string `json:"imei"`
    PhoneNumber  string `json:"phone_number"`
    Status       string `json:"status"`       // in_stock, reserved, installed, maintenance
    Condition    string `json:"condition"`    // new, used, refurbished, damaged
    ObjectID     *uint  `json:"object_id"`
}
```

## Бизнес-логика и валидация

### Планирование монтажей

1. **Проверка доступности монтажника** на указанную дату и время
2. **Проверка рабочих дней** монтажника
3. **Проверка конфликтов** в расписании (с буферным временем 30 минут)
4. **Контроль максимального количества** монтажей в день
5. **Проверка специализации** монтажника для типа работы
6. **Проверка географических ограничений** (может ли работать в локации)

### Система уведомлений

1. **Автоматические напоминания** за день до монтажа
2. **Уведомления о создании** новых монтажей
3. **Уведомления об изменениях** в расписании
4. **Уведомления о завершении** работ
5. **Логирование всех уведомлений** с отслеживанием статуса

### Отслеживание статуса

- **Автоматические переходы** между статусами
- **Контроль времени выполнения** работ
- **Определение просроченных** монтажей
- **Расчет фактического времени** выполнения

## Интеграция с другими модулями

### Объекты мониторинга

- Монтажи привязываются к объектам через `ObjectID`
- При создании монтажа проверяется существование объекта
- Объект может иметь множество монтажей (история обслуживания)

### Пользователи и роли

- Монтажи создаются пользователями системы (`CreatedByUserID`)
- Права доступа контролируются через систему ролей
- Монтажники могут быть связаны с пользователями системы

### Договоры и биллинг

- Монтажи могут быть оплачиваемыми (`IsBillable`)
- Стоимость работ учитывается в биллинге (`Cost`, `LaborCost`, `MaterialsCost`)
- Связь с договорами через объекты мониторинга

## Примеры использования

### Создание монтажа

```json
POST /api/installations
{
  "type": "монтаж",
  "object_id": 123,
  "installer_id": 45,
  "scheduled_at": "2024-01-15T10:00:00Z",
  "estimated_duration": 120,
  "priority": "normal",
  "description": "Установка GPS-трекера",
  "client_contact": "+7900123456",
  "address": "ул. Ленина, 10, Москва"
}
```

### Поиск доступных монтажников

```http
GET /api/installers/available?date=2024-01-15&location_id=1&specialization=GPS-трекер&duration=120
```

### Получение расписания монтажника

```http
GET /api/installers/5/schedule?date_from=2024-01-15&date_to=2024-01-21
```

### Завершение монтажа

```json
PUT /api/installations/123/complete
{
  "result": "Монтаж выполнен успешно",
  "notes": "Все оборудование установлено и настроено",
  "actual_duration": 90,
  "materials_cost": 1500.0,
  "labor_cost": 2000.0
}
```

## Тестирование

Система включает полный набор unit и integration тестов:

- **API тесты** (`api/installations_test.go`) - тестирование всех endpoints
- **Сервис тесты** (`services/installation_service_test.go`) - тестирование бизнес-логики
- **Benchmark тесты** для проверки производительности
- **Тесты конфликтов** расписания и валидации

Для запуска тестов:

```bash
go test ./api/... -v
go test ./services/... -v
```

## Мониторинг и статистика

Система предоставляет подробную статистику:

- **По монтажам**: общее количество, по статусам, просроченные
- **По монтажникам**: загруженность, рейтинг, количество выполненных работ
- **По локациям**: количество объектов, монтажей по регионам
- **По оборудованию**: остатки на складе, установленное, требующее обслуживания
- **По уведомлениям**: отправленные, неудачные, по каналам

## Расширение функциональности

Система спроектирована для легкого расширения:

1. **Новые каналы уведомлений** - добавление в `NotificationService`
2. **Дополнительные типы работ** - расширение enum'ов в моделях
3. **Интеграция с внешними системами** - через сервисы
4. **Новые правила планирования** - в `InstallationService`
5. **Дополнительные отчеты** - новые endpoints в API

## Требования

- Go 1.19+
- PostgreSQL 13+
- Redis (опционально, для кэширования)
- GORM v2 для работы с БД
