# Включение планировщика снимков на продакшене

## Проблема
Планировщик снимков не работал на продакшене, потому что не было переменной окружения для его включения.

## Решение
Добавлена переменная окружения `ENABLE_SNAPSHOT_SCHEDULER` для управления планировщиком.

## Инструкция по применению на продакшене

### Вариант 1: Автоматический (если установлен sshpass)

```bash
./enable_snapshot_scheduler_production.sh
```

### Вариант 2: Ручной

1. Подключитесь к продакшен серверу:
```bash
ssh root@194.87.143.169
# Пароль: g-t+XM#3an2YJM
```

2. Отредактируйте файл `.env`:
```bash
cd /opt/axenta/backend
nano .env
```

3. Добавьте или обновите следующие строки:
```bash
# Планировщик снимков
ENABLE_SNAPSHOT_SCHEDULER=true

# Системный токен для Axenta Cloud API (ОБЯЗАТЕЛЬНО!)
AXENTA_ADMIN_TOKEN=ваш_токен_здесь
```

4. Сохраните файл (Ctrl+O, Enter, Ctrl+X в nano)

5. Перезапустите сервис:
```bash
sudo systemctl restart axenta-backend
```

6. Проверьте статус:
```bash
sudo systemctl status axenta-backend
```

7. Проверьте логи, что планировщик запустился:
```bash
sudo journalctl -u axenta-backend -n 50 | grep -i "snapshot\|снимк"
```

Должны увидеть сообщение:
```
✅ Partner Snapshot Scheduler started (daily at 21:20 UTC / 00:20 MSK)
```

## Важно!

⚠️ **Обязательно установите `AXENTA_ADMIN_TOKEN`** - без него планировщик не сможет создавать снимки!

Токен можно получить из настроек Axenta Cloud API или из существующего токена пользователя.

## Проверка работы

После перезапуска проверьте логи:
```bash
sudo journalctl -u axenta-backend -f
```

Планировщик будет создавать снимки каждый день в 21:20 UTC (00:20 MSK следующего дня).
