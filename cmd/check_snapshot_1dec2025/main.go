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

	fmt.Printf("🔍 Проверка снимков за дату: %s\n\n", snapshotDate.Format("2006-01-02"))

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		log.Fatalf("Ошибка получения компаний: %v", err)
	}

	fmt.Printf("Найдено компаний: %d\n\n", len(companies))

	totalSnapshotsFound := 0
	totalContracts := 0

	// Проверяем каждую компанию
	for _, company := range companies {
		// Получаем tenant DB
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			fmt.Printf("⚠️ Компания %s (ID=%d): не удалось получить DB\n", company.Name, company.ID)
			continue
		}

		// Проверяем снимки за эту дату
		var snapshots []models.PartnerDailySnapshot
		if err := tenantDB.
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Find(&snapshots).Error; err != nil {
			fmt.Printf("⚠️ Компания %s (ID=%d): ошибка поиска снимков: %v\n", company.Name, company.ID, err)
			continue
		}

		if len(snapshots) > 0 {
			totalSnapshotsFound += len(snapshots)
			fmt.Printf("✅ Компания: %s (ID=%d, схема: %s)\n", company.Name, company.ID, company.DatabaseSchema)
			fmt.Printf("   Найдено снимков: %d\n", len(snapshots))

			// Группируем по договорам
			contractsMap := make(map[uint]int)
			for _, snapshot := range snapshots {
				contractsMap[snapshot.ContractID]++
				totalContracts++

				fmt.Printf("   📸 Снимок ID=%d:\n", snapshot.ID)
				fmt.Printf("      - Договор ID: %d\n", snapshot.ContractID)
				fmt.Printf("      - Partner Company ID: %d\n", snapshot.PartnerCompanyID)
				fmt.Printf("      - Дата снимка: %s\n", snapshot.SnapshotDate.Format("2006-01-02 15:04:05"))
				fmt.Printf("      - Активных объектов: %d\n", snapshot.ActiveObjectsCount)
				fmt.Printf("      - Всего объектов: %d\n", snapshot.TotalObjectsCount)
				fmt.Printf("      - Дневная стоимость: %.2f₽\n", snapshot.DailyCost.InexactFloat64())
				fmt.Printf("      - Статус: %s\n", snapshot.Status)
				if snapshot.Notes != "" {
					fmt.Printf("      - Примечания: %s\n", snapshot.Notes)
				}
				fmt.Printf("\n")
			}

			fmt.Printf("   Итого уникальных договоров со снимками: %d\n\n", len(contractsMap))
		} else {
			// Проверяем, есть ли вообще партнерские договоры у этой компании
			var contractsCount int64
			tenantDB.Model(&models.Contract{}).
				Where("contract_type = ? AND partner_company_id IS NOT NULL", "partner").
				Count(&contractsCount)

			if contractsCount > 0 {
				fmt.Printf("⚠️ Компания: %s (ID=%d, схема: %s)\n", company.Name, company.ID, company.DatabaseSchema)
				fmt.Printf("   Снимков за %s НЕ найдено (есть %d партнерских договоров)\n\n", snapshotDate.Format("2006-01-02"), contractsCount)
			}
		}
	}

	// Итоговая статистика
	fmt.Printf("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n")
	fmt.Printf("   - Всего компаний проверено: %d\n", len(companies))
	fmt.Printf("   - Всего снимков найдено: %d\n", totalSnapshotsFound)
	fmt.Printf("   - Всего записей снимков: %d\n", totalContracts)
	fmt.Printf(strings.Repeat("=", 60) + "\n")

	if totalSnapshotsFound == 0 {
		fmt.Printf("\n⚠️ ВНИМАНИЕ: Снимки за %s не найдены в базе данных!\n", snapshotDate.Format("2006-01-02"))
	} else {
		fmt.Printf("\n✅ Снимки за %s найдены в базе данных.\n", snapshotDate.Format("2006-01-02"))
	}
}
