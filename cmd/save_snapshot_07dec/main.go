package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

func main() {
	log.Println("📸 Сохранение снимка за 07/12/2025...")

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

	// Дата снимка: 07/12/2025
	snapshotDate, err := time.Parse("2006-01-02", "2025-12-07")
	if err != nil {
		log.Fatalf("❌ Ошибка парсинга даты: %v", err)
	}
	// Устанавливаем время на начало дня в UTC
	snapshotDate = time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 0, 0, 0, 0, time.UTC)

	fmt.Printf("📅 Дата снимка: %s\n\n", snapshotDate.Format("2006-01-02"))

	// Получаем первую активную компанию
	var firstCompany models.Company
	if err := database.DB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		log.Fatalf("❌ Не найдено активных компаний: %v", err)
	}

	fmt.Printf("🏢 Компания: %s (ID=%d, схема: %s)\n\n", firstCompany.Name, firstCompany.ID, firstCompany.DatabaseSchema)

	// Получаем tenant DB
	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		log.Fatalf("❌ Не удалось получить tenant DB для компании %d", firstCompany.ID)
	}

	// Создаем сервис накопления
	accumulationService := services.NewSnapshotAccumulationService()

	// Подсчитываем объекты на дату 07/12/2025
	fmt.Println("📊 Подсчет объектов на дату 07/12/2025...")
	totalObjects, activeObjects, err := accumulationService.CalculateObjectsCountForDate(snapshotDate, tenantDB)
	if err != nil {
		log.Fatalf("❌ Ошибка подсчета объектов: %v", err)
	}

	fmt.Printf("✅ Найдено объектов: всего=%d, активных=%d\n\n", totalObjects, activeObjects)

	// Ищем самый ранний объект для этой даты (если есть)
	var earliestObject *models.AxentaObjectSnapshot
	var earliestObj models.AxentaObjectSnapshot
	dayEnd := time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 23, 59, 59, 999999999, time.UTC)
	query := tenantDB.
		Where("axenta_created_at IS NOT NULL").
		Where("(axenta_created_at IS NULL OR axenta_created_at <= ?) AND (axenta_deleted_at IS NULL OR axenta_deleted_at > ?)",
			dayEnd, dayEnd).
		Order("axenta_created_at ASC").
		Limit(1)

	if err := query.First(&earliestObj).Error; err == nil {
		earliestObject = &earliestObj
		fmt.Printf("📦 Самый ранний объект на эту дату: ID=%d, дата создания=%s\n\n",
			earliestObject.ExternalObjectID,
			earliestObject.AxentaCreatedAt.Format("2006-01-02 15:04:05"))
	}

	// Создаем запись в истории снимков (snapshot_jobs)
	fmt.Println("💾 Сохранение снимка в историю...")
	if err := accumulationService.SaveBillingStartDateToHistory(tenantDB, snapshotDate, totalObjects, activeObjects, earliestObject); err != nil {
		log.Printf("⚠️ Ошибка сохранения в историю: %v", err)
		log.Println("   Продолжаем без записи в историю...")
	} else {
		fmt.Println("✅ Снимок сохранен в историю (snapshot_jobs)")
	}

	// Также создаем запись в таблице snapshot_jobs с типом daily_auto для этой даты
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Ошибка переключения на схему public: %v", err)
	} else {
		fmt.Println("\n💾 Создание записи в snapshot_jobs (тип: daily_auto)...")

		// Проверяем, не существует ли уже запись
		var existingJob models.SnapshotJob
		dateStr := snapshotDate.Format("2006-01-02")
		checkQuery := publicDB.
			Where("job_type = ?", models.SnapshotJobTypeDailyAuto).
			Where("date_from::date = ?::date", dateStr)

		if err := checkQuery.First(&existingJob).Error; err == nil {
			fmt.Printf("⚠️ Запись для даты %s уже существует (ID: %d), обновляем...\n", dateStr, existingJob.ID)
			existingJob.TotalObjects = totalObjects
			existingJob.ActiveObjects = activeObjects
			existingJob.FinishJob(models.SnapshotJobStatusCompleted, "")
			if err := publicDB.Save(&existingJob).Error; err != nil {
				log.Printf("❌ Ошибка обновления записи: %v", err)
			} else {
				fmt.Printf("✅ Запись обновлена (ID: %d)\n", existingJob.ID)
			}
		} else {
			// Создаем новую запись
			now := time.Now()
			job := models.SnapshotJob{
				JobType:            models.SnapshotJobTypeDailyAuto,
				StartedAt:          now,
				DateFrom:           snapshotDate,
				DateTo:             snapshotDate,
				Status:             models.SnapshotJobStatusCompleted,
				TotalObjects:       totalObjects,
				ActiveObjects:      activeObjects,
				SuccessCount:       1,
				TotalDaysProcessed: 1,
				TriggeredBy:        "manual",
			}
			job.FinishJob(models.SnapshotJobStatusCompleted, "")

			if err := publicDB.Create(&job).Error; err != nil {
				log.Printf("❌ Ошибка создания записи: %v", err)
			} else {
				fmt.Printf("✅ Запись создана (ID: %d)\n", job.ID)
			}
		}
	}

	// Выводим итоговую информацию
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ Снимок за 07/12/2025 успешно сохранен!")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("📅 Дата: %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("📊 Объектов: всего=%d, активных=%d\n", totalObjects, activeObjects)
	if earliestObject != nil {
		fmt.Printf("📦 Самый ранний объект: ID=%d, дата создания=%s\n",
			earliestObject.ExternalObjectID,
			earliestObject.AxentaCreatedAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("💾 Таблица: snapshot_jobs (схема: public)\n")
	fmt.Printf("🏢 Компания: %s (схема: %s)\n", firstCompany.Name, firstCompany.DatabaseSchema)
	fmt.Println(strings.Repeat("=", 60))
}
