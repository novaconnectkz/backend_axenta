.PHONY: help build run test clean up down logs sqlc migrate-up migrate-down

# Переменные
APP_NAME=axenta_server
DOCKER_COMPOSE=docker compose

help: ## Показать справку
	@echo "🚀 Axenta Backend - Makefile команды"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ============================================================================
# Основные команды для разработки
# ============================================================================

run: ## Запустить приложение локально (без Docker)
	go run ./main.go

dev-watch: ## Запустить с авто-перезапуском при смене git HEAD (`git pull` / `git fetch && reset` подхватываются)
	@./scripts/dev-watch.sh

test: ## Запустить тесты
	go test ./... -v

test-coverage: ## Запустить тесты с покрытием
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "📊 Отчет о покрытии: coverage.html"

# База данных
migrate-up: ## Применить миграции
	@echo "🔄 Применяем миграции..."
	go run ./cmd/migrate/main.go

migrate-down: ## Откатить миграции (требует реализации)
	@echo "⚠️ Откат миграций не реализован"

# SQLC (если используется)
sqlc: ## Сгенерировать код из SQL запросов (если используется sqlc)
	@if command -v sqlc > /dev/null; then \
		sqlc generate; \
	else \
		echo "⚠️ sqlc не установлен. Установите: go install github.com/kyleconroy/sqlc/cmd/sqlc@latest"; \
	fi

# Сборка бинарника
build-binary: ## Собрать бинарник локально
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o $(APP_NAME) ./main.go
	@echo "✅ Бинарник создан: $(APP_NAME)"

clean: ## Очистить временные файлы
	rm -f $(APP_NAME)
	rm -f coverage.out coverage.html
	$(DOCKER_COMPOSE) down -v
	@echo "✅ Временные файлы очищены"

# Docker очистка
docker-clean: ## Очистить Docker образы и контейнеры
	$(DOCKER_COMPOSE) down -v --rmi all --remove-orphans
	docker system prune -f
	@echo "✅ Docker образы и контейнеры очищены"

# Разработка
dev: ## Запустить в режиме разработки (локально + Docker для БД, если нужен)
	@echo "💡 Для локальной разработки:"
	@echo "   1. Установите PostgreSQL локально"
	@echo "   2. Настройте .env файл"
	@echo "   3. Запустите: make run"
	@echo ""
	@echo "   Или используйте Docker только для БД (опционально):"
	@echo "   $(DOCKER_COMPOSE) up -d postgres redis"
	@echo "   make run"

# ============================================================================
# Продакшен сборка (как на реальном сервере)
# ============================================================================

prod-build: ## Собрать бинарник для продакшена (linux/amd64)
	$(eval COMMIT_COUNT := $(shell git rev-list --count HEAD))
	$(eval COMMIT_HASH := $(shell git rev-parse --short HEAD))
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s -X main.gitCommitCount=$(COMMIT_COUNT) -X main.gitCommitHash=$(COMMIT_HASH)" -o axenta_backend_linux main.go
	@echo "✅ Бинарник создан: axenta_backend_linux (commit=$(COMMIT_COUNT) hash=$(COMMIT_HASH))"
	@ls -lh axenta_backend_linux

prod-build-local: ## Собрать бинарник для локальной ОС
	$(eval COMMIT_COUNT := $(shell git rev-list --count HEAD))
	$(eval COMMIT_HASH := $(shell git rev-parse --short HEAD))
	go build -ldflags="-w -s -X main.gitCommitCount=$(COMMIT_COUNT) -X main.gitCommitHash=$(COMMIT_HASH)" -o axenta_backend main.go
	@echo "✅ Бинарник создан: axenta_backend (commit=$(COMMIT_COUNT) hash=$(COMMIT_HASH))"

# ============================================================================
# Docker (ОПЦИОНАЛЬНО - только для экспериментов, не используется в продакшене)
# ============================================================================

docker-build: ## [Docker] Собрать Docker образ (опционально)
	$(DOCKER_COMPOSE) build

up: ## [Docker] Запустить все сервисы через Docker - ОПЦИОНАЛЬНО
	@echo "⚠️  Docker используется только для экспериментов"
	@echo "   Для продакшена используется: systemd + Go binary (см. DEPLOYMENT.md)"
	$(DOCKER_COMPOSE) up -d
	@echo "✅ Сервисы запущены:"
	@echo "  - App: http://localhost:8080"
	@echo "  - pgAdmin: http://localhost:5050 (admin@axenta.local / admin)"
	@echo "  - PostgreSQL: localhost:5432"
	@echo "  - Redis: localhost:6379"

down: ## [Docker] Остановить все сервисы
	$(DOCKER_COMPOSE) down

logs: ## [Docker] Показать логи всех сервисов
	$(DOCKER_COMPOSE) logs -f

logs-app: ## [Docker] Показать логи приложения
	$(DOCKER_COMPOSE) logs -f app

logs-db: ## [Docker] Показать логи базы данных
	$(DOCKER_COMPOSE) logs -f postgres

restart: ## [Docker] Перезапустить все сервисы
	$(DOCKER_COMPOSE) restart

ps: ## [Docker] Показать статус сервисов
	$(DOCKER_COMPOSE) ps

# Линтинг
lint: ## Запустить golangci-lint
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "⚠️ golangci-lint не установлен. Установите: https://golangci-lint.run/usage/install/"; \
	fi

fmt: ## Форматировать код
	go fmt ./...

# Проверка перед коммитом
pre-commit: fmt test lint ## Запустить все проверки перед коммитом
	@echo "✅ Все проверки пройдены"

