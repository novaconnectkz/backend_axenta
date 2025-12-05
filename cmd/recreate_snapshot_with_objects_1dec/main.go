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

	// Создаем scheduler для распределения объектов
	scheduler := services.NewPartnerSnapshotScheduler()

	// Запускаем пересоздание снимков через scheduler
	// Сначала удалим все старые снимки
	fmt.Printf("🗑️ Удаление всех существующих снимков за %s...\n\n", snapshotDate.Format("2006-01-02"))

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
		}
	}

	fmt.Printf("\n✅ Все старые снимки удалены\n\n")

	// Теперь создаем новые снимки через scheduler
	fmt.Printf("📸 Создание новых снимков через scheduler...\n\n")

	// Запускаем создание снимков в фоне через scheduler
	go scheduler.RunManualSnapshotForDate(snapshotDate)

	fmt.Printf("⏳ Ожидание завершения создания снимков...\n")
	fmt.Printf("💡 Это может занять несколько минут, так как нужно загрузить все объекты через API\n\n")

	// Ждем завершения (можно увеличить время ожидания)
	time.Sleep(30 * time.Second)

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

			if snapshot.TotalObjectsCount > int(objectsCount) {
				fmt.Printf("   ⚠️ Partner %d: в снимке=%d, в БД=%d (не все сохранены)\n",
					snapshot.PartnerCompanyID, snapshot.TotalObjectsCount, objectsCount)
			} else {
				fmt.Printf("   ✅ Partner %d: в снимке=%d, в БД=%d\n",
					snapshot.PartnerCompanyID, snapshot.TotalObjectsCount, objectsCount)
			}
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
		fmt.Printf("\n⚠️ ВНИМАНИЕ: Не все объекты сохранены в БД (%d из %d)\n", 
			totalObjectsInDB, totalObjectsInSnapshots)
		fmt.Printf("💡 Scheduler использует CreateSnapshotWithObjectCounts, которая не сохраняет объекты.\n")
		fmt.Printf("   Нужно модифицировать scheduler или использовать другой метод создания снимков.\n")
	} else {
		fmt.Printf("\n✅ Все объекты сохранены в БД!\n")
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
