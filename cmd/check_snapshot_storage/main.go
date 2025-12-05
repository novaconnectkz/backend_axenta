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

	snapshotDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	fmt.Printf("🔍 Проверка: что именно хранится в БД для снимков за %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка получения компаний: %v\n", err)
		os.Exit(1)
	}

	totalSnapshotsInDB := 0
	totalObjectsCountInSnapshots := 0
	totalObjectsInAxentaTable := 0

	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		// 1. Проверяем снимки в таблице partner_daily_snapshots
		var snapshots []models.PartnerDailySnapshot
		if err := tenantDB.
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Find(&snapshots).Error; err != nil {
			continue
		}

		if len(snapshots) == 0 {
			continue
		}

		totalSnapshotsInDB += len(snapshots)
		companyObjectsInSnapshots := 0
		for _, s := range snapshots {
			companyObjectsInSnapshots += s.TotalObjectsCount
		}
		totalObjectsCountInSnapshots += companyObjectsInSnapshots

		// 2. Проверяем объекты в таблице axenta_object_snapshots
		var objectCount int64
		snapshotEndOfDay := time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 23, 59, 59, 999999999, time.UTC)
		tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("last_synced_at <= ?", snapshotEndOfDay).
			Count(&objectCount)

		totalObjectsInAxentaTable += int(objectCount)

		fmt.Printf("🏢 Компания: %s (схема: %s)\n", company.Name, company.DatabaseSchema)
		fmt.Printf("   📊 В таблице partner_daily_snapshots:\n")
		fmt.Printf("      - Найдено снимков: %d\n", len(snapshots))
		fmt.Printf("      - Всего объектов (сумма из снимков): %d\n", companyObjectsInSnapshots)
		fmt.Printf("      - Это МЕТАДАННЫЕ (количество, стоимость, дата и т.д.)\n")
		fmt.Printf("   📦 В таблице axenta_object_snapshots:\n")
		fmt.Printf("      - Найдено записей объектов: %d\n", objectCount)
		fmt.Printf("      - Это САМИ ОБЪЕКТЫ (названия, ID, параметры и т.д.)\n")
		fmt.Printf("\n")
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n\n")
	fmt.Printf("1️⃣ В таблице partner_daily_snapshots хранится:\n")
	fmt.Printf("   ✅ %d снимков (записей)\n", totalSnapshotsInDB)
	fmt.Printf("   ✅ Сводная информация: всего %d объектов (сумма из всех снимков)\n", totalObjectsCountInSnapshots)
	fmt.Printf("   ✅ Это МЕТАДАННЫЕ: количество объектов, стоимость, дата, партнер ID и т.д.\n")
	fmt.Printf("   ❌ НЕ хранятся: списки объектов, названия объектов, ID объектов\n\n")

	fmt.Printf("2️⃣ В таблице axenta_object_snapshots хранится:\n")
	fmt.Printf("   ✅ %d записей объектов (на дату снимка)\n", totalObjectsInAxentaTable)
	fmt.Printf("   ✅ Это САМИ ОБЪЕКТЫ: названия, External ID, Unique ID, параметры и т.д.\n\n")

	fmt.Printf("3️⃣ ВЫВОД:\n")
	if totalObjectsCountInSnapshots > totalObjectsInAxentaTable {
		fmt.Printf("   ⚠️ В снимках учтено %d объектов, но в БД хранится только %d записей объектов\n",
			totalObjectsCountInSnapshots, totalObjectsInAxentaTable)
		fmt.Printf("   ⚠️ Это означает, что при создании снимков объекты загружались из Axenta Cloud API,\n")
		fmt.Printf("      но НЕ сохранялись в таблицу axenta_object_snapshots\n")
		fmt.Printf("   ⚠️ Сохранены только МЕТАДАННЫЕ о количестве объектов в partner_daily_snapshots\n\n")
	} else {
		fmt.Printf("   ✅ Все объекты из снимков сохранены в БД\n\n")
	}

	fmt.Printf("💡 ЧТО ЭТО ЗНАЧИТ:\n")
	fmt.Printf("   - В БД есть информация О КОЛИЧЕСТВЕ объектов: %d\n", totalObjectsCountInSnapshots)
	fmt.Printf("   - Но НЕТ информации О САМИХ ОБЪЕКТАХ (их названиях, ID и т.д.)\n")
	fmt.Printf("   - Для получения списка объектов нужно делать запрос к Axenta Cloud API\n")
	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
