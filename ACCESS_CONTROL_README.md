# Контроль доступа к CRM Axenta

## Обзор

CRM система Axenta реализует контроль доступа на основе типа аккаунта пользователя в Axenta Cloud. Доступ к системе разрешен только пользователям с определенными типами аккаунтов.

## Типы аккаунтов Axenta

### ✅ Разрешенные типы:
- **`partner`** - Партнерский аккаунт (полный доступ к CRM)

### ❌ Запрещенные типы:
- **`client`** - Клиентский аккаунт (доступ запрещен)
- Любые другие типы аккаунтов

## Реализация

### Backend (Go)

#### Основная авторизация (`/api/auth/login`)
```go
// В api/auth.go после получения данных пользователя от Axenta
if axentaUser.AccountType != "partner" {
    logAuthOperation("login_access_denied", req.Username, userIDStr, "", map[string]interface{}{
        "status":       "access_denied",
        "account_type": axentaUser.AccountType,
        "reason":       "only_partners_allowed",
    })
    c.JSON(403, gin.H{
        "status": "error", 
        "error":  "Доступ к CRM разрешен только партнерам Axenta",
        "details": gin.H{
            "account_type": axentaUser.AccountType,
            "required_type": "partner",
        },
    })
    return
}
```

#### Локальная авторизация (`/api/local/login`)
```go
// В api/local_auth.go при создании пользователя из Axenta
if axentaUser.AccountType != "partner" {
    logLocalAuthOperation("login_access_denied", req.Username, "", "", map[string]interface{}{
        "status":       "access_denied",
        "account_type": axentaUser.AccountType,
        "reason":       "only_partners_allowed",
    })
    c.JSON(http.StatusForbidden, gin.H{
        "status": "error",
        "error":  "Доступ к CRM разрешен только партнерам Axenta",
        "details": gin.H{
            "account_type":  axentaUser.AccountType,
            "required_type": "partner",
        },
    })
    return
}
```

### Frontend (Vue.js)

#### Обработка ошибок доступа
```typescript
// В src/context/auth.ts
} else if (lastError.response?.status === 403) {
  errorMessage = lastError.response.data?.error || "Доступ запрещен. Проверьте права доступа.";
```

#### Компонент AccessDeniedDialog
- Показывает информативное сообщение об ошибке доступа
- Отображает текущий и требуемый тип аккаунта
- Предоставляет ссылку на техподдержку

## Логирование

Все попытки доступа логируются с подробной информацией:

### Успешный доступ:
```json
{
  "operation": "login_success_full",
  "status": "success",
  "account_type": "partner",
  "account_name": "GLOMOS",
  "username": "glomos"
}
```

### Отказ в доступе:
```json
{
  "operation": "login_access_denied", 
  "status": "access_denied",
  "account_type": "client",
  "reason": "only_partners_allowed",
  "username": "client_user"
}
```

## Тестирование

### Тест партнерского аккаунта:
```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"partner_user","password":"password"}'
```

**Ожидаемый результат**: HTTP 200, полные данные пользователя

### Тест клиентского аккаунта:
```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"client_user","password":"password"}'
```

**Ожидаемый результат**: HTTP 403, сообщение об отказе в доступе

## Безопасность

1. **Проверка на уровне API**: Контроль доступа происходит сразу после получения данных от Axenta
2. **Логирование**: Все попытки доступа записываются в лог для аудита
3. **Информативные ошибки**: Пользователи получают понятные сообщения об ошибках
4. **Fallback защита**: Проверка работает как для основной, так и для локальной авторизации

## Расширение

Для добавления новых разрешенных типов аккаунтов:

1. Обновите условие в `api/auth.go`:
```go
if axentaUser.AccountType != "partner" && axentaUser.AccountType != "premium" {
    // отказ в доступе
}
```

2. Обновите условие в `api/local_auth.go` аналогично

3. Добавьте новые типы в frontend компоненты для корректного отображения

## Мониторинг

Для мониторинга попыток несанкционированного доступа используйте:

```bash
# Поиск отказов в доступе в логах
grep "login_access_denied" server*.log

# Статистика по типам аккаунтов
grep "account_type" server*.log | grep -E "(success|denied)"
```
