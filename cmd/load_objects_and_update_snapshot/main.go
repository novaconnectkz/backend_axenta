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
	log.Println("🔄 Загрузка объектов из Axenta и обновление снимка за 07/12/2025...")

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

	// Проверяем настройки снимков и получаем токен
	var snapshotSettings models.SnapshotSettings
	if err := tenantDB.Where("company_id = ? AND is_active = ?", 1, true).First(&snapshotSettings).Error; err != nil {
		log.Fatalf("❌ Токен Axenta не настроен: %v", err)
	}

	if snapshotSettings.AxentaToken == "" {
		log.Fatalf("❌ Токен Axenta не установлен")
	}

	fmt.Printf("✅ Токен найден (длина: %d символов)\n\n", len(snapshotSettings.AxentaToken))

	// Проверяем текущее состояние БД ДО загрузки
	fmt.Println("📊 Состояние БД ДО загрузки:")
	var countBefore int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).Count(&countBefore)
	fmt.Printf("   - Объектов в axenta_object_snapshots: %d\n", countBefore)

	var countWithDateBefore int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("axenta_created_at IS NOT NULL").
		Count(&countWithDateBefore)
	fmt.Printf("   - Объектов с axenta_created_at: %d\n\n", countWithDateBefore)

	// Загружаем все объекты из Axenta
	fmt.Println("🔄 Загружаем все объекты из Axenta API...")
	accumulationService := services.NewSnapshotAccumulationService()
	startTime := time.Now()
	if err := accumulationService.LoadAllCurrentObjects(snapshotSettings.AxentaToken); err != nil {
		log.Printf("❌ Ошибка загрузки объектов: %v", err)
		log.Println("   Продолжаем с текущими данными...")
	} else {
		duration := time.Since(startTime)
		fmt.Printf("✅ Загрузка завершена за %v\n\n", duration)
	}

	// Ждем немного, чтобы убедиться, что данные записались
	time.Sleep(2 * time.Second)

	// Проверяем состояние БД ПОСЛЕ загрузки
	fmt.Println("📊 Состояние БД ПОСЛЕ загрузки:")
	var countAfter int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).Count(&countAfter)
	fmt.Printf("   - Объектов в axenta_object_snapshots: %d (было: %d, добавлено: %d)\n", countAfter, countBefore, countAfter-countBefore)

	var countWithDateAfter int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("axenta_created_at IS NOT NULL").
		Count(&countWithDateAfter)
	fmt.Printf("   - Объектов с axenta_created_at: %d (было: %d, добавлено: %d)\n\n", countWithDateAfter, countWithDateBefore, countWithDateAfter-countWithDateBefore)

	// Дата снимка: 07/12/2025
	snapshotDate, err := time.Parse("2006-01-02", "2025-12-07")
	if err != nil {
		log.Fatalf("❌ Ошибка парсинга даты: %v", err)
	}
	snapshotDate = time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 0, 0, 0, 0, time.UTC)

	// Пересчитываем объекты на дату 07/12/2025
	fmt.Printf("📊 Пересчет объектов на дату %s...\n", snapshotDate.Format("2006-01-02"))
	totalObjects, activeObjects, err := accumulationService.CalculateObjectsCountForDate(snapshotDate, tenantDB)
	if err != nil {
		log.Fatalf("❌ Ошибка подсчета объектов: %v", err)
	}

	fmt.Printf("✅ Найдено объектов: всего=%d, активных=%d\n\n", totalObjects, activeObjects)

	// Ищем самый ранний объект для этой даты
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

	// Обновляем записи в snapshot_jobs
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Fatalf("❌ Ошибка переключения на схему public: %v", err)
	}

	dateStr := snapshotDate.Format("2006-01-02")
	fmt.Println("💾 Обновление записей в snapshot_jobs...")

	// Обновляем запись типа billing_start
	var billingStartJob models.SnapshotJob
	if err := publicDB.
		Where("job_type = ? AND date_from::date = ?::date", models.SnapshotJobTypeBillingStart, dateStr).
		First(&billingStartJob).Error; err == nil {
		fmt.Printf("   📝 Обновляем запись billing_start (ID: %d)...\n", billingStartJob.ID)
		billingStartJob.TotalObjects = totalObjects
		billingStartJob.ActiveObjects = activeObjects
		billingStartJob.FinishJob(models.SnapshotJobStatusCompleted, "")
		if err := publicDB.Save(&billingStartJob).Error; err != nil {
			log.Printf("   ❌ Ошибка обновления записи billing_start: %v", err)
		} else {
			fmt.Printf("   ✅ Запись billing_start обновлена (ID: %d)\n", billingStartJob.ID)
		}
	} else {
		fmt.Printf("   ⚠️ Запись billing_start не найдена, создаем новую...\n")
		if err := accumulationService.SaveBillingStartDateToHistory(tenantDB, snapshotDate, totalObjects, activeObjects, earliestObject); err != nil {
			log.Printf("   ❌ Ошибка создания записи billing_start: %v", err)
		} else {
			fmt.Printf("   ✅ Запись billing_start создана\n")
		}
	}

	// Обновляем запись типа daily_auto
	var dailyAutoJob models.SnapshotJob
	if err := publicDB.
		Where("job_type = ? AND date_from::date = ?::date", models.SnapshotJobTypeDailyAuto, dateStr).
		First(&dailyAutoJob).Error; err == nil {
		fmt.Printf("   📝 Обновляем запись daily_auto (ID: %d)...\n", dailyAutoJob.ID)
		dailyAutoJob.TotalObjects = totalObjects
		dailyAutoJob.ActiveObjects = activeObjects
		dailyAutoJob.FinishJob(models.SnapshotJobStatusCompleted, "")
		if err := publicDB.Save(&dailyAutoJob).Error; err != nil {
			log.Printf("   ❌ Ошибка обновления записи daily_auto: %v", err)
		} else {
			fmt.Printf("   ✅ Запись daily_auto обновлена (ID: %d)\n", dailyAutoJob.ID)
		}
	} else {
		fmt.Printf("   ⚠️ Запись daily_auto не найдена, создаем новую...\n")
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
			log.Printf("   ❌ Ошибка создания записи daily_auto: %v", err)
		} else {
			fmt.Printf("   ✅ Запись daily_auto создана (ID: %d)\n", job.ID)
		}
	}

	// Выводим итоговую информацию
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ Объекты загружены и снимок обновлен!")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("📅 Дата снимка: %s\n", snapshotDate.Format("2006-01-02"))
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
