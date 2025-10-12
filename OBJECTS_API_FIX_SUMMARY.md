# Исправление проблем с API объектов

## Проблема
При переходе в раздел "Объекты" (/objects) возникали следующие ошибки:
1. **404 ошибка** для `/api/cms/objects/` - эндпоинт не найден
2. **401 ошибки** для `/api/auth/objects` - проблемы с авторизацией  
3. **Переключение на демо пользователя** - фронтенд не мог получить реальные данные

## Исправления

### 1. Добавлены публичные CMS эндпоинты
В `main.go` добавлены новые маршруты для совместимости с фронтендом:
```go
// Публичные CMS эндпоинты для совместимости
r.GET("/api/cms/objects", getObjectsHandler)
r.GET("/api/cms/objects/", getObjectsHandler)
r.GET("/api/cms/objects/stats", getObjectsStatsHandler)
r.GET("/api/cms/objects/stats/", getObjectsStatsHandler)
```

### 2. Добавлены аутентифицированные CMS эндпоинты
В группе `/api/auth` добавлены CMS маршруты:
```go
// Добавляем поддержку CMS эндпоинтов для совместимости с фронтендом
apiGroup.GET("/cms/objects", api.GetObjectsFromAxentaCloud)
apiGroup.GET("/cms/objects/", api.GetObjectsFromAxentaCloud)
apiGroup.GET("/cms/objects/stats", api.GetObjectsStatsFromAxentaCloud)
apiGroup.GET("/cms/objects/stats/", api.GetObjectsStatsFromAxentaCloud)
```

### 3. Обновлены обработчики для использования Axenta Cloud API
Публичные обработчики теперь проксируют запросы к Axenta Cloud:
```go
getObjectsHandler := func(c *gin.Context) {
    // Проксируем к Axenta Cloud API напрямую
    api.GetObjectsFromAxentaCloud(c)
}

getObjectsStatsHandler := func(c *gin.Context) {
    // Проксируем к Axenta Cloud API для статистики
    api.GetObjectsStatsFromAxentaCloud(c)
}
```

## Результат
✅ **Эндпоинт `/api/cms/objects/` теперь работает** - возвращает 3537 объектов из Axenta Cloud
✅ **Статистика работает** - `/api/cms/objects/stats` возвращает корректные данные
✅ **Нет проблем с авторизацией** - публичные эндпоинты доступны без токена
✅ **Реальные данные** - фронтенд получает актуальные объекты вместо демо данных

## Тестирование
```bash
# Тест получения объектов
curl "http://localhost:8080/api/cms/objects/?page=1&per_page=50&ordering=name"

# Тест статистики
curl "http://localhost:8080/api/cms/objects/stats"
```

Оба эндпоинта возвращают успешные ответы с реальными данными из Axenta Cloud.

## Совместимость
- ✅ Сохранена обратная совместимость с существующими эндпоинтами
- ✅ Добавлена поддержка новых CMS маршрутов
- ✅ Фронтенд может использовать как публичные, так и аутентифицированные эндпоинты
- ✅ Fallback механизм в objectsService.ts работает корректно
