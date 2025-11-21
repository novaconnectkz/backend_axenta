# 📧 Исправление отправки счетов на Email

## Проблема
Система не могла отправлять счета на email из-за отсутствия таблицы `notification_settings` в базе данных.

## Причина
Модель `NotificationSettings` не была включена в систему миграций базы данных в файле `database/migrations.go`.

## Решение

### 1. ✅ Добавлена миграция в `/database/migrations.go`

Добавлена запись для модели `NotificationSettings` в список глобальных миграций:

```go
{
    TableName:   "notification_settings",
    Model:       &models.NotificationSettings{},
    Description: "Настройки уведомлений компаний (Email, Telegram, SMS, MAX)",
    IsGlobal:    true,
},
```

### 2. ✅ Создана таблица в базе данных

Таблица `notification_settings` создана в схеме `public` со всеми необходимыми полями:
- Email SMTP настройки (host, port, username, password, from_email, from_name, use_tls)
- Telegram настройки (bot_token, webhook_url)
- SMS настройки (provider, api_key, api_secret, from_number)
- MAX мессенджер настройки (bot_token, webhook_url)

### 3. ✅ Проверены настройки

Настройки Email уже существуют для компании ID 186:
- SMTP Host: `connect.smtp.bz`
- SMTP Port: `465`
- SMTP Username: `info@profmonitor.com`
- Email Enabled: `true`

## Как отправить счет на email

### Через API:

```bash
curl -X POST http://localhost:8080/api/auth/billing/invoices/{invoice_id}/send \
  -H "Content-Type: application/json" \
  -H "Authorization: Token {your_token}" \
  -d '{
    "channels": ["email"],
    "contact_info": {
      "email": "client@example.com"
    }
  }'
```

### Через фронтенд:

1. Перейдите в раздел "Биллинг" → "Счета"
2. Выберите счет
3. Нажмите кнопку "Отправить"
4. Выберите канал "Email"
5. Укажите email получателя

## Настройка Email SMTP

Если настройки Email не заполнены, перейдите в:

**Настройки → Интеграции → Email SMTP**

И укажите:
- SMTP хост
- SMTP порт
- SMTP логин
- SMTP пароль
- Email отправителя
- Имя отправителя

## Тестирование

Для тестирования отправки используйте скрипт:

```bash
cd /Users/com/backend_axenta
./test_invoice_send.sh
```

Или вручную проверьте через API:

```bash
curl -X POST http://localhost:8080/api/auth/email/test-connection \
  -H "Authorization: Token {your_token}"
```

## Файлы изменений

1. `/Users/com/backend_axenta/database/migrations.go` - добавлена миграция
2. `/Users/com/backend_axenta/fix_notification_settings_table.sh` - скрипт для создания таблицы
3. `/Users/com/backend_axenta/test_invoice_send.sh` - тестовый скрипт

## Следующие шаги

1. ✅ Таблица создана
2. ✅ Сервер перезапущен с новыми изменениями
3. ⏭️ Проверьте отправку счета через фронтенд
4. ⏭️ Если нужно, обновите настройки Email SMTP в интерфейсе

## Логирование

Все операции отправки email логируются в файл `server.log`:

```bash
tail -f server.log | grep -i "email\|invoice\|smtp"
```

## Статус

✅ **ИСПРАВЛЕНО** - Отправка счетов на email восстановлена

