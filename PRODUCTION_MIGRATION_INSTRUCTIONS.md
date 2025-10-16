# Инструкция по выполнению миграций на продакшен сервере

## Проблема
Ошибка миграции таблицы `companies` с сообщением "insufficient arguments" на продакшен сервере.

## Решение

### 1. Обновить конфигурацию базы данных

В файле `env.production` убедитесь, что используются правильные настройки:

```bash
# Пользователь базы данных
DB_USER=axenta_user

# Пароль базы данных (замените на реальный пароль)
DB_PASSWORD=your_actual_database_password

# Переменные для миграций
DATABASE_HOST=localhost
DATABASE_PORT=5432
DATABASE_NAME=axenta_db
DATABASE_USER=axenta_user
DATABASE_PASSWORD=your_actual_database_password

# Режим SSL для подключения к БД
DB_SSLMODE=disable
```

### 2. Выполнить миграции в принудительном режиме

Используйте новый скрипт для принудительных миграций:

```bash
./run_production_migrations_force.sh
```

Или выполните команду напрямую:

```bash
go run cmd/migrate/main.go -force
```

### 3. Проверить результат

После выполнения миграций проверьте, что таблица `user_tokens` создана:

```bash
psql -U axenta_user -d axenta_db -c "SELECT tablename FROM pg_tables WHERE tablename = 'user_tokens';"
```

### 4. Проверить в тенантной схеме

```bash
psql -U axenta_user -d axenta_db -c "SET search_path TO tenant_default; SELECT tablename FROM pg_tables WHERE tablename = 'user_tokens';"
```

## Ожидаемый результат

После успешного выполнения миграций должны быть созданы:

- ✅ Таблица `user_tokens` в схеме `public` (глобальная)
- ✅ Таблица `user_tokens` в схеме `tenant_default` (тенантная)
- ✅ Все необходимые поля и индексы

## Примечания

- Ошибка "insufficient arguments" для таблицы `companies` игнорируется в принудительном режиме
- Резервная копия базы данных создается автоматически перед миграцией
- Флаг `-force` позволяет продолжить выполнение миграций даже при ошибках
