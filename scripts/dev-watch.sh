#!/usr/bin/env bash
# dev-watch.sh — поднимает backend и автоматически перезапускает при смене git HEAD.
# Аналог git-version-watch плагина в vite.config.ts на фронте: после `git pull` или новый commit
# процесс ребилдится, чтобы /api/version отдавал свежий commit count + код был актуальным.
#
# Использование: make dev-watch (или ./scripts/dev-watch.sh)

set -e

cd "$(dirname "$0")/.."

LAST_HASH=""
PID=""

start_app() {
    go run ./main.go &
    PID=$!
    echo "[dev-watch] started PID=$PID, hash=${LAST_HASH:0:7}"
}

stop_app() {
    if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
        kill "$PID" 2>/dev/null || true
        wait "$PID" 2>/dev/null || true
    fi
    PID=""
}

cleanup() {
    echo ""
    echo "[dev-watch] остановка"
    stop_app
    exit 0
}
trap cleanup INT TERM

LAST_HASH=$(git rev-parse HEAD 2>/dev/null || echo "init")
start_app

while true; do
    sleep 5
    CUR=$(git rev-parse HEAD 2>/dev/null || echo "")
    if [ -n "$CUR" ] && [ "$CUR" != "$LAST_HASH" ]; then
        echo "[dev-watch] HEAD changed ${LAST_HASH:0:7} → ${CUR:0:7}, restart"
        stop_app
        LAST_HASH="$CUR"
        sleep 1
        start_app
    fi

    # Если процесс упал сам — рестартуем (чтобы не висел dev-watch без приложения)
    if [ -n "$PID" ] && ! kill -0 "$PID" 2>/dev/null; then
        echo "[dev-watch] процесс PID=$PID умер, рестарт"
        sleep 2
        start_app
    fi
done
