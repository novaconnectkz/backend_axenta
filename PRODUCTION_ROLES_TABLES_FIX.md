# 🔧 Исправление отсутствующих таблиц roles и user_templates на продакшене

## 📋 Проблема

На продакшене в разделе "Пользователь" перестала отображаться колонка "Роль" из-за ошибки:
```
ERROR: relation "roles" does not exist (SQLSTATE 42P01)
ERROR: relation "user_templates" does not exist (SQLSTATE 42P01)
```

Фронтенд пытается получить доступ к эндпоинтам:
- `GET /api/public/roles?page=1&limit=100&active_only=true`
- `GET /api/public/user-templates?page=1&limit=100&active_only=true`

## 🎯 Решение

### 1. Подготовка к исправлению

Убедитесь, что у вас есть доступ к серверу продакшена и файл конфигурации:

```bash
# На сервере продакшена создайте файл env.production
cp env.production.example env.production
# Отредактируйте env.production с правильными данными БД
```

### 2. Быстрое исправление (рекомендуется)

Запустите скрипт автоматического исправления:

```bash
# На сервере продакшена
./fix_production_roles_tables.sh
```

Этот скрипт:
- ✅ Создаст отсутствующие таблицы: `roles`, `permissions`, `role_permissions`, `user_templates`
- ✅ Добавит базовые роли: admin, manager, tech, accountant, user
- ✅ Добавит базовые разрешения для всех ресурсов
- ✅ Установит связи между ролями и разрешениями
- ✅ Создаст шаблоны пользователей
- ✅ Добавит индексы для оптимизации

### 3. Альтернативное решение через миграции

Если предпочитаете использовать систему миграций:

```bash
# Создайте резервную копию
./backup_before_migration.sh

# Запустите миграции
./run_production_migrations.sh
```

### 4. Ручное исправление через SQL

Если скрипты не работают, выполните SQL команды напрямую:

```sql
-- Подключитесь к базе данных продакшена
psql -h YOUR_DB_HOST -U YOUR_DB_USER -d YOUR_DB_NAME

-- Выполните SQL скрипт из fix_production_roles_tables.sh
-- (скопируйте содержимое SQL_SCRIPT из скрипта)
```

## 📊 Проверка результата

После исправления проверьте:

1. **Таблицы созданы:**
```sql
SELECT tablename FROM pg_tables 
WHERE tablename IN ('roles', 'permissions', 'role_permissions', 'user_templates')
ORDER BY tablename;
```

2. **Роли добавлены:**
```sql
SELECT name, display_name, is_active FROM roles ORDER BY priority DESC;
```

3. **API работает:**
```bash
curl -X GET "https://api.axenta.glonass-saratov.ru/api/public/roles?page=1&limit=10"
curl -X GET "https://api.axenta.glonass-saratov.ru/api/public/user-templates?page=1&limit=10"
```

## 🎉 Ожидаемый результат

После исправления:
- ✅ Колонка "Роль" снова появится в разделе "Пользователь"
- ✅ API эндпоинты `/api/public/roles` и `/api/public/user-templates` будут работать
- ✅ Фронтенд сможет загружать список ролей и шаблонов пользователей
- ✅ Система ролей и разрешений будет полностью функциональна

## 🔍 Дополнительная диагностика

Если проблема остается:

1. **Проверьте логи сервера:**
```bash
tail -f server.log | grep -i "roles\|templates"
```

2. **Проверьте подключение к БД:**
```bash
./verify_migrations.sh production
```

3. **Проверьте права доступа к таблицам:**
```sql
SELECT grantee, privilege_type 
FROM information_schema.table_privileges 
WHERE table_name IN ('roles', 'permissions', 'user_templates');
```

## 📞 Поддержка

Если проблема не решается:
1. Сохраните логи ошибок
2. Выполните проверку миграций
3. Создайте резервную копию БД
4. Обратитесь к разработчику с подробным описанием проблемы
