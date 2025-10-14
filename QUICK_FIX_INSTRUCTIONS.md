# 🚀 Быстрое исправление проблем с базой данных

## Проблема
После деплоя на продакшен сервер таблицы базы данных не создаются правильно.

## Решение (2 шага)

### Шаг 1: Диагностика
```bash
./diagnose_production_database.sh
```

### Шаг 2: Исправление
```bash
./fix_production_database.sh
```

## Что делают скрипты

### `diagnose_production_database.sh`
- ✅ Проверяет статус сервиса и PostgreSQL
- ✅ Анализирует логи
- ✅ Проверяет конфигурацию БД
- ✅ Тестирует подключение
- ✅ Проверяет существующие таблицы

### `fix_production_database.sh`
- 🔄 Останавливает сервис
- 💾 Создает резервную копию БД
- 🔧 Выполняет все миграции
- 🚀 Перезапускает сервис
- 🧪 Тестирует API

## Если скрипты не помогли

### Ручное выполнение миграций
```bash
ssh root@api.axenta.glonass-saratov.ru
cd /opt/axenta-backend

# Создайте скрипт миграций
cat > fix_db.go << 'EOF'
package main

import (
    "log"
    "backend_axenta/config"
    "backend_axenta/database"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

func main() {
    cfg, _ := config.LoadConfig()
    dsn := cfg.GetDatabaseDSN()
    db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Info)})
    database.DB = db
    database.RunAllMigrations(false)
    log.Println("✅ Готово!")
}
EOF

# Выполните
go build -o fix_db fix_db.go && ./fix_db && rm fix_db.go fix_db
systemctl restart axenta-backend
```

## Проверка результата
```bash
# Тест API
curl -s -o /dev/null -w "%{http_code}" https://api.axenta.glonass-saratov.ru/health
curl -s -o /dev/null -w "%{http_code}" https://api.axenta.glonass-saratov.ru/api/accounts
```

**Ожидаемый результат**: 200 или 401 (не 000 или 500)

## Если ничего не помогает
1. Проверьте логи: `journalctl -u axenta-backend -f`
2. Проверьте подключение к БД вручную
3. Обратитесь к разработчикам
