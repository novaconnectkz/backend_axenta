# 🔑 Как получить токен пользователя для тестирования

## 📋 Инструкция

### Способ 1: Через браузер (рекомендуется)

1. **Откройте приложение в браузере**
2. **Войдите в систему** с вашими учетными данными Axenta
3. **Откройте DevTools** (F12)
4. **Перейдите на вкладку Application/Storage**
5. **Найдите localStorage**
6. **Скопируйте значение ключа `axenta_token`**

### Способ 2: Через Network tab

1. **Откройте приложение в браузере**
2. **Войдите в систему**
3. **Откройте DevTools** (F12) → **Network**
4. **Обновите страницу пользователей**
5. **Найдите запрос** к `/api/auth/users`
6. **В Headers найдите** `Authorization: Token ВАШТОКЕН`
7. **Скопируйте токен** (без "Token ")

### Способ 3: Через Console

1. **Откройте DevTools** (F12) → **Console**
2. **Выполните команду**:
   ```javascript
   localStorage.getItem('axenta_token')
   ```
3. **Скопируйте результат**

## 🧪 Тестирование с токеном

После получения токена выполните:

```bash
# Замените YOUR_TOKEN на полученный токен
TOKEN="YOUR_TOKEN"

# Тест 1: Проверка ролей в системе
curl "http://localhost:8080/api/auth/test/roles" \
  -H "Authorization: Token $TOKEN" | jq '.'

# Тест 2: Получение пользователей с ролями
curl "http://localhost:8080/api/auth/users?page=1&per_page=3" \
  -H "Authorization: Token $TOKEN" | jq '.data.items[] | {username, account_type, role_id, role: .role.display_name}'
```

## ✅ Ожидаемый результат

### Тест ролей должен показать:
```json
{
  "status": "success",
  "data": {
    "current_roles_count": 3,
    "roles": [
      {"name": "partner", "display_name": "Партнер"},
      {"name": "client", "display_name": "Клиент"},
      {"name": "user", "display_name": "Пользователь"}
    ]
  }
}
```

### Пользователи должны иметь роли:
```json
{
  "username": "partner_user",
  "account_type": "partner",
  "role_id": 1,
  "role": "Партнер"
}
```

## 🎯 Если все работает в API, но не в интерфейсе:

1. **Очистите кэш браузера** (Ctrl+Shift+R)
2. **Проверьте Console на ошибки** JavaScript
3. **Убедитесь, что frontend использует правильный API endpoint**
4. **Перезапустите frontend** приложение

## 📞 Готов помочь!

Если роли все еще не отображаются после этих шагов, покажите мне:
1. **Ответ API** из Network tab
2. **Ошибки из Console** (если есть)
3. **Логи сервера** при запросе пользователей

Тогда я смогу точно определить, где проблема! 🔍
