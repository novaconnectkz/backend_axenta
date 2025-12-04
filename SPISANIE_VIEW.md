# Как посмотреть списания по договору

Есть несколько способов просмотреть списания (счета и платежи) по договору:

## Способ 1: Отладочный эндпоинт (рекомендуется)

Создан специальный эндпоинт для полного анализа договора:

### Запрос:
```
GET /api/auth/billing/contracts/by-number/{НОМЕР_ДОГОВОРА}/analysis
```

### Пример для договора Т-041225/065:
```
GET /api/auth/billing/contracts/by-number/Т-041225%2F065/analysis
```

**Важно:** Номер договора в URL нужно закодировать (URL-encode):
- `/` → `%2F`
- Пробелы → `%20`
- И т.д.

### Что возвращает:
```json
{
  "status": "success",
  "data": {
    "contract": {
      "id": 123,
      "number": "Т-041225/065",
      "title": "Название договора",
      "client_name": "Название клиента",
      "contract_type": "partner",
      "status": "active",
      "start_date": "2024-01-01T00:00:00Z",
      "end_date": "2024-12-31T00:00:00Z"
    },
    "statistics": {
      "total_invoiced": 50000.00,
      "total_paid": 45000.00,
      "total_outstanding": 5000.00,
      "invoices_count": 10,
      "paid_invoices": 8,
      "unpaid_invoices": 2
    },
    "invoices": [
      {
        "id": 1,
        "number": "ПМ-0026",
        "invoice_date": "2024-12-04T00:00:00Z",
        "due_date": "2024-12-18T00:00:00Z",
        "total_amount": 150.00,
        "paid_amount": 150.00,
        "outstanding": 0.00,
        "status": "paid",
        "billing_period_start": "2024-12-01T00:00:00Z",
        "billing_period_end": "2024-12-31T00:00:00Z"
      }
    ],
    "payment_history": [
      {
        "id": 1,
        "operation": "payment_received",
        "amount": 150.00,
        "currency": "RUB",
        "description": "Получен платеж по счету ПМ-0026...",
        "status": "completed",
        "created_at": "2024-12-04T10:00:00Z",
        "invoice_id": 1
      }
    ]
  }
}
```

### Использование через curl:
```bash
# Получить токен авторизации из фронтенда/приложения
TOKEN="ваш_токен_авторизации"

# Запрос (номер договора URL-encoded)
curl -H "Authorization: Token $TOKEN" \
  "http://localhost:8080/api/auth/billing/contracts/by-number/Т-041225%2F065/analysis"
```

### Использование через браузер:
1. Откройте консоль разработчика (F12)
2. Перейдите на вкладку "Network"
3. Выполните запрос с авторизацией

Или используйте Postman/Insomnia с заголовком:
```
Authorization: Token ваш_токен
```

---

## Способ 2: Через существующий эндпоинт GetInvoices

### Запрос:
```
GET /api/auth/billing/invoices?contract_id={ID_ДОГОВОРА}
```

### Что нужно:
- ID договора (не номер!)
- Сначала найдите ID договора через `/api/auth/contracts`

### Использование:
```bash
# 1. Найти ID договора по номеру
curl -H "Authorization: Token $TOKEN" \
  "http://localhost:8080/api/auth/contracts" | grep -A 5 "Т-041225/065"

# 2. Получить счета по ID договора
curl -H "Authorization: Token $TOKEN" \
  "http://localhost:8080/api/auth/billing/invoices?contract_id=123"
```

---

## Способ 3: Через UI (веб-интерфейс)

1. Откройте раздел **"Биллинг и договоры"**
2. Перейдите на вкладку **"Счета"**
3. Используйте фильтры для поиска счетов по договору
4. Для просмотра истории платежей используйте раздел истории биллинга

---

## Как работает списание денег?

### 1. Для обычных договоров (client):
- Расчет на основе объектов, привязанных к договору
- Учитываются активные/неактивные объекты
- Применяются скидки и настройки биллинга

### 2. Для партнерских договоров (partner):
- Используются ежедневные снимки объектов (`partner_daily_snapshots`)
- Формула для каждого дня: `(тариф/30) * количество_активных_объектов`
- Общая стоимость = сумма `daily_cost` из всех снимков за период

### Процесс списания:
1. **Расчет биллинга** → `CalculateBillingForContract()` рассчитывает сумму за период
2. **Создание счета** → `GenerateInvoiceForContract()` создаёт счет на основе расчёта
3. **Оплата счета** → `ProcessPayment()` обновляет статус счета и создаёт запись в истории

---

## Дополнительная информация

### Эндпоинты для детального анализа:
- `/api/auth/billing/contracts/{id}/breakdown` - детализация расчета по месяцам
- `/api/auth/billing/contracts/{id}/calculate` - расчет биллинга за период
- `/api/auth/billing/history` - история всех операций биллинга

### Пример запроса breakdown:
```bash
curl -H "Authorization: Token $TOKEN" \
  "http://localhost:8080/api/auth/billing/contracts/123/breakdown?year=2024&month=12"
```

---

## Примечания

- Все эндпоинты требуют авторизации (токен в заголовке `Authorization`)
- Номера договоров в URL должны быть URL-encoded
- Для партнерских договоров данные берутся из снимков (snapshots)
- История платежей хранится в таблице `billing_history`

