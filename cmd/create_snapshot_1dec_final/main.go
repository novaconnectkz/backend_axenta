package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	// Загружаем конфиг
	config.LoadConfig()

	// Инициализируем БД
	if err := database.ConnectDatabase(); err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка подключения к БД: %v\n", err)
		os.Exit(1)
	}

	// Дата снимка: 1 декабря 2025 года
	snapshotDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	fmt.Printf("📸 Пересоздание снимков за %s с сохранением всех объектов в БД\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка получения компаний: %v\n", err)
		os.Exit(1)
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

	fmt.Printf("📸 Запуск создания новых снимков через scheduler...\n")
	fmt.Printf("💡 Объекты будут автоматически сохранены в БД при создании снимков\n\n")

	// Запускаем создание снимков (синхронно, чтобы дождаться завершения)
	// Используем внутренний метод createDailySnapshotsForDate напрямую
	// Но он приватный, поэтому используем RunManualSnapshotForDate

	// RunManualSnapshotForDate работает асинхронно, но мы можем подождать
	go scheduler.RunManualSnapshotForDate(snapshotDate)

	fmt.Printf("⏳ Ожидание создания снимков (это может занять несколько минут)...\n\n")

	// Ждем некоторое время для создания снимков
	maxWaitTime := 5 * time.Minute
	checkInterval := 10 * time.Second
	elapsed := time.Duration(0)

	for elapsed < maxWaitTime {
		time.Sleep(checkInterval)
		elapsed += checkInterval

		// Проверяем прогресс
		snapshotCount := 0
		for _, company := range companies {
			tenantDB := database.GetTenantDBByID(company.ID)
			if tenantDB == nil {
				continue
			}

			var count int64
			tenantDB.Model(&models.PartnerDailySnapshot{}).
				Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
				Count(&count)
			snapshotCount += int(count)
		}

		if snapshotCount > 0 {
			fmt.Printf("   ✅ Создано снимков: %d (ожидаем завершения...)\n", snapshotCount)
		}
	}

	// Финальная проверка
	fmt.Printf("\n" + strings.Repeat("=", 100) + "\n")
	fmt.Printf("🔍 ФИНАЛЬНАЯ ПРОВЕРКА:\n\n")

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
		snapshotEndOfDay := time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 23, 59, 59, 999999999, time.UTC)

		for _, snapshot := range snapshots {
			companyObjectsInSnapshots += snapshot.TotalObjectsCount

			var objectsCount int64
			tenantDB.Model(&models.AxentaObjectSnapshot{}).
				Where("account_external_id = ? AND last_synced_at <= ?", int64(snapshot.PartnerCompanyID), snapshotEndOfDay).
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

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n")
	fmt.Printf("   - Дата: %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("   - Всего снимков: %d\n", totalSnapshots)
	fmt.Printf("   - Всего объектов в снимках: %d\n", totalObjectsInSnapshots)
	fmt.Printf("   - Всего объектов в БД: %d\n", totalObjectsInDB)

	if totalObjectsInSnapshots > 0 {
		if totalObjectsInSnapshots > totalObjectsInDB {
			fmt.Printf("\n⚠️ Не все объекты сохранены в БД (%d из %d)\n",
				totalObjectsInDB, totalObjectsInSnapshots)
			fmt.Printf("💡 Процесс сохранения объектов может еще продолжаться.\n")
		} else {
			fmt.Printf("\n✅ Все объекты сохранены в БД!\n")
		}
	} else {
		fmt.Printf("\n⚠️ Снимки еще не созданы. Проверьте логи или попробуйте позже.\n")
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
