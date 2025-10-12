# 🔍 Руководство по проверке миграций базы данных Axenta CRM

Данное руководство описывает систему проверки и безопасного выполнения миграций базы данных для проекта Axenta CRM.

## 📋 Обзор системы

Система включает в себя:

1. **Проверка миграций** - анализ текущего состояния БД
2. **Резервное копирование** - создание backup'ов перед миграциями  
3. **Безопасное выполнение** - миграции с проверками и откатом
4. **Мониторинг** - отчеты и логирование процесса

## 🛠️ Компоненты системы

### 1. Скрипт проверки миграций (`verify_migrations.sh`)

Проверяет состояние миграций в локальной и/или продакшен базе данных.

```bash
# Проверка локальной БД
./verify_migrations.sh local

# Проверка продакшен БД
./verify_migrations.sh production

# Проверка обеих БД с сохранением отчетов
./verify_migrations.sh both --save-report
```

**Что проверяется:**
- Существование всех таблиц
- Структура таблиц (колонки, индексы)
- Количество записей в таблицах
- Соответствие схем компаний
- Целостность мультитенантной архитектуры

### 2. Скрипт резервного копирования (`backup_before_migration.sh`)

Создает резервные копии базы данных перед выполнением миграций.

```bash
# Резервная копия локальной БД
./backup_before_migration.sh local

# Резервная копия продакшен БД со сжатием
./backup_before_migration.sh production --compress
```

**Особенности:**
- Автоматическое сжатие backup'ов
- Проверка доступности БД перед копированием
- Скачивание backup'ов с продакшен сервера
- Информация о размере созданных файлов

### 3. Скрипт безопасных миграций (`safe_migrate.sh`)

Выполняет миграции с полным циклом проверок и возможностью отката.

```bash
# Безопасная миграция локальной БД
./safe_migrate.sh local

# Безопасная миграция продакшен БД
./safe_migrate.sh production

# Принудительная миграция без подтверждений
./safe_migrate.sh production --force --skip-backup
```

**Этапы выполнения:**
1. Проверка текущего состояния БД
2. Создание резервной копии
3. Выполнение миграций
4. Проверка результата
5. Тестирование API (для продакшена)

### 4. Программа проверки (`cmd/verify_migration/main.go`)

Go-программа для детальной проверки состояния миграций.

```bash
# Проверка с выводом в JSON
go run cmd/verify_migration/main.go --production --output report.json

# Проверка локальной БД
go run cmd/verify_migration/main.go --env development
```

## 📊 Структура отчетов

### JSON отчет содержит:

```json
{
  "environment": "production",
  "database_info": {
    "host": "localhost",
    "port": "5432",
    "database_name": "axenta_db",
    "version": "PostgreSQL 14.x",
    "connected": true
  },
  "global_tables": [
    {
      "table_name": "companies",
      "description": "Таблица компаний (мультитенантность)",
      "exists": true,
      "column_count": 15,
      "index_count": 3,
      "record_count": 5,
      "status": "ok",
      "issues": []
    }
  ],
  "tenant_schemas": [
    {
      "company_id": 1,
      "company_name": "Test Company",
      "schema_name": "tenant_company1",
      "is_active": true,
      "tables": [...],
      "status": "ok"
    }
  ],
  "summary": {
    "total_tables": 25,
    "tables_ok": 23,
    "tables_with_warning": 2,
    "tables_with_error": 0,
    "total_companies": 3,
    "active_companies": 2,
    "overall_status": "warning"
  },
  "recommendations": [
    "Проверьте структуру таблицы users: Отсутствует колонка axenta_user_type"
  ]
}
```

## 🚀 Быстрый старт

### 1. Проверка локальной разработки

```bash
# Полная проверка локальной БД
./verify_migrations.sh local --save-report

# Если есть проблемы - выполнить миграции
./safe_migrate.sh local
```

### 2. Подготовка к деплою на продакшен

```bash
# 1. Проверить текущее состояние продакшена
./verify_migrations.sh production --save-report

# 2. Создать резервную копию
./backup_before_migration.sh production --compress

# 3. Выполнить безопасную миграцию
./safe_migrate.sh production

# 4. Проверить результат
./verify_migrations.sh production --save-report
```

### 3. Сравнение окружений

```bash
# Проверить оба окружения и сравнить
./verify_migrations.sh both --save-report
```

## ⚠️ Важные моменты

### Безопасность

1. **Всегда создавайте backup** перед миграциями на продакшене
2. **Проверяйте доступность сервера** перед удаленными операциями
3. **Тестируйте API** после миграций на продакшене
4. **Имейте план отката** на случай проблем

### Мультитенантность

Система учитывает мультитенантную архитектуру:
- **Глобальные таблицы** в схеме `public`
- **Тенантные таблицы** в схемах компаний (`tenant_companyX`)
- **Автоматическое создание схем** для новых компаний

### Мониторинг

- Все операции логируются с временными метками
- Создаются детальные JSON отчеты
- Отслеживается статус каждой таблицы
- Генерируются рекомендации по исправлению

## 🔧 Настройка окружения

### Переменные окружения

```bash
# Локальная БД
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=axenta_db

# Продакшен БД (в .env.production)
DB_HOST=prod_host
DB_PORT=5432
DB_USER=prod_user
DB_PASSWORD=prod_password
DB_NAME=axenta_db
```

### SSH доступ к продакшену

```bash
# Настройте SSH ключи для доступа к серверу
ssh-copy-id root@api.axenta.glonass-saratov.ru

# Проверьте доступ
ssh root@api.axenta.glonass-saratov.ru "echo 'Доступ работает'"
```

## 📝 Коды возврата

- `0` - Успешно, все проверки пройдены
- `1` - Ошибка, требуется вмешательство
- `2` - Предупреждения, рекомендуется проверка

## 🆘 Устранение проблем

### Проблема: Таблица не существует

```bash
# Выполнить миграции
./run_migrations.sh

# Или для продакшена
./run_production_migrations.sh
```

### Проблема: Отсутствуют колонки

```bash
# Проверить модели в коде
# Выполнить AutoMigrate для конкретной модели
go run create_tables.go
```

### Проблема: Схема компании не существует

```bash
# Создать схему через API или напрямую в БД
# Или использовать функцию CreateTenantSchema
```

### Проблема: Продакшен сервер недоступен

```bash
# Проверить SSH ключи
ssh-add ~/.ssh/id_rsa

# Проверить доступность сервера
ping api.axenta.glonass-saratov.ru

# Проверить SSH соединение
ssh -v root@api.axenta.glonass-saratov.ru
```

## 📚 Дополнительные ресурсы

- [MIGRATION_SYSTEM_README.md](MIGRATION_SYSTEM_README.md) - Подробности системы миграций
- [DATABASE_MIGRATION_GUIDE.md](DATABASE_MIGRATION_GUIDE.md) - Руководство по миграциям
- [MULTITENANCY_README.md](MULTITENANCY_README.md) - Мультитенантная архитектура

## 🤝 Поддержка

При возникновении проблем:

1. Проверьте логи выполнения скриптов
2. Изучите JSON отчеты с деталями ошибок
3. Проверьте доступность БД и сервера
4. Обратитесь к документации по конкретным ошибкам

---

*Последнее обновление: $(date +"%Y-%m-%d")*
