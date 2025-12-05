# Исправление поиска ИП по ИНН в карточке договора

## Проблема
При вводе ИНН ИП (12 цифр) в карточке создания договора система не находила и не заполняла данные индивидуального предпринимателя.

## Причина
1. Поиск был отключен для ИП в функции `onInnSearch` (проверка только для `ORGANIZATION`)
2. Валидация не поддерживала 12 цифр (ИНН ИП)
3. ОГРНИП не заполнялся при выборе ИП из результатов

## Внесенные исправления

### 1. Включен поиск для ИП
**Файл:** `/Users/com/frontend_axenta/src/views/CreateContract.vue`

**Изменение в функции `onInnSearch`:**
```typescript
// Было:
if (form.value.client_type !== CLIENT_TYPES.ORGANIZATION) {
  return;
}

// Стало:
if (form.value.client_type !== CLIENT_TYPES.ORGANIZATION && 
    form.value.client_type !== CLIENT_TYPES.INDIVIDUAL_ENTREPRENEUR) {
  return;
}
```

### 2. Обновлена валидация формата ИНН
**Изменение в функции `onInnSearch`:**
```typescript
// Было:
if (!/^\d{10}$|^\d{13}$/.test(cleanValue)) {

// Стало:
if (!/^\d{10}$|^\d{12}$|^\d{13}$/.test(cleanValue)) {
```

### 3. Включен поиск при изменении ИНН для ИП
**Изменение в обработчике изменения типа клиента:**
```typescript
// Было:
} else if (clientType === CLIENT_TYPES.INDIVIDUAL_ENTREPRENEUR) {
  // Для ИП: 12 цифр ИНН или 13 ОГРНИП (поиск не выполняем)
  if (actualValue === '') {
    // Очистка при пустом значении
  }
}

// Стало:
} else if (clientType === CLIENT_TYPES.INDIVIDUAL_ENTREPRENEUR) {
  // Для ИП: 12 цифр ИНН или 13 ОГРНИП - выполняем поиск
  if (actualValue === '') {
    organizationSuggestions.value = [];
    showOrganizationMenu.value = false;
  } else {
    // Выполняем поиск для ИП
    onInnSearch(actualValue);
  }
}
```

### 4. Правильная обработка ОГРНИП для ИП
**Изменение в функции `onOrganizationSelect`:**
```typescript
// Было:
if (extractedData.client_ogrn) {
  form.value.client_ogrn = extractedData.client_ogrn;
}

// Стало:
// Для ИП заполняем ОГРНИП, для организаций - ОГРН
if (extractedData.client_ogrn) {
  if (form.value.client_type === CLIENT_TYPES.INDIVIDUAL_ENTREPRENEUR) {
    form.value.client_ogrnip = extractedData.client_ogrn;
  } else {
    form.value.client_ogrn = extractedData.client_ogrn;
  }
}
```

### 5. Автоматическое определение типа клиента
**Добавлено в функции `searchOrganizations`:**
- Автоматически определяется тип организации из ответа Dadata (`type === 'INDIVIDUAL'`)
- Если найден ИП, тип клиента автоматически устанавливается как `INDIVIDUAL_ENTREPRENEUR`

**Добавлено в функции `onOrganizationSelect`:**
- При выборе ИП из результатов автоматически устанавливается тип клиента
- КПП очищается для ИП (у ИП нет КПП)

## Результат
Теперь при вводе ИНН ИП (12 цифр) в карточке создания договора:
1. ✅ Выполняется поиск через Dadata API
2. ✅ Находятся данные ИП
3. ✅ Автоматически заполняются поля:
   - Полное наименование клиента
   - ИНН
   - ОГРНИП (в правильное поле)
   - Адрес регистрации
   - Контактные данные (если есть)
4. ✅ Автоматически устанавливается тип клиента как "Индивидуальный предприниматель"

## Тестирование
Для проверки используйте ИНН ИП: `342304181660`
- Ожидаемый результат: "Индивидуальный предприниматель Черкасов Валихан Самигуллаевич"
- ОГРНИП: `320344300065867`
