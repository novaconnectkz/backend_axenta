package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// NullWriter для подавления логов
type NullWriter struct{}

func (w *NullWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func main() {
	// Отключаем лишние логи для чистого вывода
	log.SetOutput(&NullWriter{})

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

		// Удаляем все снимки за эту дату
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

	fmt.Printf("📸 Создание новых снимков через scheduler (с сохранением объектов)...\n")
	fmt.Printf("⏳ Это может занять несколько минут...\n\n")

	// Запускаем создание снимков
	scheduler.RunManualSnapshotForDate(snapshotDate)

	fmt.Printf("✅ Создание снимков запущено. Объекты будут сохранены автоматически.\n\n")

	// Ждем немного для завершения
	time.Sleep(10 * time.Second)

	// Проверяем результаты
	fmt.Printf(strings.Repeat("=", 100) + "\n")
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

	if totalObjectsInSnapshots > totalObjectsInDB {
		fmt.Printf("\n⚠️ Не все объекты сохранены в БД (%d из %d)\n",
			totalObjectsInDB, totalObjectsInSnapshots)
		fmt.Printf("💡 Процесс создания снимков может еще продолжаться.\n")
		fmt.Printf("   Проверьте через несколько минут или посмотрите логи.\n")
	} else {
		fmt.Printf("\n✅ Все объекты сохранены в БД!\n")
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
