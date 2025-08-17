# Система уведомлений Axenta CRM

Полнофункциональная система уведомлений с поддержкой Telegram Bot, Email, SMS и настраиваемых шаблонов.

## Возможности

### 🤖 Telegram Bot

- Интеграция с Telegram Bot API
- Обработка входящих сообщений и команд
- Inline клавиатуры для интерактивности
- Webhook и polling режимы

### 📧 Email уведомления

- SMTP интеграция с поддержкой TLS
- HTML шаблоны писем
- Настраиваемые отправители

### 📱 SMS уведомления

- Поддержка различных SMS провайдеров
- Заглушка для интеграции с конкретными провайдерами

### 🎨 Система шаблонов

- Настраиваемые шаблоны с плейсхолдерами
- Поддержка HTML для email
- Многоязычность
- Приоритеты и повторные попытки

### 🔄 Fallback механизмы

- Автоматическое переключение между каналами
- Учет предпочтений пользователей
- Тихие часы
- Отложенная отправка

### 📊 Мониторинг и логирование

- Полное логирование всех уведомлений
- Статистика по каналам и типам
- Повторные попытки с экспоненциальной задержкой

## Архитектура

### Основные компоненты

1. **NotificationService** - основной сервис для отправки уведомлений
2. **TelegramClient** - клиент для работы с Telegram Bot API
3. **NotificationFallbackService** - fallback логика и обработка ошибок
4. **NotificationAPI** - REST API для управления уведомлениями

### Модели данных

- `NotificationTemplate` - шаблоны уведомлений
- `NotificationLog` - логи отправленных уведомлений
- `NotificationSettings` - настройки уведомлений компании
- `UserNotificationPreferences` - предпочтения пользователей

## Настройка

### 1. Telegram Bot

```bash
# Создайте бота через @BotFather в Telegram
# Получите токен бота

# Установите токен в настройках компании
curl -X PUT http://localhost:8080/api/notifications/settings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "telegram_enabled": true,
    "telegram_bot_token": "YOUR_BOT_TOKEN"
  }'
```

### 2. Email (SMTP)

```bash
curl -X PUT http://localhost:8080/api/notifications/settings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "email_enabled": true,
    "smtp_host": "smtp.gmail.com",
    "smtp_port": 587,
    "smtp_username": "your-email@gmail.com",
    "smtp_password": "your-password",
    "smtp_from_email": "noreply@yourcompany.com",
    "smtp_from_name": "Axenta CRM",
    "smtp_use_tls": true
  }'
```

### 3. SMS

```bash
curl -X PUT http://localhost:8080/api/notifications/settings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "sms_enabled": true,
    "sms_provider": "twilio",
    "sms_api_key": "YOUR_API_KEY",
    "sms_api_secret": "YOUR_API_SECRET",
    "sms_from_number": "+1234567890"
  }'
```

## API Endpoints

### Логи уведомлений

- `GET /api/notifications/logs` - получить логи уведомлений
- `GET /api/notifications/statistics` - статистика по уведомлениям

### Шаблоны

- `GET /api/notifications/templates` - получить шаблоны
- `POST /api/notifications/templates` - создать шаблон
- `PUT /api/notifications/templates/:id` - обновить шаблон
- `DELETE /api/notifications/templates/:id` - удалить шаблон
- `POST /api/notifications/templates/defaults` - создать шаблоны по умолчанию

### Настройки

- `GET /api/notifications/settings` - получить настройки компании
- `PUT /api/notifications/settings` - обновить настройки компании
- `GET /api/notifications/preferences` - получить предпочтения пользователя
- `PUT /api/notifications/preferences` - обновить предпочтения пользователя

### Тестирование

- `POST /api/notifications/test` - отправить тестовое уведомление

### Webhook

- `POST /api/notifications/telegram/webhook/:company_id` - webhook для Telegram

## Использование

### Отправка простого уведомления

```go
templateData := map[string]interface{}{
    "UserName": "Иван Иванов",
    "Message": "Ваш монтаж запланирован на завтра в 10:00",
    "Address": "ул. Пушкина, д. 1",
}

err := notificationService.SendNotification(
    "installation_reminder",  // тип уведомления
    "telegram",              // канал
    "123456789",            // получатель (Telegram ID)
    templateData,           // данные для шаблона
    companyID,              // ID компании
    installationID,         // ID связанной сущности
    "installation"          // тип связанной сущности
)
```

### Отправка с fallback

```go
err := fallbackService.SendWithFallback(
    "billing_alert",
    "user@example.com",  // получатель
    templateData,
    companyID,
    invoiceID,
    "invoice"
)
```

### Создание шаблона

```go
template := models.NotificationTemplate{
    Name:        "Напоминание о платеже",
    Type:        "payment_reminder",
    Channel:     "telegram",
    Subject:     "",
    Template:    "💰 <b>Напоминание о платеже</b>\n\nСумма: {{.Amount}} руб.\nСрок: {{.DueDate}}\n\nОплатить можно по ссылке: {{.PaymentURL}}",
    Description: "Напоминание о предстоящем платеже",
    Priority:    "high",
    CompanyID:   companyID,
}
```

## Типы уведомлений

### Монтажи и установка

- `installation_reminder` - напоминание о монтаже
- `installation_created` - новый монтаж создан
- `installation_updated` - монтаж обновлен
- `installation_completed` - монтаж завершен
- `installation_cancelled` - монтаж отменен
- `installation_rescheduled` - монтаж перенесен

### Биллинг

- `billing_alert` - уведомление о биллинге
- `invoice_created` - счет создан
- `payment_reminder` - напоминание о платеже
- `payment_overdue` - просроченный платеж

### Склад

- `stock_alert` - низкий остаток на складе
- `warranty_alert` - истечение гарантии
- `maintenance_alert` - требуется обслуживание
- `equipment_movement` - движение оборудования

### Система

- `system_notification` - системное уведомление
- `security_alert` - уведомление безопасности
- `emergency_alert` - экстренное уведомление

## Шаблоны по умолчанию

Система автоматически создает следующие шаблоны:

### Telegram шаблоны

```
🔧 Напоминание о монтаже
📅 Дата: {{.Date}}
⏰ Время: {{.Time}}
📍 Адрес: {{.Address}}
🏢 Объект: {{.Object.Name}}
📞 Контакт клиента: {{.ClientContact}}
```

### Email шаблоны

```html
<h2>{{.Title}}</h2>
<p>{{.Description}}</p>
<p><b>Уровень важности:</b> {{.Severity}}</p>
```

## Telegram Bot команды

- `/start` - начать работу с ботом
- `/help` - справка по командам
- `/status` - проверить статус подключения
- `/settings` - настройки уведомлений

## Тестирование

### Запуск тестов

```bash
# Все тесты
go test ./services -v

# Только тесты уведомлений
go test ./services -run TestNotification -v

# Benchmark тесты
go test ./services -bench=BenchmarkNotification -v
```

### Тестовое уведомление

```bash
curl -X POST http://localhost:8080/api/notifications/test \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "channel": "telegram",
    "recipient": "123456789",
    "message": "Тестовое сообщение",
    "subject": "Тест"
  }'
```

## Мониторинг и отладка

### Просмотр логов

```bash
# Получить последние 100 уведомлений
curl "http://localhost:8080/api/notifications/logs?limit=100" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# Фильтр по типу
curl "http://localhost:8080/api/notifications/logs?type=installation_reminder" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# Фильтр по статусу
curl "http://localhost:8080/api/notifications/logs?status=failed" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

### Статистика

```bash
curl "http://localhost:8080/api/notifications/statistics" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

Ответ:

```json
{
  "status": "success",
  "data": {
    "total": 1250,
    "sent": 1180,
    "failed": 45,
    "pending": 25,
    "by_channel": [
      { "channel": "telegram", "count": 850 },
      { "channel": "email", "count": 300 },
      { "channel": "sms", "count": 100 }
    ],
    "by_type": [
      { "type": "installation_reminder", "count": 500 },
      { "type": "billing_alert", "count": 300 },
      { "type": "stock_alert", "count": 200 }
    ]
  }
}
```

## Производительность

### Оптимизация

1. **Кэширование** - Telegram клиенты кэшируются по company_id
2. **Асинхронная отправка** - уведомления отправляются в горутинах
3. **Batch обработка** - группировка повторных попыток
4. **Connection pooling** - переиспользование SMTP соединений

### Benchmark результаты

```
BenchmarkNotificationService_SendNotification-8     1000    1.2ms/op
BenchmarkNotificationService_RenderTemplate-8       5000    0.3ms/op
```

## Безопасность

### Защита данных

- Шифрование токенов в базе данных
- Валидация входящих webhook
- Rate limiting для API endpoints

### Конфиденциальность

- Маскирование чувствительных данных в логах
- Удаление старых логов уведомлений
- Контроль доступа к настройкам

## Troubleshooting

### Частые проблемы

1. **Telegram бот не отвечает**

   - Проверьте токен бота
   - Убедитесь, что бот не заблокирован
   - Проверьте настройки webhook

2. **Email не доставляются**

   - Проверьте SMTP настройки
   - Убедитесь в правильности TLS конфигурации
   - Проверьте спам-фильтры

3. **Уведомления не отправляются**
   - Проверьте активность шаблонов
   - Убедитесь в правильности типа уведомления
   - Проверьте предпочтения пользователя

### Логи

```bash
# Проверка логов приложения
tail -f /var/log/axenta/app.log | grep -i notification

# Поиск ошибок в базе данных
SELECT * FROM notification_logs WHERE status = 'failed' ORDER BY created_at DESC LIMIT 10;
```

## Расширение функциональности

### Добавление нового канала

1. Создайте интерфейс для нового канала
2. Реализуйте клиент для канала
3. Добавьте обработку в `sendNotificationWithTemplate`
4. Добавьте настройки в `NotificationSettings`
5. Создайте тесты

### Добавление нового типа уведомления

1. Определите тип в константах
2. Создайте шаблоны по умолчанию
3. Добавьте обработку в соответствующий сервис
4. Обновите документацию

## Roadmap

- [ ] Push уведомления для мобильных приложений
- [ ] Интеграция с Microsoft Teams
- [ ] Slack интеграция
- [ ] WhatsApp Business API
- [ ] Расширенная аналитика и дашборды
- [ ] A/B тестирование шаблонов
- [ ] Машинное обучение для оптимизации времени отправки
