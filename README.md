# Backend Axenta

Backend часть проекта Axenta, построенная на Go с использованием Gin и GORM.

## Функциональность

- **API для объектов** - управление GPS-трекерами и устройствами
- **База данных PostgreSQL** - автомиграция моделей
- **GORM ORM** - для работы с базой данных
- **Gin Web Framework** - HTTP сервер и роутинг
- **CORS поддержка** - для работы с фронтендом

## Модели

### Object

- `Name` - название объекта (varchar 100)
- `Latitude` - широта (float64)
- `Longitude` - долгота (float64)
- `IMEI` - уникальный идентификатор устройства

## API Endpoints

### Публичные

- `GET /ping` - проверка работоспособности
- `POST /api/auth/login` - авторизация пользователя

### Объекты мониторинга

- `GET /api/objects` - получение списка объектов с фильтрацией и пагинацией
- `GET /api/objects/:id` - получение конкретного объекта по ID
- `POST /api/objects` - создание нового объекта
- `PUT /api/objects/:id` - обновление существующего объекта
- `DELETE /api/objects/:id` - мягкое удаление объекта

### Плановое удаление объектов

- `PUT /api/objects/:id/schedule-delete` - запланировать удаление объекта на указанную дату
- `PUT /api/objects/:id/cancel-delete` - отменить плановое удаление объекта

### Корзина объектов

- `GET /api/objects-trash` - получение списка удаленных объектов
- `PUT /api/objects/:id/restore` - восстановление объекта из корзины
- `DELETE /api/objects/:id/permanent` - окончательное удаление объекта

### Шаблоны объектов

- `GET /api/object-templates` - получение списка шаблонов объектов
- `GET /api/object-templates/:id` - получение конкретного шаблона по ID
- `POST /api/object-templates` - создание нового шаблона объекта
- `PUT /api/object-templates/:id` - обновление существующего шаблона
- `DELETE /api/object-templates/:id` - удаление шаблона объекта

### Пользователи и роли

- `GET /api/users` - получение списка пользователей
- `GET/POST/PUT/DELETE /api/users/*` - управление пользователями
- `GET/POST/PUT/DELETE /api/roles/*` - управление ролями
- `GET/POST /api/permissions/*` - управление правами доступа
- `GET/POST/PUT/DELETE /api/user-templates/*` - управление шаблонами пользователей

## Запуск

### Локальная разработка

1. Установите PostgreSQL и Redis (если нужен)
2. Создайте файл `.env` на основе `env.example`
3. Настройте подключение к базе данных
4. Запустите:
   ```bash
   make run
   ```
   
   Или напрямую:
   ```bash
   go run main.go
   ```

Сервер будет доступен на `http://localhost:8080`

### Полезные команды Makefile

```bash
make help          # Список всех доступных команд
make run           # Запустить приложение локально
make test          # Запустить тесты
make test-coverage # Тесты с покрытием
make build-binary   # Собрать бинарник для продакшена
make lint          # Проверить код линтером
make fmt           # Форматировать код
make pre-commit    # Все проверки перед коммитом
```

## Инфраструктура проекта

Проект использует классический подход без контейнеризации:

- **Systemd** - управление сервисом (`axenta-backend.service`)
- **Nginx** - reverse proxy перед приложением
- **PostgreSQL** - база данных на сервере
- **Go binary** - бинарник, собираемый на сервере
- **SSH деплой** - автоматический через GitHub Actions
- **Bash скрипты** - автоматизация деплоя (`deploy_production.sh`)

Подробнее о продакшен развертывании: см. `QUICK_START.md`

### Docker (Опционально, только для локальной разработки)

Docker файлы присутствуют для экспериментов, но не используются в продакшене:
```bash
make up    # Запустить все сервисы через Docker (только для локальной разработки)
make down  # Остановить сервисы
make logs  # Просмотр логов
```

**Важно**: Для продакшена используется обычный Go binary через systemd + Nginx.

## Структура проекта

```
backend_axenta/
├── api/           # HTTP handlers
├── database/      # Подключение к БД и миграции
├── handlers/      # Шаблоны handlers
├── models/        # Модели данных
├── main.go        # Точка входа
└── go.mod         # Go модули
```
