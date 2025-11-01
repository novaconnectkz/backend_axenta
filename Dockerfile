# Многоэтапная сборка для Go приложения
# Этап 1: сборка
FROM golang:1.23-alpine AS builder

# Устанавливаем необходимые пакеты для сборки
RUN apk add --no-cache git make

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем go mod файлы
COPY go.mod go.sum ./

# Загружаем зависимости
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем бинарник
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o axenta_server ./main.go

# Этап 2: финальный образ
FROM alpine:latest

# Устанавливаем ca-certificates для HTTPS запросов
RUN apk --no-cache add ca-certificates tzdata

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем бинарник из builder этапа
COPY --from=builder /app/axenta_server .

# Копируем env.example как шаблон (опционально)
COPY --from=builder /app/env.example .

# Открываем порт
EXPOSE 8080

# Устанавливаем переменные окружения по умолчанию
ENV GIN_MODE=release
ENV SERVER_PORT=8080

# Команда запуска
CMD ["./axenta_server"]

