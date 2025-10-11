# 🚀 Деплой API учетных записей на продакшен

## 🔍 Диагностика проблемы

**Проблема:** На продакшене не работает получение списка учетных записей.

**Причина:** API эндпоинт `/api/accounts` возвращает 404, что означает, что новые изменения backend не задеплоены на продакшен.

## ✅ Что уже исправлено в коде:

### 🔧 Backend изменения:
- ✅ Добавлен `handlers/accounts.go` - прокси к Axenta API
- ✅ Зарегистрированы эндпоинты в `main.go`:
  - `GET /api/accounts` - список учетных записей
  - `GET /api/accounts/:id` - конкретная учетная запись
- ✅ Поддержка авторизации и X-Tenant-ID
- ✅ Обработка ошибок и логирование

### 🎨 Frontend изменения:
- ✅ Исправлены эндпоинты в `accountsService.ts` (`/auth/accounts` → `/accounts`)
- ✅ Улучшена обработка токенов (поддержка разных ключей localStorage)
- ✅ Добавлено детальное логирование запросов

## 🧪 Результаты тестирования:

```bash
📊 Test Results Summary:
========================
✅ PASS backendHealth     - Сервер доступен
❌ FAIL accountsNoAuth    - 404 (эндпоинт не найден)
❌ FAIL accountsWithAuth  - Пропущен (нет токена)
✅ PASS cors             - CORS настроен правильно
```

**Вывод:** Backend сервер работает, CORS настроен, но API эндпоинты accounts отсутствуют.

## 🚀 Инструкции для деплоя:

### 1. **Подготовка сборки:**
```bash
cd /path/to/backend_axenta
go build -o main_production .
```

### 2. **Проверка сборки:**
```bash
# Проверить, что файл создан
ls -la main_production

# Проверить зависимости
go mod tidy
go mod verify
```

### 3. **Деплой на продакшен:**

#### Вариант A: Через SSH
```bash
# Скопировать файлы на сервер
scp main_production user@api.axenta.glonass-saratov.ru:/path/to/app/
scp handlers/accounts.go user@api.axenta.glonass-saratov.ru:/path/to/app/handlers/

# Подключиться к серверу
ssh user@api.axenta.glonass-saratov.ru

# Остановить текущий сервис
sudo systemctl stop axenta-backend

# Заменить исполняемый файл
sudo cp main_production /path/to/app/main
sudo chmod +x /path/to/app/main

# Запустить сервис
sudo systemctl start axenta-backend
sudo systemctl status axenta-backend
```

#### Вариант B: Через Docker
```bash
# Если используется Docker
docker build -t axenta-backend:latest .
docker stop axenta-backend-container
docker rm axenta-backend-container
docker run -d --name axenta-backend-container -p 8080:8080 axenta-backend:latest
```

### 4. **Проверка деплоя:**
```bash
# Проверить, что сервер запустился
curl https://api.axenta.glonass-saratov.ru/health

# Проверить новый эндпоинт (должен вернуть 401 вместо 404)
curl https://api.axenta.glonass-saratov.ru/api/accounts

# Запустить полный тест
node test-accounts-api.cjs
```

## 🔧 Файлы для деплоя:

### Обязательные файлы:
- `main_production` - исполняемый файл
- `handlers/accounts.go` - новый обработчик
- `main.go` - обновленный с новыми роутами

### Конфигурационные файлы:
- `.env` или переменные окружения
- `config/` папка (если используется)

## 📊 Ожидаемые результаты после деплоя:

```bash
📊 Test Results Summary:
========================
✅ PASS backendHealth     - Сервер доступен
✅ PASS accountsNoAuth    - 401 (требуется авторизация)
✅ PASS accountsWithAuth  - 200 (с валидным токеном)
✅ PASS cors             - CORS настроен правильно
```

## 🚨 Возможные проблемы:

### 1. **Порты и файрволл:**
- Убедиться, что порт 8080 открыт
- Проверить nginx конфигурацию для проксирования

### 2. **Права доступа:**
- Исполняемый файл должен иметь права на выполнение
- Пользователь должен иметь права на запуск сервиса

### 3. **Зависимости:**
- Go модули должны быть скачаны (`go mod download`)
- Переменные окружения должны быть настроены

## 📞 Контакты для поддержки:

После деплоя запустить тест и проверить логи:
```bash
# Проверить логи сервиса
sudo journalctl -u axenta-backend -f

# Запустить тест
cd /path/to/frontend_axenta
node test-accounts-api.cjs
```

---

**Статус:** 🔄 Готов к деплою  
**Приоритет:** 🔥 Высокий  
**Время деплоя:** ~10-15 минут
