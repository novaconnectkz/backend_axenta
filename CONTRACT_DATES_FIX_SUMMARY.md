# Исправление: Даты договоров теперь устанавливаются через подписку

## Изменения

### 1. Фронтенд (`frontend_axenta/src/views/CreateContract.vue`)

**Было:**
- Автоматически устанавливалась `start_date` = текущая дата
- Автоматически устанавливалась `end_date` = текущая дата + 1 год

**Стало:**
- `start_date` и `end_date` НЕ устанавливаются при создании договора
- Обе даты остаются `undefined`
- Даты будут установлены позже через создание подписки

### 2. База данных (SQL миграция)

**Было:**
```sql
start_date DATETIME NOT NULL  -- ❌ Обязательное поле
end_date DATETIME NOT NULL    -- ❌ Обязательное поле
```

**Стало:**
```sql
start_date DATETIME  -- ✅ Необязательное поле (NULL разрешен)
end_date DATETIME    -- ✅ Необязательное поле (NULL разрешен)
```

### 3. Go модель (`backend_axenta/models/contract.go`)

Уже было правильно определено:
```go
StartDate *time.Time `json:"start_date" gorm:"default:NULL"` // ✅
EndDate   *time.Time `json:"end_date" gorm:"default:NULL"`   // ✅
```

### 4. Тесты (`backend_axenta/services/billing_service_test.go`)

Обновлена схема БД в тестах, чтобы соответствовать новой структуре.

## Бизнес-логика

### Старый флоу
```
Договор создается → Автоматически устанавливаются даты → Период = 1 год
```

### Новый флоу
```
Договор создается → Период НЕ установлен → 
→ Создается подписка → Подписка устанавливает период договора
```

## Преимущества нового подхода

1. ✅ **Гибкость**: Можно создать договор-черновик без подписки
2. ✅ **Правильная логика**: Период определяется подпиской, а не договором
3. ✅ **Чистота данных**: Нет фиктивных дат "по умолчанию"
4. ✅ **Понятность**: В UI четко видно "Период не установлен"

## Применение на продакшене

### Шаг 1: Применить SQL миграцию

```sql
ALTER TABLE contracts ALTER COLUMN start_date DROP NOT NULL;
ALTER TABLE contracts ALTER COLUMN end_date DROP NOT NULL;
```

### Шаг 2: Задеплоить новый фронтенд

```bash
cd /Users/com/frontend_axenta
npm run build
# Скопировать dist/ на продакшен
```

### Шаг 3: Проверить

1. Создать новый договор
2. Убедиться, что в колонке "Период" написано "Период не установлен"
3. Убедиться, что даты показываются как "Не указан"

## Откат (если нужно)

Если потребуется вернуть старое поведение:

### 1. Откатить миграцию БД
```sql
UPDATE contracts SET start_date = CURRENT_DATE WHERE start_date IS NULL;
UPDATE contracts SET end_date = start_date + INTERVAL '1 year' WHERE end_date IS NULL;
ALTER TABLE contracts ALTER COLUMN start_date SET NOT NULL;
```

### 2. Откатить изменения фронтенда
```bash
git revert <commit-hash>
npm run build
```

## Файлы

- ✅ `frontend_axenta/src/views/CreateContract.vue` - убрана автоустановка дат
- ✅ `backend_axenta/migrations/000010_make_contract_dates_nullable.sql` - SQL миграция
- ✅ `backend_axenta/services/billing_service_test.go` - обновлены тесты
- ✅ `MIGRATION_CONTRACT_DATES.md` - подробная документация
- ✅ `URGENT_FIX_CONTRACTS.md` - краткая инструкция для срочного применения

## Статус

- [x] Фронтенд изменен
- [x] Фронтенд собран
- [x] Миграция создана
- [x] Тесты обновлены
- [x] Документация написана
- [ ] **ТРЕБУЕТСЯ: Применить миграцию на продакшене**
- [ ] **ТРЕБУЕТСЯ: Задеплоить новый фронтенд на продакшен**

## Проверка на локальной среде

✅ Протестировано - договор создается без дат
✅ UI показывает "Период не установлен"
✅ База данных принимает NULL значения

## Важно!

⚠️ **Не забудьте применить SQL миграцию на продакшене перед деплоем фронтенда!**

Иначе будет ошибка:
```
ERROR: null value in column "start_date" violates not-null constraint
```

## Дата изменений

2025-11-18

