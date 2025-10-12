# 🔄 Исправление проблемы с миграциями на продакшене

## 📋 Описание проблемы

На продакшене возникают ошибки:
```
ERROR: relation "roles" does not exist (SQLSTATE 42P01)
ERROR: relation "user_templates" does not exist (SQLSTATE 42P01)
```

**Причина:** В скриптах деплоя на продакшен отсутствует выполнение миграций базы данных.

## 🔍 Анализ проблемы

1. **Локальная разработка работает** - миграции выполняются автоматически при запуске приложения через `database.ConnectDatabase()` → `RunAllMigrations(false)`

2. **Продакшен не работает** - скрипты деплоя (`deploy_production.sh`, `deploy-production.sh`) не выполняют миграции

3. **Отсутствующие таблицы:**
   - `roles` - роли пользователей
   - `user_templates` - шаблоны пользователей
   - `permissions` - разрешения
   - И другие тенантные таблицы

## 🚀 Решения

### Вариант 1: Быстрое исправление - Выполнить миграции отдельно

```bash
# Запустить скрипт миграций
./run_production_migrations.sh
```

Этот скрипт:
- Подключается к продакшн серверу
- Создает временный скрипт миграций
- Выполняет все необходимые миграции
- Перезапускает сервис
- Проверяет работоспособность API

### Вариант 2: Использовать улучшенный скрипт деплоя

```bash
# Использовать новый скрипт деплоя с миграциями
./deploy_production_with_migrations.sh
```

Этот скрипт включает:
- Все функции обычного деплоя
- Автоматическое выполнение миграций
- Проверку создания таблиц
- Тестирование API эндпоинтов

### Вариант 3: Ручное выполнение миграций на сервере

```bash
# Подключиться к серверу
ssh root@api.axenta.glonass-saratov.ru

# Перейти в директорию приложения
cd /opt/axenta-backend

# Создать скрипт миграций
cat > migrate.go << 'EOF'
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
	log.Println("🚀 Начинаем выполнение миграций базы данных")

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	dsn := cfg.GetDatabaseDSN()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	database.DB = db

	if err := database.RunAllMigrations(false); err != nil {
		log.Fatalf("❌ Ошибка выполнения миграций: %v", err)
	}

	log.Println("✅ Все миграции выполнены успешно")
}
EOF

# Собрать и выполнить миграции
go build -o migrate migrate.go
./migrate

# Перезапустить сервис
systemctl restart axenta-backend

# Очистить временные файлы
rm -f migrate.go migrate
```

## 🔧 Проверка результата

После выполнения миграций проверьте:

### 1. Статус сервиса
```bash
systemctl status axenta-backend
```

### 2. API эндпоинты
```bash
# Roles API
curl -I https://api.axenta.glonass-saratov.ru/api/auth/roles

# User Templates API  
curl -I https://api.axenta.glonass-saratov.ru/api/auth/user-templates
```

### 3. Логи приложения
```bash
journalctl -u axenta-backend -f
```

## 📊 Ожидаемые результаты

После успешного выполнения миграций:

- ✅ API эндпоинты `/api/auth/roles` и `/api/auth/user-templates` должны отвечать (не 500 ошибка)
- ✅ В логах не должно быть ошибок "relation does not exist"
- ✅ Фронтенд должен корректно загружать роли и шаблоны пользователей

## 🛡️ Предотвращение проблемы в будущем

### 1. Обновить основные скрипты деплоя

Добавить выполнение миграций в существующие скрипты:
- `deploy_production.sh`
- `deploy-production.sh`

### 2. Использовать новый скрипт

Заменить старые скрипты на `deploy_production_with_migrations.sh`

### 3. Автоматизация

Добавить проверку миграций в CI/CD pipeline:
```yaml
- name: Run Database Migrations
  run: |
    ssh ${{ secrets.PRODUCTION_USER }}@${{ secrets.PRODUCTION_SERVER }} \
    "cd /opt/axenta-backend && ./migrate"
```

## 📝 Логи для диагностики

Если проблемы продолжаются, соберите логи:

```bash
# Логи приложения
journalctl -u axenta-backend -n 100

# Логи PostgreSQL
journalctl -u postgresql -n 50

# Проверка подключения к БД
sudo -u postgres psql -c "\l" | grep axenta
```

## 🆘 Контакты для поддержки

При возникновении проблем:
1. Проверьте логи приложения и БД
2. Убедитесь, что все зависимости установлены
3. Проверьте настройки подключения к БД в `.env` файле
4. Обратитесь к команде разработки с логами

---

**Важно:** Всегда делайте резервную копию базы данных перед выполнением миграций на продакшене!
