# Интеграция с Axetna.cloud API

## Обзор

Реализована полная интеграция с Axetna.cloud API для синхронизации объектов мониторинга.

## Компоненты

### 1. AxetnaClient (`services/axetna_client.go`)

- HTTP клиент для работы с Axetna.cloud API
- Retry механизм с экспоненциальным backoff
- Автоматическое обновление JWT токенов
- Поддержка таймаутов и обработки ошибок

### 2. IntegrationService (`services/integration_service.go`)

- Управление интеграциями с внешними системами
- Мультитенантное хранение учетных данных с шифрованием
- Асинхронная синхронизация объектов
- Кэширование учетных данных

### 3. Синхронизация объектов

Автоматическая синхронизация при:

- ✅ Создании объекта (`CREATE`)
- ✅ Обновлении объекта (`UPDATE`)
- ✅ Удалении объекта (`DELETE`)

### 4. Обработка ошибок

- Структурированные ошибки интеграции
- Сохранение ошибок в БД для анализа
- Retry механизм с настраиваемыми параметрами
- Логирование всех операций

### 5. API Endpoints

```
GET    /api/integration/health                    - Статус интеграций
GET    /api/integration/errors                    - Список ошибок интеграции
GET    /api/integration/errors/stats              - Статистика ошибок
POST   /api/integration/errors/:id/retry          - Повторить обработку ошибки
POST   /api/integration/errors/:id/resolve        - Отметить ошибку как решенную
POST   /api/integration/credentials               - Настроить учетные данные
DELETE /api/integration/cache                     - Очистить кэш интеграций
```

## Конфигурация

### Переменные окружения

```bash
# URL API Axetna.cloud
AXETNA_API_URL=https://api.axetna.cloud

# Ключ шифрования (32 символа)
ENCRYPTION_KEY=your-32-character-encryption-key!!
```

### Настройка учетных данных компании

```bash
POST /api/integration/credentials
{
  "axetna_login": "company_login",
  "axetna_password": "company_password"
}
```

## Тестирование

### Unit тесты

```bash
go test ./services/ -v
```

### Integration тесты с моками

```bash
go test ./services/ -run TestIntegrationService -v
```

### Бенчмарки

```bash
go test ./services/ -bench=BenchmarkIntegrationService -v
```

## Мониторинг

### Проверка здоровья интеграций

```bash
GET /api/integration/health
```

### Просмотр ошибок интеграции

```bash
GET /api/integration/errors?status=pending&retryable_only=true
```

### Статистика ошибок

```bash
GET /api/integration/errors/stats
```

## Архитектура

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Objects API   │───▶│ IntegrationService│───▶│  AxetnaClient   │
│   (CRUD ops)    │    │   (async sync)    │    │  (HTTP + retry) │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                        │                        │
         ▼                        ▼                        ▼
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│  Local Database │    │ Integration Errors│    │ Axetna.cloud API│
│   (tenant DB)   │    │   (error logs)    │    │  (external)     │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

## Особенности реализации

1. **Мультитенантность**: Каждая компания имеет свои учетные данные
2. **Шифрование**: Пароли шифруются AES-GCM перед сохранением в БД
3. **Асинхронность**: Синхронизация не блокирует основные операции
4. **Отказоустойчивость**: Retry механизм с экспоненциальным backoff
5. **Мониторинг**: Полное логирование и статистика ошибок
6. **Тестируемость**: Моки для всех внешних зависимостей

## Статус реализации

✅ **ЗАДАЧА 6 ПОЛНОСТЬЮ ВЫПОЛНЕНА**

- ✅ Клиент для работы с Axetna.cloud API
- ✅ Мультитенантное хранение учетных данных
- ✅ Синхронизация объектов с внешним API
- ✅ Retry механизмы и обработка ошибок
- ✅ Integration тесты с мокированием внешнего API
- ✅ API endpoints для управления интеграциями
- ✅ Система логирования и мониторинга ошибок
