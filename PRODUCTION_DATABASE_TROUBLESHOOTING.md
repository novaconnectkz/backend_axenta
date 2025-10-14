# 🔧 Решение проблем с базой данных на продакшен сервере

## Обзор проблемы

После деплоя на продакшен сервер таблицы базы данных не создаются правильно. Это может быть связано с различными факторами:

1. **Проблемы с подключением к БД**
2. **Неправильная конфигурация переменных окружения**
3. **Ошибки в процессе миграций**
4. **Проблемы с правами доступа**
5. **Отсутствие необходимых схем**

## Быстрое решение

### 1. Диагностика проблемы

Запустите скрипт диагностики для выявления проблем:

```bash
./diagnose_production_database.sh
```

Этот скрипт проверит:
- ✅ Статус сервиса axenta-backend
- ✅ Логи сервиса
- ✅ Конфигурацию переменных окружения
- ✅ Подключение к PostgreSQL
- ✅ Структуру базы данных
- ✅ Наличие необходимых таблиц

### 2. Автоматическое исправление

Если диагностика выявила проблемы, запустите скрипт исправления:

```bash
./fix_production_database.sh
```

Этот скрипт:
- 🔄 Остановит сервис для безопасного выполнения миграций
- 💾 Создаст резервную копию базы данных
- 🔧 Выполнит все необходимые миграции
- 🚀 Перезапустит сервис
- 🧪 Протестирует API endpoints

## Ручное решение

Если автоматические скрипты не помогли, выполните следующие шаги вручную:

### 1. Подключение к серверу

```bash
ssh root@api.axenta.glonass-saratov.ru
cd /opt/axenta-backend
```

### 2. Проверка конфигурации

```bash
# Проверьте наличие .env файла
ls -la .env

# Проверьте основные переменные БД
grep -E '^DB_(HOST|PORT|NAME|USER|SSLMODE)=' .env
```

### 3. Проверка подключения к PostgreSQL

```bash
# Проверьте статус PostgreSQL
systemctl status postgresql

# Проверьте подключение к БД
source .env
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "SELECT 'Connected successfully';"
```

### 4. Выполнение миграций вручную

```bash
# Создайте скрипт миграций
cat > migrate_manual.go << 'EOF'
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
    log.Println("🚀 Ручной запуск миграций")
    
    cfg, err := config.LoadConfig()
    if err != nil {
        log.Fatal(err)
    }
    
    dsn := cfg.GetDatabaseDSN()
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Info),
    })
    if err != nil {
        log.Fatal(err)
    }
    
    database.DB = db
    
    if err := database.RunAllMigrations(false); err != nil {
        log.Fatal(err)
    }
    
    log.Println("✅ Миграции выполнены")
}
EOF

# Соберите и запустите
go build -o migrate_manual migrate_manual.go
./migrate_manual

# Очистите временные файлы
rm migrate_manual.go migrate_manual
```

### 5. Проверка созданных таблиц

```bash
# Проверьте глобальные таблицы
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "
SELECT table_name FROM information_schema.tables 
WHERE table_schema = 'public' 
AND table_name IN ('companies', 'billing_plans', 'subscriptions', 'integrations', 'integration_errors', 'local_users', 'refresh_tokens')
ORDER BY table_name;
"

# Проверьте тенантные схемы
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "
SELECT schema_name FROM information_schema.schemata 
WHERE schema_name LIKE 'tenant_%' 
ORDER BY schema_name;
"

# Проверьте тенантные таблицы
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "
SELECT schemaname, tablename FROM pg_tables 
WHERE schemaname LIKE 'tenant_%' 
AND tablename IN ('users', 'roles', 'permissions', 'user_templates', 'objects', 'contracts', 'equipment')
ORDER BY schemaname, tablename;
"
```

### 6. Перезапуск сервиса

```bash
systemctl restart axenta-backend
systemctl status axenta-backend
```

### 7. Тестирование API

```bash
# Проверьте health endpoint
curl -s -o /dev/null -w "%{http_code}" https://api.axenta.glonass-saratov.ru/health

# Проверьте accounts API
curl -s -o /dev/null -w "%{http_code}" https://api.axenta.glonass-saratov.ru/api/accounts

# Проверьте roles API
curl -s -o /dev/null -w "%{http_code}" https://api.axenta.glonass-saratov.ru/api/auth/roles
```

## Частые проблемы и решения

### 1. Ошибка подключения к БД

**Проблема**: `connection refused` или `authentication failed`

**Решение**:
```bash
# Проверьте статус PostgreSQL
systemctl status postgresql

# Проверьте настройки в .env
grep -E '^DB_' .env

# Проверьте права пользователя БД
sudo -u postgres psql -c "\du"
```

### 2. Таблицы не создаются

**Проблема**: Миграции выполняются, но таблицы не появляются

**Решение**:
```bash
# Проверьте права на создание таблиц
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "
SELECT has_database_privilege('$DB_USER', '$DB_NAME', 'CREATE');
"

# Проверьте схему по умолчанию
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "SHOW search_path;"
```

### 3. Ошибки миграций

**Проблема**: Ошибки при выполнении AutoMigrate

**Решение**:
```bash
# Проверьте логи миграций
journalctl -u axenta-backend -f

# Выполните миграции с подробным логированием
./migrate_manual 2>&1 | tee migration.log
```

### 4. Проблемы с тенантными схемами

**Проблема**: Тенантные таблицы не создаются

**Решение**:
```bash
# Создайте схему вручную
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "CREATE SCHEMA IF NOT EXISTS tenant_test;"

# Проверьте права на схему
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "
SELECT has_schema_privilege('$DB_USER', 'tenant_test', 'CREATE');
"
```

## Профилактика

### 1. Регулярные проверки

Создайте cron задачу для регулярной проверки:

```bash
# Добавьте в crontab
0 */6 * * * /opt/axenta-backend/diagnose_production_database.sh >> /var/log/axenta-db-check.log 2>&1
```

### 2. Мониторинг

Настройте мониторинг:
- Статус сервиса axenta-backend
- Подключение к PostgreSQL
- Наличие ключевых таблиц
- Использование места на диске

### 3. Резервное копирование

Настройте автоматическое резервное копирование:

```bash
# Добавьте в crontab для ежедневного бэкапа
0 2 * * * PGPASSWORD=$DB_PASSWORD pg_dump -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME > /backup/axenta_$(date +\%Y\%m\%d).sql
```

## Контакты и поддержка

При возникновении проблем:
1. Запустите диагностику: `./diagnose_production_database.sh`
2. Проверьте логи: `journalctl -u axenta-backend -f`
3. Обратитесь к разработчикам с результатами диагностики

---

**Последнее обновление**: $(date)
**Версия**: 1.0
