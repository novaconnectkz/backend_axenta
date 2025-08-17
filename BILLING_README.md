# Система биллинга Axenta CRM

## Обзор

Система биллинга предоставляет полнофункциональное решение для управления тарифными планами, генерации счетов, обработки платежей и ведения истории биллинга в Axenta CRM.

## Основные возможности

### ✅ 1. Расчет стоимости услуг по тарифным планам

- Автоматический расчет стоимости на основе количества объектов
- Поддержка бесплатных объектов в тарифном плане
- Применение скидок по тарифному плану
- Расчет НДС (включенного и не включенного в стоимость)

### ✅ 2. Генерация счетов с учетом плановых удалений

- Автоматическая генерация счетов для договоров
- Учет плановых удалений объектов при расчете
- Поддержка различных периодов биллинга (месячный, годовой)
- Генерация уникальных номеров счетов

### ✅ 3. Обработка льготных тарифов для неактивных объектов

- Автоматическое применение льготных коэффициентов для неактивных объектов
- Настройка коэффициента скидки в настройках биллинга
- Отдельные позиции в счете для активных и неактивных объектов

### ✅ 4. API для просмотра истории биллинга

- Полная история всех биллинговых операций
- Фильтрация по компаниям, договорам и периодам
- Отслеживание создания счетов, платежей, отмен
- Логирование автоматических процессов

### ✅ 5. Тесты для биллинговых расчетов

- Комплексные unit-тесты для всех расчетов
- Тестирование различных сценариев тарификации
- Проверка корректности генерации счетов
- Тестирование обработки платежей

## Архитектура

### Модели данных

#### Invoice (Счет)

```go
type Invoice struct {
    ID                 uint
    Number             string
    Title              string
    InvoiceDate        time.Time
    DueDate           time.Time
    CompanyID         uint
    ContractID        *uint
    TariffPlanID      uint
    BillingPeriodStart time.Time
    BillingPeriodEnd   time.Time
    SubtotalAmount     decimal.Decimal
    TaxAmount          decimal.Decimal
    TotalAmount        decimal.Decimal
    PaidAmount         decimal.Decimal
    Status             string
    Items              []InvoiceItem
}
```

#### InvoiceItem (Позиция счета)

```go
type InvoiceItem struct {
    ID          uint
    InvoiceID   uint
    Name        string
    Description string
    ItemType    string
    ObjectID    *uint
    Quantity    decimal.Decimal
    UnitPrice   decimal.Decimal
    Amount      decimal.Decimal
    PeriodStart *time.Time
    PeriodEnd   *time.Time
}
```

#### BillingHistory (История биллинга)

```go
type BillingHistory struct {
    ID          uint
    CompanyID   uint
    InvoiceID   *uint
    ContractID  *uint
    Operation   string
    Amount      decimal.Decimal
    Description string
    PeriodStart *time.Time
    PeriodEnd   *time.Time
    Status      string
}
```

#### BillingSettings (Настройки биллинга)

```go
type BillingSettings struct {
    ID                        uint
    CompanyID                 uint
    AutoGenerateInvoices      bool
    InvoiceGenerationDay      int
    InvoicePaymentTermDays    int
    DefaultTaxRate            decimal.Decimal
    TaxIncluded               bool
    NotifyBeforeInvoice       int
    NotifyBeforeDue           int
    NotifyOverdue             int
    InvoiceNumberPrefix       string
    InvoiceNumberFormat       string
    Currency                  string
    EnableInactiveDiscounts   bool
    InactiveDiscountRatio     decimal.Decimal
}
```

### Сервисы

#### BillingService

Основной сервис для работы с биллингом:

- `CalculateBillingForContract()` - расчет биллинга для договора
- `GenerateInvoiceForContract()` - генерация счета
- `ProcessPayment()` - обработка платежа
- `CancelInvoice()` - отмена счета
- `GetBillingHistory()` - получение истории
- `GetOverdueInvoices()` - получение просроченных счетов

#### BillingAutomationService

Сервис автоматизации биллинга:

- `AutoGenerateInvoicesForMonth()` - автогенерация счетов за месяц
- `ProcessScheduledDeletions()` - обработка плановых удалений
- `GetBillingStatistics()` - получение статистики
- `SendInvoiceReminders()` - отправка напоминаний

## API Endpoints

### Основные операции с счетами

```
GET    /api/billing/contracts/:contract_id/calculate  - Расчет биллинга
POST   /api/billing/contracts/:contract_id/invoice    - Генерация счета
GET    /api/billing/invoices                          - Список счетов
GET    /api/billing/invoices/:id                      - Получение счета
POST   /api/billing/invoices/:id/payment              - Обработка платежа
POST   /api/billing/invoices/:id/cancel               - Отмена счета
```

### История и отчеты

```
GET    /api/billing/history                           - История биллинга
GET    /api/billing/invoices/overdue                  - Просроченные счета
GET    /api/billing/statistics                        - Статистика
GET    /api/billing/invoices/period                   - Счета за период
```

### Настройки

```
GET    /api/billing/settings                          - Получение настроек
PUT    /api/billing/settings                          - Обновление настроек
```

### Автоматизация

```
POST   /api/billing/auto-generate                     - Автогенерация счетов
POST   /api/billing/process-deletions                 - Обработка удалений
```

## Примеры использования

### 1. Расчет стоимости для договора

```bash
curl -X GET "http://localhost:8080/api/billing/contracts/1/calculate?period_start=2024-01-01&period_end=2024-01-31"
```

Ответ:

```json
{
  "status": "success",
  "data": {
    "contract_id": 1,
    "company_id": 1,
    "active_objects": 5,
    "inactive_objects": 2,
    "scheduled_deletes": 1,
    "base_amount": "1000.00",
    "objects_amount": "350.00",
    "discount_amount": "0.00",
    "subtotal_amount": "1350.00",
    "tax_amount": "270.00",
    "total_amount": "1620.00",
    "items": [
      {
        "name": "Подписка \"Базовый план\"",
        "item_type": "subscription",
        "quantity": "1.000",
        "unit_price": "1000.00",
        "amount": "1000.00"
      },
      {
        "name": "Активные объекты мониторинга",
        "item_type": "object",
        "quantity": "3.000",
        "unit_price": "100.00",
        "amount": "300.00"
      },
      {
        "name": "Неактивные объекты мониторинга",
        "item_type": "object",
        "quantity": "2.000",
        "unit_price": "50.00",
        "amount": "100.00"
      }
    ]
  }
}
```

### 2. Генерация счета

```bash
curl -X POST "http://localhost:8080/api/billing/contracts/1/invoice" \
  -H "Content-Type: application/json" \
  -d '{
    "period_start": "2024-01-01",
    "period_end": "2024-01-31"
  }'
```

### 3. Обработка платежа

```bash
curl -X POST "http://localhost:8080/api/billing/invoices/123/payment" \
  -H "Content-Type: application/json" \
  -d '{
    "amount": "1620.00",
    "payment_method": "bank_transfer",
    "notes": "Оплата по банковскому переводу"
  }'
```

### 4. Получение статистики

```bash
curl -X GET "http://localhost:8080/api/billing/statistics?company_id=1&year=2024&month=1"
```

## Алгоритм расчета биллинга

### 1. Базовая стоимость

- Берется из тарифного плана (`TariffPlan.Price`)
- Добавляется как отдельная позиция "Подписка"

### 2. Стоимость объектов

```
Всего объектов = Активные + Неактивные
Платных объектов = Всего - Бесплатные

Если Активных >= Платных объектов:
    Платных активных = Платных объектов
    Платных неактивных = 0
Иначе:
    Платных активных = Активные
    Платных неактивных = Платных объектов - Активные

Стоимость активных = Платных активных × Цена за объект
Стоимость неактивных = Платных неактивных × Цена за объект × Коэффициент льготы
```

### 3. Применение скидок

- Скидка тарифного плана применяется к промежуточной сумме
- Льготы для неактивных объектов применяются отдельно

### 4. Расчет налогов

- НДС рассчитывается от промежуточной суммы (после скидок)
- Может быть включен в стоимость или добавляться сверху

## Статусы счетов

- `draft` - Черновик (только что создан)
- `sent` - Отправлен клиенту
- `partially_paid` - Частично оплачен
- `paid` - Полностью оплачен
- `overdue` - Просрочен
- `cancelled` - Отменен

## Типы операций в истории

- `invoice_created` - Создан счет
- `payment_received` - Получен платеж
- `invoice_cancelled` - Отменен счет
- `reminder_sent` - Отправлено напоминание
- `object_scheduled_deletion` - Плановое удаление объекта
- `monthly_report_generated` - Сгенерирован месячный отчет

## Автоматизация

### Автогенерация счетов

Система может автоматически генерировать счета за месяц для всех активных договоров:

```bash
curl -X POST "http://localhost:8080/api/billing/auto-generate?year=2024&month=1"
```

### Обработка плановых удалений

Автоматическая обработка объектов с истекшим сроком планового удаления:

```bash
curl -X POST "http://localhost:8080/api/billing/process-deletions"
```

## Настройки компании

Каждая компания может настроить:

- Автоматическую генерацию счетов
- День месяца для генерации
- Срок оплаты в днях
- Ставку НДС
- Префикс и формат номеров счетов
- Настройки уведомлений
- Льготы для неактивных объектов

## Тестирование

Запуск тестов:

```bash
cd services
go test -v billing_service_test.go
```

Тесты покрывают:

- ✅ Расчет биллинга с различными сценариями
- ✅ Генерацию счетов
- ✅ Обработку платежей
- ✅ Отмену счетов
- ✅ Получение просроченных счетов
- ✅ Расчет стоимости объектов с льготами

## Безопасность

- Все операции требуют аутентификации
- Мультитенантность обеспечивает изоляцию данных между компаниями
- Валидация входных данных на всех уровнях
- Логирование всех критических операций

## Производительность

- Использование индексов БД для быстрого поиска
- Пагинация для больших списков
- Кэширование настроек биллинга
- Оптимизированные запросы с Preload для связанных данных

## Мониторинг

Система ведет подробные логи:

- Создание и изменение счетов
- Обработка платежей
- Ошибки в расчетах
- Автоматические процессы

## Интеграции

Система готова к интеграции с:

- Внешними платежными системами
- 1С для экспорта реестра платежей
- Битрикс24 для синхронизации сделок
- Системами уведомлений (email, Telegram)

## Расширение функциональности

Архитектура позволяет легко добавить:

- Новые типы тарифных планов
- Дополнительные методы расчета
- Интеграции с платежными системами
- Автоматические уведомления
- Экспорт отчетов в различных форматах
