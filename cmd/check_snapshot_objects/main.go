package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"time"
)

func main() {
	// Загружаем конфиг
	config.LoadConfig()
	
	// Инициализируем БД
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		log.Fatalf("Ошибка получения компаний: %v", err)
	}

	fmt.Printf("📊 Проверка количества объектов в снимках за 01/12/2025\n")
	fmt.Printf("Найдено компаний: %d\n\n", len(companies))
	
	// Выводим названия компаний
	fmt.Printf("📋 Список компаний:\n")
	for i, company := range companies {
		fmt.Printf("   %d. %s (ID=%d, схема: %s)\n", i+1, company.Name, company.ID, company.DatabaseSchema)
	}
	fmt.Printf("\n")

	totalObjects := int64(0)
	totalActive := int64(0)
	snapshotDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	for _, company := range companies {
		// Получаем tenant DB
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			fmt.Printf("⚠️ Компания %s (ID=%d): не удалось получить DB\n", company.Name, company.ID)
			continue
		}

		// Подсчитываем объекты в AxentaObjectSnapshot (все, не только за дату снимка)
		var objectCount int64
		var activeCount int64
		var recentCount int64 // Объекты синхронизированные за последние 24 часа

		if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Count(&objectCount).Error; err != nil {
			fmt.Printf("⚠️ Компания %s (ID=%d): ошибка подсчета объектов: %v\n", company.Name, company.ID, err)
			continue
		}

		if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("is_active = ?", true).
			Count(&activeCount).Error; err != nil {
			fmt.Printf("⚠️ Компания %s (ID=%d): ошибка подсчета активных объектов: %v\n", company.Name, company.ID, err)
			continue
		}

		// Объекты синхронизированные за последние 24 часа
		recentTime := time.Now().Add(-24 * time.Hour)
		tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("last_synced_at >= ?", recentTime).
			Count(&recentCount)

		// Проверяем PartnerDailySnapshot за эту дату
		var snapshotCount int64
		var totalObjectsInSnapshots int64
		var totalActiveInSnapshots int64
		
		tenantDB.Model(&models.PartnerDailySnapshot{}).
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Count(&snapshotCount)
		
		// Суммируем объекты из снимков
		tenantDB.Model(&models.PartnerDailySnapshot{}).
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Select("COALESCE(SUM(total_objects_count), 0)").
			Scan(&totalObjectsInSnapshots)
		
		tenantDB.Model(&models.PartnerDailySnapshot{}).
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Select("COALESCE(SUM(active_objects_count), 0)").
			Scan(&totalActiveInSnapshots)

		// Выводим информацию для ВСЕХ компаний
		fmt.Printf("✅ Компания %s (ID=%d, схема: %s):\n", company.Name, company.ID, company.DatabaseSchema)
		fmt.Printf("   - Объектов в AxentaObjectSnapshot: %d (активных: %d)\n", objectCount, activeCount)
		fmt.Printf("   - Синхронизировано за последние 24ч: %d\n", recentCount)
		fmt.Printf("   - Снимков PartnerDailySnapshot за %s: %d\n", snapshotDate.Format("2006-01-02"), snapshotCount)
		if snapshotCount > 0 {
			fmt.Printf("   - Объектов в снимках: %d (активных: %d)\n", totalObjectsInSnapshots, totalActiveInSnapshots)
		}
		fmt.Printf("\n")
		
		totalObjects += objectCount
		totalActive += activeCount
	}

	// Подсчитываем общее количество объектов из снимков
	var totalObjectsInAllSnapshots int64
	var totalActiveInAllSnapshots int64
	
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}
		
		var companyObjects, companyActive int64
		tenantDB.Model(&models.PartnerDailySnapshot{}).
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Select("COALESCE(SUM(total_objects_count), 0)").
			Scan(&companyObjects)
		
		tenantDB.Model(&models.PartnerDailySnapshot{}).
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Select("COALESCE(SUM(active_objects_count), 0)").
			Scan(&companyActive)
		
		totalObjectsInAllSnapshots += companyObjects
		totalActiveInAllSnapshots += companyActive
	}

	fmt.Printf("\n📊 ИТОГО:\n")
	fmt.Printf("   - Всего объектов в AxentaObjectSnapshot: %d\n", totalObjects)
	fmt.Printf("   - Активных объектов в AxentaObjectSnapshot: %d\n", totalActive)
	fmt.Printf("   - Всего объектов в PartnerDailySnapshot: %d\n", totalObjectsInAllSnapshots)
	fmt.Printf("   - Активных объектов в PartnerDailySnapshot: %d\n", totalActiveInAllSnapshots)

	// Проверяем последнюю задачу снимка
	var lastJob models.SnapshotJob
	if err := database.DB.Order("started_at DESC").First(&lastJob).Error; err == nil {
		fmt.Printf("\n📋 Последняя задача снимка:\n")
		fmt.Printf("   - ID: %d\n", lastJob.ID)
		fmt.Printf("   - Статус: %s\n", lastJob.Status)
		fmt.Printf("   - Дата: %s - %s\n", lastJob.DateFrom.Format("2006-01-02"), lastJob.DateTo.Format("2006-01-02"))
		fmt.Printf("   - Объектов: %d (активных: %d)\n", lastJob.TotalObjects, lastJob.ActiveObjects)
		fmt.Printf("   - Компаний: %d, Договоров: %d\n", lastJob.TotalCompanies, lastJob.TotalContracts)
		fmt.Printf("   - Успешно: %d, Ошибок: %d\n", lastJob.SuccessCount, lastJob.ErrorCount)
	}
}

