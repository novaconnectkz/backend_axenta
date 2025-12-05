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
	log.SetOutput(&NullWriter{})
	config.LoadConfig()

	if err := database.ConnectDatabase(); err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка подключения к БД: %v\n", err)
		os.Exit(1)
	}

	snapshotDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	fmt.Printf("📊 Объяснение статистики 'Договоров: 91'\n")
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка получения компаний: %v\n", err)
		os.Exit(1)
	}

	totalPartners := 0
	totalWithContracts := 0
	totalWithoutContracts := 0

	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		// Получаем все снимки за дату
		var snapshots []models.PartnerDailySnapshot
		if err := tenantDB.
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Find(&snapshots).Error; err != nil {
			continue
		}

		if len(snapshots) == 0 {
			continue
		}

		// Получаем договоры
		var contracts []models.Contract
		contractsByPartnerID := make(map[int64]*models.Contract)
		if err := tenantDB.
			Where("contract_type = ? AND status = ?", "partner", "active").
			Find(&contracts).Error; err == nil {
			for i := range contracts {
				if contracts[i].PartnerCompanyID != nil && *contracts[i].PartnerCompanyID > 0 {
					contractsByPartnerID[int64(*contracts[i].PartnerCompanyID)] = &contracts[i]
				}
			}
		}

		companyPartners := 0
		companyWithContracts := 0
		companyWithoutContracts := 0

		// Уникальные партнеры из снимков
		partnerSet := make(map[uint]bool)
		for _, snapshot := range snapshots {
			if !partnerSet[snapshot.PartnerCompanyID] {
				partnerSet[snapshot.PartnerCompanyID] = true
				companyPartners++

				if _, hasContract := contractsByPartnerID[int64(snapshot.PartnerCompanyID)]; hasContract {
					companyWithContracts++
				} else {
					companyWithoutContracts++
				}
			}
		}

		totalPartners += companyPartners
		totalWithContracts += companyWithContracts
		totalWithoutContracts += companyWithoutContracts

		fmt.Printf("🏢 Компания: %s\n", company.Name)
		fmt.Printf("   Всего партнеров: %d\n", companyPartners)
		fmt.Printf("   - С договорами: %d\n", companyWithContracts)
		fmt.Printf("   - Без договоров: %d\n", companyWithoutContracts)
		fmt.Printf("   - Всего договоров в БД: %d\n\n", len(contracts))
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГО:\n")
	fmt.Printf("   Всего партнеров (это и есть 'Договоров: 91'): %d\n", totalPartners)
	fmt.Printf("   - С реальными договорами: %d\n", totalWithContracts)
	fmt.Printf("   - Без договоров: %d\n\n", totalWithoutContracts)

	fmt.Printf("💡 ОБЪЯСНЕНИЕ:\n")
	fmt.Printf("   'Договоров: 91' - это НЕ количество договоров в БД!\n")
	fmt.Printf("   Это количество УНИКАЛЬНЫХ ПАРТНЕРОВ (partner_company_id),\n")
	fmt.Printf("   для которых есть объекты и нужно создать снимки.\n\n")

	fmt.Printf("   Из этих %d партнеров:\n", totalPartners)
	fmt.Printf("   - %d имеют реальные договоры в БД\n", totalWithContracts)
	fmt.Printf("   - %d работают БЕЗ договоров (используют дефолтный тариф)\n\n", totalWithoutContracts)

	fmt.Printf("   'Успешно: 46' означает:\n")
	fmt.Printf("   - Для 46 партнеров снимки были созданы или уже существовали\n")
	fmt.Printf("   - Остальные %d могли быть пропущены (snapshot already exists)\n", totalPartners-46)

	fmt.Printf(strings.Repeat("=", 100) + "\n")
}

type NullWriter struct{}

func (w *NullWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
