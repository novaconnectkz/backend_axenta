package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	// Отключаем лишние логи
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

	fmt.Printf("🔍 Проверка объектов в БД за %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка получения компаний: %v\n", err)
		os.Exit(1)
	}

	totalObjectsInSnapshots := 0
	totalUniqueObjectsInDB := 0

	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		// Подсчитываем объекты в снимках
		var snapshots []models.PartnerDailySnapshot
		if err := tenantDB.
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Find(&snapshots).Error; err != nil {
			continue
		}

		if len(snapshots) == 0 {
			continue
		}

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

		fmt.Printf("🏢 Компания: %s\n", company.Name)
		fmt.Printf("   Снимков: %d\n", len(snapshots))
		fmt.Printf("   Объектов в снимках: %d\n", companyObjectsInSnapshots)
		fmt.Printf("   Уникальных объектов в БД: %d\n", companyObjectsInDB)

		if companyObjectsInSnapshots > companyObjectsInDB {
			percentage := float64(companyObjectsInDB) / float64(companyObjectsInSnapshots) * 100
			fmt.Printf("   ⚠️ Не все объекты сохранены (%.1f%%, %d из %d)\n\n", percentage, companyObjectsInDB, companyObjectsInSnapshots)
		} else {
			fmt.Printf("   ✅ Все объекты сохранены!\n\n")
		}
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГО:\n")
	fmt.Printf("   - Всего объектов в снимках: %d\n", totalObjectsInSnapshots)
	fmt.Printf("   - Уникальных объектов в БД: %d\n", totalUniqueObjectsInDB)

	if totalObjectsInSnapshots > totalUniqueObjectsInDB {
		percentage := float64(totalUniqueObjectsInDB) / float64(totalObjectsInSnapshots) * 100
		missing := totalObjectsInSnapshots - totalUniqueObjectsInDB
		fmt.Printf("   - ⚠️ Не сохранено: %d объектов (%.1f%%)\n", missing, 100-percentage)
		fmt.Printf("\n💡 Возможные причины:\n")
		fmt.Printf("   1. Объекты еще не были загружены через интерфейс\n")
		fmt.Printf("   2. Ошибка при сохранении объектов\n")
		fmt.Printf("   3. Объекты сохраняются в другой схеме\n")
	} else {
		fmt.Printf("   ✅ Все объекты сохранены в БД!\n")
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
}

type NullWriter struct{}

func (w *NullWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
