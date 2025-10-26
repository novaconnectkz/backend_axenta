#!/bin/bash

echo "🚀 Подготовка к деплою Axenta CRM..."

# Проверяем, что мы в правильной директории
if [ ! -f "main.go" ]; then
    echo "❌ Ошибка: Запустите скрипт из директории backend_axenta"
    exit 1
fi

echo "📦 Создаем архив для деплоя..."

# Создаем временную директорию для деплоя
DEPLOY_DIR="axenta_deploy_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$DEPLOY_DIR"

# Копируем только необходимые файлы backend
echo "📁 Копируем backend файлы..."
cp -r api/ "$DEPLOY_DIR/"
cp -r cmd/ "$DEPLOY_DIR/"
cp -r config/ "$DEPLOY_DIR/"
cp -r database/ "$DEPLOY_DIR/"
cp -r handlers/ "$DEPLOY_DIR/"
cp -r middleware/ "$DEPLOY_DIR/"
cp -r models/ "$DEPLOY_DIR/"
cp -r services/ "$DEPLOY_DIR/"
cp -r types/ "$DEPLOY_DIR/"
cp -r testutils/ "$DEPLOY_DIR/"
cp -r examples/ "$DEPLOY_DIR/"
cp -r scripts/ "$DEPLOY_DIR/"

# Копируем основные файлы
cp main.go "$DEPLOY_DIR/"
cp go.mod "$DEPLOY_DIR/"
cp go.sum "$DEPLOY_DIR/"
cp openapi.yaml "$DEPLOY_DIR/"
cp README.md "$DEPLOY_DIR/"
cp QUICK_START.md "$DEPLOY_DIR/"
cp QUICK_TEST.md "$DEPLOY_DIR/"
cp env.example "$DEPLOY_DIR/"
cp env.production.example "$DEPLOY_DIR/"
cp .gitignore "$DEPLOY_DIR/"
cp .cursorignore "$DEPLOY_DIR/"
cp .cursorrules "$DEPLOY_DIR/"

# Копируем скрипты деплоя
cp deploy_production.sh "$DEPLOY_DIR/" 2>/dev/null || echo "⚠️ deploy_production.sh не найден"
cp deploy_production_with_migrations.sh "$DEPLOY_DIR/" 2>/dev/null || echo "⚠️ deploy_production_with_migrations.sh не найден"
cp run_migrations.sh "$DEPLOY_DIR/" 2>/dev/null || echo "⚠️ run_migrations.sh не найден"
cp safe_migrate.sh "$DEPLOY_DIR/" 2>/dev/null || echo "⚠️ safe_migrate.sh не найден"

# Копируем frontend (только необходимые файлы)
echo "📁 Копируем frontend файлы..."
mkdir -p "$DEPLOY_DIR/frontend"
cd ../frontend_axenta

# Копируем основные файлы frontend
cp -r src/ "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ src/ не найден"
cp -r public/ "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ public/ не найден"
cp -r dist/ "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ dist/ не найден"
cp package.json "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ package.json не найден"
cp package-lock.json "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ package-lock.json не найден"
cp vite.config.ts "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ vite.config.ts не найден"
cp tsconfig.json "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ tsconfig.json не найден"
cp tsconfig.app.json "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ tsconfig.app.json не найден"
cp tsconfig.node.json "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ tsconfig.node.json не найден"
cp index.html "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ index.html не найден"
cp README.md "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ README.md не найден"
cp env.example "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ env.example не найден"
cp env.production.example "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ env.production.example не найден"
cp .gitignore "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ .gitignore не найден"
cp .cursorignore "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ .cursorignore не найден"
cp .cursorrules "$DEPLOY_DIR/frontend/" 2>/dev/null || echo "⚠️ .cursorrules не найден"

cd ../backend_axenta

# Создаем архив
echo "📦 Создаем архив..."
tar -czf "${DEPLOY_DIR}.tar.gz" "$DEPLOY_DIR/"

# Удаляем временную директорию
rm -rf "$DEPLOY_DIR"

echo "✅ Архив создан: ${DEPLOY_DIR}.tar.gz"
echo ""
echo "📊 Содержимое архива:"
echo "   Backend:"
echo "     - Исходный код Go (api/, cmd/, config/, database/, etc.)"
echo "     - Основные файлы (main.go, go.mod, go.sum)"
echo "     - Конфигурация (openapi.yaml, env.example)"
echo "     - Документация (README.md, QUICK_START.md)"
echo "     - Скрипты деплоя"
echo ""
echo "   Frontend:"
echo "     - Исходный код Vue.js (src/)"
echo "     - Публичные файлы (public/)"
echo "     - Собранная версия (dist/)"
echo "     - Конфигурация (package.json, vite.config.ts)"
echo "     - Документация (README.md)"
echo ""
echo "🚀 Архив готов для деплоя!"
echo "   Размер: $(du -h ${DEPLOY_DIR}.tar.gz | cut -f1)"
echo "   Файл: ${DEPLOY_DIR}.tar.gz"
