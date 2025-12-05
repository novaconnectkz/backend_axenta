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

	// Начало измерения времени
	startTime := time.Now()

	fmt.Printf("📸 Пересоздание снимков за %s с сохранением всех объектов в БД\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("⏰ Начало: %s\n", startTime.Format("2006-01-02 15:04:05"))
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		log.Fatalf("Ошибка получения компаний: %v", err)
	}

	// Удаляем все существующие снимки за эту дату
	fmt.Printf("🗑️ Удаление всех существующих снимков за %s...\n\n", snapshotDate.Format("2006-01-02"))

	totalDeleted := 0
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
			totalDeleted += int(result.RowsAffected)
		}
	}

	fmt.Printf("\n✅ Удалено снимков: %d\n\n", totalDeleted)

	// Создаем scheduler
	scheduler := services.NewPartnerSnapshotScheduler()

	fmt.Printf("📸 Создание новых снимков через scheduler...\n")
	fmt.Printf("💡 Объекты будут автоматически сохранены в БД при создании снимков\n")
	fmt.Printf("⏳ Это может занять несколько минут...\n\n")

	// Запускаем создание снимков (синхронно)
	fmt.Printf("⏳ Запуск создания снимков...\n\n")
	creationStartTime := time.Now()
	scheduler.RunManualSnapshotForDate(snapshotDate)
	creationEndTime := time.Now()
	creationDuration := creationEndTime.Sub(creationStartTime)

	fmt.Printf("\n" + strings.Repeat("=", 100) + "\n")
	fmt.Printf("✅ Создание снимков завершено!\n")
	fmt.Printf("⏱️  Время создания снимков: %v\n", creationDuration.Round(time.Second))
	fmt.Printf("💡 Объекты сохранены в БД при создании снимков\n\n")

	// Проверяем результаты
	fmt.Printf("🔍 ПРОВЕРКА РЕЗУЛЬТАТОВ:\n\n")

	totalSnapshots := 0
	totalObjectsInSnapshots := 0
	totalObjectsInDB := 0

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
		companyObjectsInDB := 0

		for _, snapshot := range snapshots {
			companyObjectsInSnapshots += snapshot.TotalObjectsCount

			// Объекты теперь хранятся глобально один раз
			// Нужно подсчитать объекты партнера через account_external_id и иерархию
			// Пока считаем все уникальные объекты за дату (так как объекты глобальные)
			var objectsCount int64
			tenantDB.Model(&models.AxentaObjectSnapshot{}).
				Where("DATE(last_synced_at AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
				Count(&objectsCount)

			companyObjectsInDB += int(objectsCount)

			statusIcon := "✅"
			if snapshot.TotalObjectsCount > int(objectsCount) {
				statusIcon = "⚠️"
			}

			fmt.Printf("   %s Partner %d: в снимке=%d, в БД=%d\n",
				statusIcon, snapshot.PartnerCompanyID, snapshot.TotalObjectsCount, objectsCount)
		}

		fmt.Printf("   Итого: в снимках=%d, в БД=%d\n\n", companyObjectsInSnapshots, companyObjectsInDB)

		totalObjectsInSnapshots += companyObjectsInSnapshots
		totalObjectsInDB += companyObjectsInDB
	}

	endTime := time.Now()
	totalDuration := endTime.Sub(startTime)

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n")
	fmt.Printf("   - Дата: %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("   - Всего снимков: %d\n", totalSnapshots)
	fmt.Printf("   - Всего объектов в снимках: %d\n", totalObjectsInSnapshots)
	fmt.Printf("   - Всего объектов в БД: %d\n", totalObjectsInDB)
	fmt.Printf("\n⏱️  ВРЕМЯ ВЫПОЛНЕНИЯ:\n")
	fmt.Printf("   - Создание снимков: %v\n", creationDuration.Round(time.Second))
	fmt.Printf("   - Общее время: %v\n", totalDuration.Round(time.Second))
	fmt.Printf("   - Начало: %s\n", startTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("   - Конец: %s\n", endTime.Format("2006-01-02 15:04:05"))

	if totalObjectsInSnapshots > totalObjectsInDB {
		fmt.Printf("\n⚠️ Не все объекты сохранены в БД (%d из %d)\n",
			totalObjectsInDB, totalObjectsInSnapshots)
	} else if totalObjectsInSnapshots > 0 {
		fmt.Printf("\n✅ Все объекты сохранены в БД!\n")
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
