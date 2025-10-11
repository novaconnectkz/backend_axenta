# 🚀 Инструкции для ручного деплоя backend на продакшен

## 🔍 Текущая ситуация

**Проблема:** API эндпоинт `/api/accounts` возвращает 404 на продакшене  
**Причина:** Backend с новыми изменениями не задеплоен на сервер  
**Решение:** Необходимо задеплоить обновленный backend

## 📋 Что готово к деплою:

### ✅ Backend изменения:
- `handlers/accounts.go` - новый обработчик для прокси к Axenta API
- `main.go` - зарегистрированы новые эндпоинты `/api/accounts`
- `main_production` - готовая production сборка
- Все зависимости проверены

### ✅ Frontend изменения:
- Исправлены эндпоинты в `accountsService.ts`
- Улучшена обработка токенов авторизации
- Добавлено детальное логирование

## 🛠️ Пошаговые инструкции для деплоя:

### Шаг 1: Подготовка файлов
```bash
cd /Users/com/backend_axenta

# Проверить, что production сборка готова
ls -la main_production

# Если нет, создать сборку
GOOS=linux GOARCH=amd64 go build -o main_production .
```

### Шаг 2: Подключение к серверу
```bash
# Подключиться к продакшен серверу
ssh root@api.axenta.glonass-saratov.ru

# Или через другого пользователя, если root недоступен
ssh your-user@api.axenta.glonass-saratov.ru
```

### Шаг 3: Остановка текущего сервиса
```bash
# На сервере - остановить backend сервис
sudo systemctl stop axenta-backend

# Проверить статус
sudo systemctl status axenta-backend
```

### Шаг 4: Бэкап текущей версии
```bash
# Создать бэкап текущей версии
sudo cp /opt/axenta-backend/main /opt/axenta-backend/main_backup_$(date +%Y%m%d_%H%M%S)
```

### Шаг 5: Копирование новых файлов
```bash
# На локальной машине - скопировать файлы на сервер
scp main_production root@api.axenta.glonass-saratov.ru:/tmp/
scp -r handlers/ root@api.axenta.glonass-saratov.ru:/tmp/

# На сервере - переместить файлы
sudo mv /tmp/main_production /opt/axenta-backend/main
sudo chmod +x /opt/axenta-backend/main

# Обновить handlers (если нужно)
sudo cp -r /tmp/handlers/ /opt/axenta-backend/
```

### Шаг 6: Запуск сервиса
```bash
# Запустить сервис
sudo systemctl start axenta-backend

# Проверить статус
sudo systemctl status axenta-backend

# Посмотреть логи
sudo journalctl -u axenta-backend -f
```

### Шаг 7: Проверка работы API
```bash
# Проверить health endpoint
curl https://api.axenta.glonass-saratov.ru/health

# Проверить accounts endpoint (должен вернуть 401 вместо 404)
curl https://api.axenta.glonass-saratov.ru/api/accounts
```

## 🧪 Тестирование после деплоя:

### На локальной машине:
```bash
cd /Users/com/frontend_axenta
node test-accounts-api.cjs
```

### Ожидаемые результаты:
```
📊 Test Results Summary:
========================
✅ PASS backendHealth     - Сервер доступен
✅ PASS accountsNoAuth    - 401 (требуется авторизация) ← ИСПРАВЛЕНО!
⚠️  SKIP accountsWithAuth  - Пропущен (нет токена)
✅ PASS cors             - CORS настроен правильно
```

## 🔧 Альтернативные пути деплоя:

### Вариант A: Через Docker (если используется)
```bash
# Пересобрать Docker образ
docker build -t axenta-backend:latest .

# Остановить и удалить старый контейнер
docker stop axenta-backend
docker rm axenta-backend

# Запустить новый контейнер
docker run -d --name axenta-backend -p 8080:8080 axenta-backend:latest
```

### Вариант B: Через Git на сервере
```bash
# На сервере - обновить код из репозитория
cd /opt/axenta-backend
git pull origin main

# Пересобрать
go build -o main .

# Перезапустить сервис
sudo systemctl restart axenta-backend
```

## 🚨 Возможные проблемы и решения:

### 1. Сервис не запускается
```bash
# Проверить логи
sudo journalctl -u axenta-backend -n 50

# Проверить права на файл
ls -la /opt/axenta-backend/main
sudo chmod +x /opt/axenta-backend/main
```

### 2. API все еще возвращает 404
```bash
# Проверить, что новая версия запустилась
curl https://api.axenta.glonass-saratov.ru/health

# Проверить логи на наличие ошибок регистрации роутов
sudo journalctl -u axenta-backend | grep "accounts"
```

### 3. Проблемы с CORS
```bash
# Проверить CORS заголовки
curl -H "Origin: https://axenta.glonass-saratov.ru" \
     -H "Access-Control-Request-Method: GET" \
     -H "Access-Control-Request-Headers: authorization" \
     -X OPTIONS \
     https://api.axenta.glonass-saratov.ru/api/accounts
```

## 📞 Контакты и поддержка:

После деплоя обязательно:
1. ✅ Проверить статус сервиса
2. ✅ Запустить тест API
3. ✅ Проверить логи на ошибки
4. ✅ Протестировать frontend

**Время деплоя:** ~5-10 минут  
**Риск:** Низкий (есть бэкап)  
**Приоритет:** 🔥 Критический

---

## 🎯 Быстрый чеклист:

- [ ] Подключиться к серверу
- [ ] Остановить сервис
- [ ] Создать бэкап
- [ ] Скопировать новые файлы
- [ ] Запустить сервис
- [ ] Проверить API (401 вместо 404)
- [ ] Запустить тест

**После успешного деплоя API учетных записей заработает!** 🎉
