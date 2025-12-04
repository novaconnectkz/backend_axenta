package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/services"
	"log"
	"time"
)

func main() {
	log.Println("🚀 Запуск снимка за 01/12/2025...")

	// Загружаем конфигурацию
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Не удалось загрузить конфигурацию: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Не удалось подключиться к базе данных: %v", err)
	}
	defer func() {
		sqlDB, _ := database.DB.DB()
		sqlDB.Close()
	}()

	// Парсим дату
	snapshotDate, err := time.Parse("2006-01-02", "2025-12-01")
	if err != nil {
		log.Fatalf("❌ Неверный формат даты: %v", err)
	}

	log.Printf("📅 Дата снимка: %s", snapshotDate.Format("2006-01-02"))

	// Создаем scheduler
	scheduler := services.NewPartnerSnapshotScheduler()

	// Запускаем снимок в фоне
	log.Println("⏳ Запускаем создание снимков...")
	go scheduler.RunManualSnapshotForDate(snapshotDate)

	log.Println("✅ Запрос на создание снимков отправлен. Проверьте историю через несколько минут.")
	log.Println("💡 Для проверки статуса используйте: go run cmd/check_jobs_stats/main.go")
	
	// Ждем немного, чтобы задача успела начаться
	time.Sleep(2 * time.Second)
}

