package main

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"os"

	"gorm.io/gorm"
)

func main() {
	// Подключаемся к БД
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}

	log.Println("🔍 Проверка записи о стартовой дате биллинга в таблице snapshot_jobs...")

	// Переключаемся на схему public
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Fatalf("Ошибка переключения на схему public: %v", err)
	}

	// Проверяем существование таблицы
	if !publicDB.Migrator().HasTable(&models.SnapshotJob{}) {
		log.Fatalf("❌ Таблица snapshot_jobs не существует в схеме public!")
	}
	log.Println("✅ Таблица snapshot_jobs существует")

	// Ищем все записи типа billing_start
	var billingStartJobs []models.SnapshotJob
	if err := publicDB.
		Where("job_type = ?", models.SnapshotJobTypeBillingStart).
		Order("started_at DESC").
		Find(&billingStartJobs).Error; err != nil {
		log.Fatalf("❌ Ошибка поиска записей: %v", err)
	}

	if len(billingStartJobs) == 0 {
		log.Println("⚠️ Записи типа billing_start не найдены!")
		log.Println("   Вызовите GET /api/auth/snapshots/billing-start-date для создания записи")
		os.Exit(1)
	}

	log.Printf("✅ Найдено записей billing_start: %d\n", len(billingStartJobs))

	// Выводим информацию о каждой записи
	for i, job := range billingStartJobs {
		fmt.Printf("\n--- Запись %d ---\n", i+1)
		fmt.Printf("ID: %d\n", job.ID)
		fmt.Printf("Тип: %s\n", job.JobType)
		fmt.Printf("Дата начала: %s\n", job.DateFrom.Format("2006-01-02"))
		fmt.Printf("Дата окончания: %s\n", job.DateTo.Format("2006-01-02"))
		fmt.Printf("Статус: %s\n", job.Status)
		fmt.Printf("Всего объектов: %d\n", job.TotalObjects)
		fmt.Printf("Активных объектов: %d\n", job.ActiveObjects)
		fmt.Printf("Создано: %s\n", job.StartedAt.Format("2006-01-02 15:04:05"))
		if job.FinishedAt != nil {
			fmt.Printf("Завершено: %s\n", job.FinishedAt.Format("2006-01-02 15:04:05"))
		}
		if job.DurationSeconds != nil {
			fmt.Printf("Длительность: %d сек\n", *job.DurationSeconds)
		}
	}

	// Проверяем, есть ли запись от 01/01/2025
	date2025 := "2025-01-01"
	var job2025 models.SnapshotJob
	if err := publicDB.
		Where("job_type = ? AND date_from::date = ?::date", models.SnapshotJobTypeBillingStart, date2025).
		First(&job2025).Error; err == nil {
		log.Printf("\n✅ Запись от %s найдена! ID: %d", date2025, job2025.ID)
	} else {
		log.Printf("\n⚠️ Запись от %s не найдена", date2025)
		log.Printf("   Найдены записи со следующими датами:")
		for _, job := range billingStartJobs {
			log.Printf("   - %s (ID: %d)", job.DateFrom.Format("2006-01-02"), job.ID)
		}
	}

	// Проверяем все записи в таблице (для сравнения)
	var allJobs []models.SnapshotJob
	if err := publicDB.
		Order("started_at DESC").
		Limit(10).
		Find(&allJobs).Error; err == nil {
		log.Printf("\n📊 Последние 10 записей в таблице snapshot_jobs:")
		for _, job := range allJobs {
			log.Printf("   ID: %d, Тип: %s, Дата: %s, Статус: %s",
				job.ID, job.JobType, job.DateFrom.Format("2006-01-02"), job.Status)
		}
	}
}
