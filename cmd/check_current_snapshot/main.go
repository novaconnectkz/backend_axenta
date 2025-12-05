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

	// Дата снимка - сегодня
	now := time.Now().UTC()
	snapshotDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	fmt.Printf("📊 Проверка снимков за %s (сегодня)\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf(strings.Repeat("=", 80) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка получения компаний: %v\n", err)
		os.Exit(1)
	}

	totalSnapshots := 0
	totalObjects := 0
	totalActiveObjects := 0

	// Проверяем снимки в каждой компании
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
		fmt.Printf("🏢 Компания: %s (схема: %s)\n", company.Name, company.DatabaseSchema)
		fmt.Printf("   Найдено снимков: %d\n\n", len(snapshots))

		for _, snapshot := range snapshots {
			contractInfo := fmt.Sprintf("Договор ID=%d", snapshot.ContractID)
			if snapshot.ContractID == 0 {
				contractInfo = "Без договора"
			}

			fmt.Printf("   📸 Снимок ID=%d\n", snapshot.ID)
			fmt.Printf("      - Partner Company ID: %d\n", snapshot.PartnerCompanyID)
			fmt.Printf("      - %s\n", contractInfo)
			fmt.Printf("      - Всего объектов: %d\n", snapshot.TotalObjectsCount)
			fmt.Printf("      - ✅ Активных: %d\n", snapshot.ActiveObjectsCount)
			fmt.Printf("      - ❌ Неактивных: %d\n", snapshot.TotalObjectsCount-snapshot.ActiveObjectsCount)
			fmt.Printf("      - Дневная стоимость: %.2f₽\n", snapshot.DailyCost.InexactFloat64())
			fmt.Printf("      - Статус: %s\n", snapshot.Status)

			totalObjects += snapshot.TotalObjectsCount
			totalActiveObjects += snapshot.ActiveObjectsCount

			if snapshot.Notes != "" {
				fmt.Printf("      - Примечания: %s\n", snapshot.Notes)
			}
			fmt.Printf("\n")
		}
	}

	// Итоговая статистика
	fmt.Printf(strings.Repeat("=", 80) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА ПО СНИМКАМ В БД:\n")
	fmt.Printf("   - Дата: %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("   - Всего снимков: %d\n", totalSnapshots)
	fmt.Printf("   - Всего объектов: %d\n", totalObjects)
	fmt.Printf("   - ✅ Активных объектов: %d\n", totalActiveObjects)
	fmt.Printf("   - ❌ Неактивных объектов: %d\n", totalObjects-totalActiveObjects)
	fmt.Printf(strings.Repeat("=", 80) + "\n")

	if totalSnapshots == 0 {
		fmt.Printf("\n⚠️ Снимки за сегодня не найдены в базе данных.\n")
		os.Exit(1)
	}
}
