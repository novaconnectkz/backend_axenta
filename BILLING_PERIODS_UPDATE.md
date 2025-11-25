# Обновление периодов биллинга

## Дата: 2025-11-25

## Описание
Добавлена поддержка новых периодов биллинга для более гибкой тарификации.

## Новые периоды

### 1. **Часовой (hourly)**
- Тарификация за час использования
- Расчет: `цена/час × количество часов × количество объектов`
- Для месячного расчета: `цена × 720 часов (30 дней × 24 часа)`
- Цвет в UI: Orange

### 2. **Дневной (daily)**
- Тарификация за день использования
- Расчет: `цена/день × количество дней × количество объектов`
- Для месячного расчета: `цена × 30 дней`
- Цвет в UI: Cyan

### 3. **Недельный (weekly)**
- Тарификация за неделю использования
- Расчет: `цена/неделя × количество недель × количество объектов`
- Для месячного расчета: `цена × 4 недели`
- Цвет в UI: Teal

### 4. **Месячный (monthly)** - без изменений
- Стандартная месячная тарификация
- Расчет: `цена/месяц × количество объектов`
- Цвет в UI: Blue

### 5. **Годовой (yearly)** - без изменений
- Годовая тарификация с пропорциональным расчетом
- Расчет: `(цена/год ÷ 12) × количество месяцев × количество объектов`
- Цвет в UI: Purple

### 6. **Одноразовый (one-time)** - без изменений
- Разовая оплата без рекуррентных платежей
- Цвет в UI: Grey

## Измененные файлы

### Backend
1. **`models/billing_plan.go`**
   - Обновлен комментарий к полю `BillingPeriod`
   - Добавлены новые значения: `hourly`, `daily`, `weekly`

2. **`api/billing.go`**
   - Функция `recalculateContractTotalAmount()` - добавлен `switch` для обработки всех периодов
   - Функция `GetContractBillingBreakdown()` - обновлена логика расчета для детализации

3. **`migrations/20251125_recalculate_contract_totals.sql`**
   - Добавлены условия для новых периодов биллинга

### Frontend
1. **`src/views/Billing.vue`**
   - Массив `billingPeriods` - добавлены новые опции
   - Функция `getBillingPeriodText()` - добавлены переводы новых периодов

2. **`src/components/Billing/ContractsTab.vue`**
   - Добавлены функции:
     - `getBillingPeriodLabel()` - получение названия периода
     - `getBillingPeriodColor()` - получение цвета для чипа
   - Обновлен шаблон отображения периода в детализации расчета

## Логика расчета стоимости

### Для recalculateContractTotalAmount (общая сумма договора):
```go
switch billingPlan.BillingPeriod {
case "yearly":
    pricePerMonth := price / 12
    amount = pricePerMonth × months × objectsCount
case "weekly":
    weeks := (months × 30) / 7
    amount = price × weeks × objectsCount
case "daily":
    days := months × 30
    amount = price × days × objectsCount
case "hourly":
    hours := months × 30 × 24
    amount = price × hours × objectsCount
default: // monthly
    amount = price × objectsCount
}
```

### Для GetContractBillingBreakdown (детализация по месяцам):
```go
switch billingPlan.BillingPeriod {
case "yearly":
    pricePerObject = price / 12
    amount = pricePerObject × objectsCount
case "weekly":
    pricePerObject = price × 4  // ~4 недели в месяце
    amount = pricePerObject × objectsCount
case "daily":
    pricePerObject = price × 30  // 30 дней в месяце
    amount = pricePerObject × objectsCount
case "hourly":
    pricePerObject = price × 720  // 30 дней × 24 часа
    amount = pricePerObject × objectsCount
default: // monthly
    pricePerObject = price
    amount = price × objectsCount
}
```

## Примеры использования

### Пример 1: Часовая тарификация
- Тариф: 10₽/час
- Объектов: 5
- Период договора: 1 месяц (720 часов)
- **Итого:** 10₽ × 720 × 5 = **36,000₽**

### Пример 2: Дневная тарификация
- Тариф: 150₽/день
- Объектов: 3
- Период договора: 1 месяц (30 дней)
- **Итого:** 150₽ × 30 × 3 = **13,500₽**

### Пример 3: Недельная тарификация
- Тариф: 1000₽/неделя
- Объектов: 2
- Период договора: 1 месяц (≈4 недели)
- **Итого:** 1000₽ × 4 × 2 = **8,000₽**

## Миграция данных
Существующие тарифные планы с периодами `monthly` и `yearly` продолжат работать без изменений.
Новые периоды доступны для создания новых тарифных планов через UI.

## Обратная совместимость
✅ Полностью обратно совместимо с существующими данными.
✅ Все существующие подписки продолжат работать корректно.
✅ Пересчет сумм договоров автоматически применяется при создании/обновлении подписок.

