# 🔧 Руководство по настройке переменных окружения Axenta CRM

## 📋 Обзор

Данное руководство описывает правильную настройку переменных окружения для корректной работы системы Axenta CRM (backend и frontend).

## 🔴 Критические исправления

### ✅ Что было исправлено:

1. **Backend**: Добавлена централизованная система конфигурации с автоматической загрузкой `.env` файлов
2. **Backend**: Исправлена проблема с отсутствием использования `JWT_SECRET`
3. **Backend**: Добавлена валидация критически важных переменных для продакшена
4. **Frontend**: Добавлена недостающая переменная `VITE_WS_BASE_URL` для WebSocket соединений
5. **Frontend**: Расширена конфигурация с дополнительными параметрами
6. **Общее**: Синхронизированы примеры env файлов между разработкой и продакшеном

## 🚀 Backend Configuration

### 📁 Структура конфигурации

```
backend_axenta/
├── .env                    # Основной файл конфигурации (не коммитится)
├── env.example            # Пример для разработки
├── env.production.example # Пример для продакшена
└── config/
    └── config.go          # Централизованная система конфигурации
```

### 🔧 Основные переменные Backend

#### 🏢 Приложение

```bash
# Режим работы (development, production, testing)
APP_ENV=development

# Порт и хост
APP_PORT=8080
APP_HOST=0.0.0.0

# URL бэкенда (внешний адрес)
BACKEND_URL=http://localhost:8080

# Версия API
API_VERSION=v1

# Режим отладки
DEBUG_MODE=false
```

#### 🗄️ База данных PostgreSQL

```bash
# Подключение к БД
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_secure_password
DB_NAME=axenta_db
DB_SSLMODE=disable

# Пул соединений
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME=300s
```

#### 🔴 Redis

```bash
# Подключение к Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_URL=redis://localhost:6379/0
REDIS_TIMEOUT=5s
REDIS_MAX_CONNECTIONS=10
```

#### 🔐 JWT и Безопасность

```bash
# JWT настройки (КРИТИЧЕСКИ ВАЖНО!)
JWT_SECRET=your_super_secret_jwt_key_here_must_be_at_least_32_characters_long
JWT_EXPIRES_IN=24h
JWT_REFRESH_EXPIRES_IN=168h
JWT_ISSUER=axenta-crm
```

#### 🔗 Axenta Cloud Integration

```bash
# Интеграция с Axenta Cloud
AXENTA_API_URL=https://api.axetna.cloud
AXENTA_TIMEOUT=30s
AXENTA_MAX_RETRIES=3

# Ключ шифрования (КРИТИЧЕСКИ ВАЖНО!)
ENCRYPTION_KEY=your-32-character-encryption-key!!
```

#### 🌐 CORS

```bash
# CORS настройки
CORS_ALLOWED_ORIGINS=https://yourdomain.com,https://www.yourdomain.com
CORS_ALLOWED_METHODS=GET,POST,PUT,DELETE,OPTIONS
CORS_ALLOWED_HEADERS=Content-Type,Authorization,X-Requested-With,Accept,Origin
CORS_ALLOW_CREDENTIALS=true
CORS_MAX_AGE=86400
```

#### 📊 Логирование

```bash
# Настройки логирования
LOG_LEVEL=info
LOG_FORMAT=json
LOG_FILE=
LOG_MAX_SIZE=100
LOG_MAX_BACKUPS=10
LOG_MAX_AGE=30
```

### ⚠️ Критически важные переменные для продакшена

```bash
# ЭТИ ПЕРЕМЕННЫЕ ОБЯЗАТЕЛЬНЫ В ПРОДАКШЕНЕ:
JWT_SECRET=              # Минимум 32 символа
ENCRYPTION_KEY=          # Минимум 32 символа
DB_PASSWORD=             # Надежный пароль БД
```

## 🎨 Frontend Configuration

### 📁 Структура конфигурации

```
frontend_axenta/
├── .env.local             # Локальный файл конфигурации (не коммитится)
├── env.example           # Пример для разработки
├── env.production.example # Пример для продакшена
└── src/config/
    └── env.ts            # Централизованная система конфигурации
```

### 🔧 Основные переменные Frontend

```bash
# URL бэкенда
VITE_BACKEND_URL=http://localhost:8080

# WebSocket URL для реального времени
VITE_WS_BASE_URL=ws://localhost:8080

# Название приложения
VITE_APP_NAME=Axenta CRM

# Версия API
VITE_API_VERSION=v1

# Режим приложения
VITE_APP_ENV=development

# Таймаут для API запросов (в миллисекундах)
VITE_API_TIMEOUT=30000
```

### 📝 Примеры для продакшена

```bash
# Продакшен настройки
VITE_BACKEND_URL=http://194.87.143.169:8080
VITE_WS_BASE_URL=ws://194.87.143.169:8080
VITE_APP_ENV=production
VITE_API_TIMEOUT=10000
```

## 🚀 Быстрый старт

### 1️⃣ Backend Setup

```bash
cd backend_axenta

# Скопируйте пример конфигурации
cp env.example .env

# Отредактируйте .env файл
nano .env

# Установите зависимости
go mod tidy

# Запустите сервер
go run main.go
```

### 2️⃣ Frontend Setup

```bash
cd frontend_axenta

# Скопируйте пример конфигурации
cp env.example .env.local

# Отредактируйте .env.local файл
nano .env.local

# Установите зависимости
npm install

# Запустите dev сервер
npm run dev
```

## 🔒 Безопасность

### ⚠️ Важные правила:

1. **Никогда не коммитьте файлы `.env` и `.env.local`**
2. **В продакшене всегда используйте сильные пароли и ключи**
3. **JWT_SECRET и ENCRYPTION_KEY должны быть минимум 32 символа**
4. **Регулярно меняйте секретные ключи**
5. **Используйте HTTPS в продакшене**

### 🔐 Генерация секретных ключей:

```bash
# Генерация JWT секрета
openssl rand -base64 32

# Генерация ключа шифрования
openssl rand -hex 16
```

## 🐛 Диагностика проблем

### Backend не запускается:

1. Проверьте наличие `.env` файла
2. Убедитесь что все критические переменные заданы
3. Проверьте подключение к БД и Redis
4. Посмотрите логи на предмет ошибок валидации

### Frontend не подключается к Backend:

1. Проверьте `VITE_BACKEND_URL` в `.env.local`
2. Убедитесь что backend запущен на указанном порту
3. Проверьте CORS настройки в backend
4. Откройте Developer Tools для просмотра ошибок сети

### WebSocket не работает:

1. Проверьте `VITE_WS_BASE_URL` в `.env.local`
2. Убедитесь что WebSocket сервер запущен
3. Проверьте firewall и proxy настройки

## 📚 Дополнительные ресурсы

- [Документация Vite по env переменным](https://vitejs.dev/guide/env-and-mode.html)
- [Документация godotenv](https://github.com/joho/godotenv)
- [Безопасность переменных окружения](https://12factor.net/config)

## 🔄 Миграция с старой системы

Если вы обновляетесь с предыдущей версии:

1. Создайте новые `.env` файлы по примерам
2. Перенесите существующие настройки
3. Добавьте новые обязательные переменные
4. Перезапустите приложения

---

_Последнее обновление: январь 2025_
_Версия системы: 1.0.0_
