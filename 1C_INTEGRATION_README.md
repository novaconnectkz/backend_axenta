# Интеграция с 1С

Модуль интеграции с системой 1С:Предприятие для обмена данными о платежах и контрагентах.

## Оглавление

- [Обзор](#обзор)
- [Архитектура](#архитектура)
- [Компоненты](#компоненты)
- [Настройка](#настройка)
- [API Endpoints](#api-endpoints)
- [Форматы данных](#форматы-данных)
- [Обработка ошибок](#обработка-ошибок)
- [Тестирование](#тестирование)
- [Производительность](#производительность)
- [Безопасность](#безопасность)

## Обзор

Интеграция с 1С обеспечивает:

- **Экспорт реестров платежей** в формате 1С
- **Импорт контрагентов** из 1С в систему
- **Синхронизацию статусов платежей** между системами
- **Автоматические процессы** экспорта и импорта
- **Мониторинг и логирование** всех операций

### Основные возможности

1. **Двусторонний обмен данными**

   - Экспорт оплаченных счетов как реестры платежей
   - Импорт справочника контрагентов
   - Синхронизация статусов документов

2. **Автоматизация процессов**

   - Планировщик автоматического экспорта
   - Настраиваемые интервалы синхронизации
   - Обработка ошибок с повторными попытками

3. **Гибкая настройка**
   - Настройка подключения к различным базам 1С
   - Маппинг полей и справочников
   - Фильтрация данных при обмене

## Архитектура

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Frontend      │    │   Backend API   │    │   1С:Предприятие│
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │ 1С Settings │ │◄──►│ │ 1С API      │ │◄──►│ │ Web-сервисы │ │
│ │ Management  │ │    │ │ Endpoints   │ │    │ │ HTTP API    │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
│                 │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ ┌─────────────┐ │    │ │ Integration │ │    │ │ Справочники │ │
│ │ Monitoring  │ │◄──►│ │ Service     │ │◄──►│ │ Документы   │ │
│ │ Dashboard   │ │    │ └─────────────┘ │    │ └─────────────┘ │
│ └─────────────┘ │    │ ┌─────────────┐ │    │                 │
│                 │    │ │ 1С Client   │ │    │                 │
│                 │    │ │ + Mock      │ │    │                 │
│                 │    │ └─────────────┘ │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Компоненты

### 1. OneCClient (`services/1c_client.go`)

HTTP клиент для взаимодействия с веб-сервисами 1С.

**Основные методы:**

- `CallMethod()` - универсальный вызов методов API
- `GetCounterparties()` - получение списка контрагентов
- `CreateCounterparty()` - создание контрагента
- `ExportPaymentRegistry()` - экспорт реестра платежей
- `GetPaymentStatus()` - получение статуса платежа
- `UpdatePaymentStatus()` - обновление статуса платежа
- `IsHealthy()` - проверка доступности API

**Особенности:**

- Retry механизм с экспоненциальным backoff
- Базовая HTTP аутентификация
- Поддержка различных версий API 1С
- Логирование всех запросов и ответов

### 2. OneCIntegrationService (`services/1c_integration_service.go`)

Сервис бизнес-логики для интеграции с 1С.

**Основные методы:**

- `ExportPaymentRegistry()` - экспорт реестра платежей
- `ImportCounterparties()` - импорт контрагентов
- `SyncPaymentStatuses()` - синхронизация статусов
- `TestConnection()` - тест подключения
- `ScheduleAutoExport()` - автоматический экспорт

**Возможности:**

- Кэширование учетных данных
- Логирование ошибок интеграции
- Автоматические процессы
- Валидация данных

### 3. OneCIntegrationAPI (`api/1c_integration.go`)

REST API для управления интеграцией с 1С.

**Endpoints:**

- `POST /api/1c/setup` - настройка интеграции
- `PUT /api/1c/setup` - обновление настроек
- `GET /api/1c/config` - получение конфигурации
- `DELETE /api/1c/setup` - удаление интеграции
- `POST /api/1c/test-connection` - тест подключения
- `POST /api/1c/export/payment-registry` - экспорт платежей
- `POST /api/1c/import/counterparties` - импорт контрагентов
- `POST /api/1c/sync/payment-statuses` - синхронизация
- `GET /api/1c/errors` - список ошибок
- `PUT /api/1c/errors/:id/resolve` - разрешение ошибки
- `GET /api/1c/status` - статус интеграции

### 4. OneCClientMock (`services/1c_client_mock.go`)

Mock-клиент для тестирования без реального подключения к 1С.

**Возможности:**

- Имитация всех методов реального клиента
- Настраиваемые ответы и ошибки
- Отслеживание вызовов для тестов
- Тестовые данные для разработки

## Настройка

### 1. Настройка интеграции через API

```bash
curl -X POST http://localhost:8080/api/1c/setup \
  -H "Content-Type: application/json" \
  -d '{
    "base_url": "http://1c-server:8080",
    "username": "integration_user",
    "password": "secure_password",
    "database": "production_db",
    "api_version": "v1",
    "organization_code": "ORG001",
    "bank_account_code": "BANK001",
    "payment_type_code": "PAYMENT001",
    "contract_type_code": "CONTRACT001",
    "currency_code": "RUB",
    "auto_export_enabled": true,
    "auto_import_enabled": true,
    "sync_interval": 60
  }'
```

### 2. Параметры конфигурации

| Параметр              | Описание                        | Обязательный |
| --------------------- | ------------------------------- | ------------ |
| `base_url`            | Базовый URL веб-сервисов 1С     | Да           |
| `username`            | Имя пользователя                | Да           |
| `password`            | Пароль                          | Да           |
| `database`            | Имя информационной базы         | Да           |
| `api_version`         | Версия API (по умолчанию v1)    | Нет          |
| `organization_code`   | Код организации в 1С            | Да           |
| `bank_account_code`   | Код банковского счета           | Да           |
| `payment_type_code`   | Код типа платежа                | Да           |
| `contract_type_code`  | Код типа договора               | Да           |
| `currency_code`       | Код валюты (по умолчанию RUB)   | Нет          |
| `auto_export_enabled` | Автоматический экспорт          | Нет          |
| `auto_import_enabled` | Автоматический импорт           | Нет          |
| `sync_interval`       | Интервал синхронизации (минуты) | Нет          |

### 3. Настройка 1С

В 1С необходимо настроить:

1. **HTTP-сервисы** для API
2. **Пользователя интеграции** с необходимыми правами
3. **Справочники и документы** для обмена данными
4. **Веб-сервисы** для внешнего доступа

Пример настройки HTTP-сервиса в 1С:

```bsl
// Модуль HTTP-сервиса
Функция ПолучитьКонтрагентов(Запрос)
    Результат = Новый Структура;

    Попытка
        // Получение параметров
        Лимит = Запрос.ПараметрыURL.Получить("limit");
        Смещение = Запрос.ПараметрыURL.Получить("offset");

        // Запрос к справочнику
        Запрос = Новый Запрос;
        Запрос.Текст = "
        |ВЫБРАТЬ
        |    Контрагенты.Ссылка КАК Ref_Key,
        |    Контрагенты.Код КАК Code,
        |    Контрагенты.Наименование КАК Description,
        |    Контрагенты.ПолноеНаименование КАК FullName,
        |    Контрагенты.ИНН КАК INN,
        |    Контрагенты.КПП КАК KPP,
        |    Контрагенты.Телефон КАК Phone,
        |    Контрагенты.АдресЭлектроннойПочты КАК Email
        |ИЗ
        |    Справочник.Контрагенты КАК Контрагенты
        |ГДЕ
        |    НЕ Контрагенты.ПометкаУдаления";

        РезультатЗапроса = Запрос.Выполнить();

        Результат.Вставить("success", Истина);
        Результат.Вставить("data", РезультатЗапроса.Выгрузить());

    Исключение
        Результат.Вставить("success", Ложь);
        Результат.Вставить("error", ОписаниеОшибки());
    КонецПопытки;

    Возврат Результат;
КонецФункции
```

## API Endpoints

### Настройка интеграции

#### POST /api/1c/setup

Создает новую интеграцию с 1С.

**Запрос:**

```json
{
  "base_url": "http://1c-server:8080",
  "username": "integration_user",
  "password": "secure_password",
  "database": "production_db",
  "organization_code": "ORG001",
  "bank_account_code": "BANK001",
  "payment_type_code": "PAYMENT001",
  "contract_type_code": "CONTRACT001"
}
```

**Ответ:**

```json
{
  "message": "Интеграция с 1С успешно настроена",
  "integration_id": 123
}
```

#### PUT /api/1c/setup

Обновляет настройки существующей интеграции.

#### GET /api/1c/config

Получает текущую конфигурацию интеграции.

**Ответ:**

```json
{
  "integration": {
    "id": 123,
    "name": "Интеграция с 1С",
    "is_active": true,
    "created_at": "2024-01-15T10:00:00Z"
  },
  "config": {
    "base_url": "http://1c-server:8080",
    "username": "integration_user",
    "password": "***",
    "database": "production_db",
    "auto_export_enabled": true,
    "sync_interval": 60
  }
}
```

#### DELETE /api/1c/setup

Удаляет интеграцию с 1С.

### Операции с данными

#### POST /api/1c/export/payment-registry

Экспортирует реестр платежей в 1С.

**Запрос:**

```json
{
  "invoice_ids": [1, 2, 3],
  "registry_number": "REG-2024-001",
  "start_date": "2024-01-01",
  "end_date": "2024-01-31"
}
```

**Ответ:**

```json
{
  "message": "Реестр платежей успешно экспортирован в 1С",
  "registry_number": "REG-2024-001",
  "invoices_count": 3
}
```

#### POST /api/1c/import/counterparties

Импортирует контрагентов из 1С.

**Ответ:**

```json
{
  "message": "Контрагенты успешно импортированы из 1С"
}
```

#### POST /api/1c/sync/payment-statuses

Синхронизирует статусы платежей с 1С.

**Ответ:**

```json
{
  "message": "Статусы платежей успешно синхронизированы"
}
```

### Мониторинг и диагностика

#### POST /api/1c/test-connection

Тестирует подключение к 1С.

**Ответ:**

```json
{
  "message": "Подключение к 1С успешно",
  "connected": true
}
```

#### GET /api/1c/status

Получает статус интеграции.

**Ответ:**

```json
{
  "configured": true,
  "active": true,
  "connection_ok": true,
  "errors_count": 0,
  "last_sync": "2024-01-15T10:30:00Z",
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-15T10:00:00Z"
}
```

#### GET /api/1c/errors

Получает список ошибок интеграции.

**Параметры:**

- `resolved` (boolean) - фильтр по статусу разрешения

**Ответ:**

```json
{
  "errors": [
    {
      "id": 1,
      "operation": "export_payment",
      "entity_type": "registry",
      "entity_id": "REG-001",
      "error_code": "CONNECTION_ERROR",
      "error_message": "Ошибка подключения к 1С",
      "resolved": false,
      "created_at": "2024-01-15T10:00:00Z"
    }
  ],
  "count": 1
}
```

#### PUT /api/1c/errors/:id/resolve

Помечает ошибку как решенную.

**Ответ:**

```json
{
  "message": "Ошибка помечена как решенная"
}
```

## Форматы данных

### Контрагент (OneCCounterparty)

```json
{
  "id": "counterparty-123",
  "code": "CP001",
  "description": "ООО Тестовая компания",
  "full_name": "Общество с ограниченной ответственностью \"Тестовая компания\"",
  "inn": "1234567890",
  "kpp": "123456789",
  "ogrn": "1234567890123",
  "legal_address": "123456, г. Москва, ул. Тестовая, д. 1",
  "actual_address": "123456, г. Москва, ул. Тестовая, д. 1",
  "phone": "+7 (495) 123-45-67",
  "email": "info@testcompany.ru",
  "is_active": true
}
```

### Платеж (OneCPayment)

```json
{
  "id": "payment-123",
  "number": "PAY-001",
  "date": "2024-01-15T10:00:00Z",
  "posted": true,
  "amount": 10000.0,
  "purpose": "Оплата по счету INV-001",
  "payment_method": "bank_transfer",
  "operation_type": "income",
  "currency": "RUB",
  "external_id": "invoice_123",
  "comment": "Комментарий к платежу"
}
```

### Реестр платежей (OneCPaymentRegistry)

```json
{
  "registry_number": "REG-2024-001",
  "registry_date": "2024-01-15T10:00:00Z",
  "organization": "ORG001",
  "bank_account": "BANK001",
  "total_amount": 50000.0,
  "payments_count": 5,
  "payments": [
    {
      "number": "PAY-001",
      "date": "2024-01-15T10:00:00Z",
      "amount": 10000.0,
      "purpose": "Оплата по счету INV-001"
    }
  ],
  "period": {
    "start_date": "2024-01-01T00:00:00Z",
    "end_date": "2024-01-31T23:59:59Z"
  },
  "status": "pending"
}
```

## Обработка ошибок

### Типы ошибок

1. **CONNECTION_ERROR** - Ошибки подключения к 1С
2. **AUTH_ERROR** - Ошибки аутентификации
3. **VALIDATION_ERROR** - Ошибки валидации данных
4. **EXPORT_ERROR** - Ошибки экспорта данных
5. **IMPORT_ERROR** - Ошибки импорта данных
6. **SYNC_ERROR** - Ошибки синхронизации

### Retry механизм

Клиент автоматически повторяет запросы при временных ошибках:

```go
config := RetryConfig{
    MaxRetries:      3,
    InitialDelay:    2 * time.Second,
    MaxDelay:        30 * time.Second,
    BackoffFactor:   2.0,
    RetryableErrors: []int{500, 502, 503, 504, 408, 429},
}
```

### Логирование ошибок

Все ошибки интеграции сохраняются в таблице `1c_integration_errors`:

```sql
CREATE TABLE 1c_integration_errors (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    operation VARCHAR(50) NOT NULL,
    entity_type VARCHAR(50),
    entity_id VARCHAR(50),
    error_code VARCHAR(50),
    error_message TEXT,
    request_data JSONB,
    response_data JSONB,
    resolved BOOLEAN DEFAULT FALSE,
    resolved_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

## Тестирование

### Unit тесты

Запуск unit тестов:

```bash
cd services
go test -v -run TestOneCIntegrationService
```

### Integration тесты

Запуск integration тестов:

```bash
cd api
go test -v -run TestOneCIntegrationAPI
```

### Тестирование с mock клиентом

```go
// Создание mock клиента
mockClient := services.NewOneCClientMock(logger)
mockClient.SetupMockData()

// Настройка на ошибку
mockClient.SetFailure(true, "Тестовая ошибка")

// Проверка вызовов
calls := mockClient.GetCallHistory()
assert.Len(t, calls, 1)
assert.Equal(t, "counterparties", calls[0].Method)
```

### Benchmark тесты

Запуск benchmark тестов:

```bash
cd services
go test -bench=BenchmarkOneCIntegrationService -benchmem
```

## Производительность

### Оптимизации

1. **Кэширование учетных данных** - 1 час
2. **Пакетная обработка** - до 100 записей за раз
3. **Параллельная обработка** - для независимых операций
4. **Connection pooling** - переиспользование HTTP соединений

### Мониторинг

Ключевые метрики для мониторинга:

- Время выполнения запросов к 1С
- Количество успешных/неуспешных операций
- Размер обрабатываемых данных
- Использование памяти и CPU

### Лимиты

- **Максимальный размер реестра**: 1000 платежей
- **Timeout запроса**: 60 секунд
- **Максимальное количество retry**: 3
- **Интервал между запросами**: 1 секунда

## Безопасность

### Защита учетных данных

1. **Шифрование паролей** в базе данных
2. **HTTPS соединения** с 1С
3. **Маскирование паролей** в логах и API ответах
4. **Ротация токенов** доступа

### Аудит операций

Все операции интеграции логируются:

```go
type OneCIntegrationError struct {
    CompanyID    uint      `json:"company_id"`
    Operation    string    `json:"operation"`
    EntityType   string    `json:"entity_type"`
    EntityID     string    `json:"entity_id"`
    RequestData  string    `json:"request_data"`
    ResponseData string    `json:"response_data"`
    CreatedAt    time.Time `json:"created_at"`
}
```

### Ограничения доступа

- **Авторизация по company_id** - изоляция данных
- **Валидация входных данных** - предотвращение инъекций
- **Rate limiting** - защита от DDoS атак
- **IP whitelist** - ограничение доступа к API

## Развертывание

### Production настройки

```yaml
# docker-compose.yml
version: "3.8"
services:
  backend:
    environment:
      - 1C_TIMEOUT=60s
      - 1C_MAX_RETRIES=3
      - 1C_CACHE_TTL=3600s
      - 1C_BATCH_SIZE=100
```

### Мониторинг

Настройка алертов:

```yaml
# alerts.yml
- alert: OneCIntegrationDown
  expr: 1c_connection_status == 0
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "1С интеграция недоступна"

- alert: OneCIntegrationErrors
  expr: increase(1c_integration_errors_total[5m]) > 10
  for: 2m
  labels:
    severity: warning
  annotations:
    summary: "Высокий уровень ошибок интеграции с 1С"
```

### Резервное копирование

Настройка backup для конфигурации интеграции:

```bash
# Backup конфигурации
pg_dump -t integrations -t 1c_integration_errors axenta_db > 1c_backup.sql

# Восстановление
psql axenta_db < 1c_backup.sql
```

---

Документация актуальна для версии интеграции 1.0.0
Последнее обновление: 2024-01-15
