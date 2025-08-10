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

- `GET /ping` - проверка работоспособности
- `GET /api/objects` - получение списка всех объектов

## Запуск

1. Установите PostgreSQL
2. Создайте файл `.env` на основе `env.example`
3. Настройте подключение к базе данных
4. Запустите: `go run main.go`

Сервер будет доступен на `http://localhost:8080`

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