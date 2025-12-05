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

	// Дата снимка: 1 декабря 2025 года
	snapshotDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	fmt.Printf("📋 Все объекты из снимков за %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка получения компаний: %v\n", err)
		os.Exit(1)
	}

	totalSnapshots := 0
	totalObjectsInSnapshots := 0
	totalActiveInSnapshots := 0
	totalObjectsInDB := 0

	// Обрабатываем каждую компанию
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		// Находим все снимки за эту дату
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
		fmt.Printf("🏢 Компания: %s (схема: %s)\n", company.Name, company.DatabaseSchema)
		fmt.Printf("   Найдено снимков: %d\n\n", len(snapshots))

		// Обрабатываем каждый снимок
		for _, snapshot := range snapshots {
			contractInfo := fmt.Sprintf("Договор ID=%d", snapshot.ContractID)
			if snapshot.ContractID == 0 {
				contractInfo = "Без договора (contract_id=0)"
			}

			fmt.Printf("   📸 Снимок ID=%d: Partner Company ID=%d, %s\n", snapshot.ID, snapshot.PartnerCompanyID, contractInfo)
			fmt.Printf("      - Всего объектов в снимке: %d\n", snapshot.TotalObjectsCount)
			fmt.Printf("      - Активных: %d\n", snapshot.ActiveObjectsCount)
			fmt.Printf("      - Неактивных: %d\n", snapshot.TotalObjectsCount-snapshot.ActiveObjectsCount)

			totalObjectsInSnapshots += snapshot.TotalObjectsCount
			totalActiveInSnapshots += snapshot.ActiveObjectsCount

			// Ищем объекты в БД для этого партнера
			snapshotEndOfDay := time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 23, 59, 59, 999999999, time.UTC)

			var objects []models.AxentaObjectSnapshot
			tenantDB.Where("account_external_id = ? AND last_synced_at <= ?", int64(snapshot.PartnerCompanyID), snapshotEndOfDay).
				Order("object_name ASC").
				Find(&objects)

			if len(objects) > 0 {
				fmt.Printf("      - Найдено объектов в БД: %d\n", len(objects))
				totalObjectsInDB += len(objects)

				// Показываем первые 10 объектов, если их много
				maxShow := 10
				if len(objects) <= maxShow {
					fmt.Printf("\n      📋 Список объектов:\n")
					for i, obj := range objects {
						statusIcon := "✅"
						if !obj.IsActive {
							statusIcon = "❌"
						}
						fmt.Printf("         %d. %s %s (ID: %d)\n", i+1, statusIcon, obj.ObjectName, obj.ExternalObjectID)
						if obj.UniqueID != "" {
							fmt.Printf("            Уникальный ID: %s\n", obj.UniqueID)
						}
						if obj.DeviceTypeName != "" {
							fmt.Printf("            Тип: %s\n", obj.DeviceTypeName)
						}
					}
				} else {
					fmt.Printf("\n      📋 Первые %d объектов из %d:\n", maxShow, len(objects))
					for i, obj := range objects[:maxShow] {
						statusIcon := "✅"
						if !obj.IsActive {
							statusIcon = "❌"
						}
						fmt.Printf("         %d. %s %s (ID: %d)\n", i+1, statusIcon, obj.ObjectName, obj.ExternalObjectID)
					}
					fmt.Printf("         ... и еще %d объектов\n", len(objects)-maxShow)
				}
			} else {
				fmt.Printf("      - ⚠️ Объекты в БД не найдены (возможно, были загружены через API при создании снимка)\n")
			}

			if snapshot.Notes != "" {
				fmt.Printf("      - Примечания: %s\n", snapshot.Notes)
			}

			fmt.Printf("\n")
		}
	}

	// Итоговая статистика
	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n")
	fmt.Printf("   - Дата: %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("   - Всего снимков: %d\n", totalSnapshots)
	fmt.Printf("   - Всего объектов в снимках: %d\n", totalObjectsInSnapshots)
	fmt.Printf("   - Активных объектов в снимках: %d\n", totalActiveInSnapshots)
	fmt.Printf("   - Неактивных объектов в снимках: %d\n", totalObjectsInSnapshots-totalActiveInSnapshots)
	fmt.Printf("   - Найдено объектов в БД: %d\n", totalObjectsInDB)
	if totalObjectsInSnapshots > totalObjectsInDB {
		fmt.Printf("   - ⚠️ Разница: %d объектов не синхронизированы в БД (были учтены через API)\n", totalObjectsInSnapshots-totalObjectsInDB)
	}
	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
