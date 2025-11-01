# 📘 API Документация (OpenAPI)

## Файлы спецификаций

- **`billing-openapi.yaml`** - OpenAPI 3.1.3 спецификация для биллинговых endpoints

## Эндпоинты в спецификации

### Dashboard
- `GET /api/dashboard` - Статистика биллинга

### Contracts
- `GET /api/auth/contracts` - Список договоров
- `POST /api/auth/contracts` - Создать договор
- `GET /api/auth/contracts/{id}` - Получить договор
- `PUT /api/auth/contracts/{id}` - Обновить договор
- `DELETE /api/auth/contracts/{id}` - Удалить договор
- `GET /api/auth/contracts/expiring` - Истекающие договоры

### Invoices
- `GET /api/auth/billing/invoices` - Список счетов
- `POST /api/auth/billing/invoices` - Создать счет
- `GET /api/auth/invoices/{id}` - Получить счет
- `POST /api/invoices/run` - Автоматическая генерация счетов
- `POST /api/invoices/{id}/send` - Отправить счет
- `POST /api/invoices/{id}/pay` - Зарегистрировать оплату
- `POST /api/invoices/{id}/cancel` - Отменить счет

### Tariffs (Billing Plans)
- `GET /api/auth/billing/plans` - Список тарифов
- `POST /api/auth/billing/plans` - Создать тариф
- `GET /api/auth/billing/plans/{id}` - Получить тариф
- `PUT /api/auth/billing/plans/{id}` - Обновить тариф
- `DELETE /api/auth/billing/plans/{id}` - Удалить тариф

### Billing Settings
- `GET /api/auth/billing/settings` - Получить настройки
- `PUT /api/auth/billing/settings` - Обновить настройки

## Использование

### Просмотр в Swagger UI

1. Откройте файл в онлайн редакторе:
   - https://editor.swagger.io/
   - Загрузите файл `billing-openapi.yaml`

2. Или используйте локальный Swagger UI:
   ```bash
   # Установите swagger-ui если нужно
   npm install -g swagger-ui-serve
   
   # Запустите
   swagger-ui-serve configs/billing-openapi.yaml
   ```

### Валидация спецификации

```bash
# Используйте swagger-cli для валидации
npm install -g swagger-cli
swagger-cli validate configs/billing-openapi.yaml
```

### Генерация клиентов

Из OpenAPI спецификации можно сгенерировать клиентские библиотеки:

```bash
# Используя openapi-generator
openapi-generator generate -i configs/billing-openapi.yaml -g typescript-axios -o ./generated-client
```

## Примечания

- Все эндпоинты в `/api/auth/*` требуют JWT авторизации (Bearer token)
- Эндпоинт `/api/dashboard` публичный, но поддерживает опциональную авторизацию
- Параметр `?demo=1` поддерживается для `/api/dashboard` и некоторых других endpoints
- Все денежные суммы возвращаются как `decimal` (строка числа для точности)

## Версионирование

Текущая версия: **v1**

При изменении API создавайте новые версии спецификаций или обновляйте версию в `info.version`.

