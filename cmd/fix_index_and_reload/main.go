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
)

func main() {
	// Загружаем конфиг
	config.LoadConfig()

	// Инициализируем БД
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}

	// Дата снимка: 1 декабря 2025 года
	snapshotDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	startTime := time.Now()
	fmt.Printf("🔧 Исправление индекса и перезагрузка данных за %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("⏰ Начало: %s\n", startTime.Format("2006-01-02 15:04:05"))
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		log.Fatalf("Ошибка получения компаний: %v", err)
	}

	// ШАГ 0: Удаляем все объекты за дату (чтобы можно было создать индекс)
	fmt.Printf("🗑️  ШАГ 0: Удаление всех объектов за %s для исправления индекса...\n\n", snapshotDate.Format("2006-01-02"))

	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		result := tenantDB.Unscoped().
			Where("DATE(last_synced_at AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Delete(&models.AxentaObjectSnapshot{})

		if result.Error == nil && result.RowsAffected > 0 {
			fmt.Printf("   🗑️ Компания %s: удалено объектов: %d\n", company.Name, result.RowsAffected)
		}
	}

	// ШАГ 1: Удаляем все снимки за эту дату
	fmt.Printf("\n🗑️  ШАГ 1: Удаление всех снимков за %s...\n\n", snapshotDate.Format("2006-01-02"))

	totalSnapshotsDeleted := 0
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		result := tenantDB.Unscoped().
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Delete(&models.PartnerDailySnapshot{})

		if result.Error == nil && result.RowsAffected > 0 {
			fmt.Printf("   🗑️ Компания %s: удалено снимков: %d\n", company.Name, result.RowsAffected)
			totalSnapshotsDeleted += int(result.RowsAffected)
		}
	}

	fmt.Printf("\n✅ Удалено снимков: %d\n\n", totalSnapshotsDeleted)

	// ШАГ 2: Создаем новые снимки с сохранением объектов
	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📸 ШАГ 2: Создание новых снимков с сохранением всех объектов...\n")
	fmt.Printf("💡 Объекты будут автоматически сохранены в БД при создании снимков (глобально, без дублирования)\n")
	fmt.Printf("⏳ Это может занять несколько минут...\n\n")

	creationStartTime := time.Now()

	// Создаем scheduler
	scheduler := services.NewPartnerSnapshotScheduler()

	// Запускаем создание снимков (синхронно)
	scheduler.RunManualSnapshotForDate(snapshotDate)

	creationEndTime := time.Now()
	creationDuration := creationEndTime.Sub(creationStartTime)

	fmt.Printf("\n✅ Создание снимков завершено за %v\n\n", creationDuration.Round(time.Second))

	// ШАГ 3: Проверка результатов
	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("🔍 ШАГ 3: Проверка результатов...\n\n")

	totalSnapshots := 0
	totalObjectsInSnapshots := 0
	totalUniqueObjectsInDB := 0

	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		var snapshots []models.PartnerDailySnapshot
		if err := tenantDB.
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Order("partner_company_id ASC").
			Find(&snapshots).Error; err != nil {
			continue
		}

		if len(snapshots) == 0 {
			continue
		}

		totalSnapshots += len(snapshots)
		fmt.Printf("🏢 Компания: %s\n", company.Name)
		fmt.Printf("   Найдено снимков: %d\n", len(snapshots))

		companyObjectsInSnapshots := 0
		for _, snapshot := range snapshots {
			companyObjectsInSnapshots += snapshot.TotalObjectsCount
		}

		// Подсчитываем уникальные объекты в БД за эту дату (глобально)
		var uniqueObjectsCount int64
		tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("DATE(last_synced_at AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Count(&uniqueObjectsCount)

		companyObjectsInDB := int(uniqueObjectsCount)
		totalObjectsInSnapshots += companyObjectsInSnapshots
		totalUniqueObjectsInDB += companyObjectsInDB

		fmt.Printf("   Объектов в снимках: %d\n", companyObjectsInSnapshots)
		fmt.Printf("   Уникальных объектов в БД: %d\n", companyObjectsInDB)

		if companyObjectsInSnapshots > companyObjectsInDB {
			fmt.Printf("   ⚠️ Не все объекты сохранены (%d из %d)\n\n", companyObjectsInDB, companyObjectsInSnapshots)
		} else {
			fmt.Printf("   ✅ Все объекты сохранены в БД!\n\n")
		}
	}

	endTime := time.Now()
	totalDuration := endTime.Sub(startTime)

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n")
	fmt.Printf("   - Дата: %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("   - Удалено снимков: %d\n", totalSnapshotsDeleted)
	fmt.Printf("   - Создано снимков: %d\n", totalSnapshots)
	fmt.Printf("   - Всего объектов в снимках: %d\n", totalObjectsInSnapshots)
	fmt.Printf("   - Уникальных объектов в БД: %d\n", totalUniqueObjectsInDB)
	fmt.Printf("\n⏱️  ВРЕМЯ ВЫПОЛНЕНИЯ:\n")
	fmt.Printf("   - Создание снимков: %v\n", creationDuration.Round(time.Second))
	fmt.Printf("   - Общее время: %v\n", totalDuration.Round(time.Second))
	fmt.Printf("   - Начало: %s\n", startTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("   - Конец: %s\n", endTime.Format("2006-01-02 15:04:05"))

	if totalObjectsInSnapshots > 0 {
		if totalObjectsInSnapshots > totalUniqueObjectsInDB {
			percentage := float64(totalUniqueObjectsInDB) / float64(totalObjectsInSnapshots) * 100
			fmt.Printf("\n⚠️ Не все объекты сохранены в БД (%d из %d, %.1f%%)\n",
				totalUniqueObjectsInDB, totalObjectsInSnapshots, percentage)
		} else if totalUniqueObjectsInDB <= totalObjectsInSnapshots {
			fmt.Printf("\n✅ Все объекты сохранены в БД (глобально, без дублирования)!\n")
			fmt.Printf("💡 Объекты хранятся один раз, привязка к партнерам через account_external_id и иерархию\n")
		}
	} else {
		fmt.Printf("\n⚠️ Снимки не созданы. Проверьте логи.\n")
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
}

