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

	// Дата снимка - сегодня
	now := time.Now().UTC()
	snapshotDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	fmt.Printf("📸 Создание снимка за %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf(strings.Repeat("=", 80) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка получения компаний: %v\n", err)
		os.Exit(1)
	}

	totalSnapshotsCreated := 0
	totalObjects := 0
	totalActiveObjects := 0
	errors := []string{}

	// Обрабатываем каждую компанию
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		// Получаем токен из настроек снимков
		var snapshotSettings models.SnapshotSettings
		if err := tenantDB.Where("company_id = ?", 1).First(&snapshotSettings).Error; err != nil {
			continue
		}

		if snapshotSettings.AxentaToken == "" {
			fmt.Printf("⚠️ Компания %s: токен не настроен\n\n", company.Name)
			continue
		}

		// Получаем все партнерские договоры
		var contracts []models.Contract
		if err := tenantDB.
			Where("contract_type = ? AND partner_company_id IS NOT NULL AND tariff_plan_id IS NOT NULL", "partner").
			Find(&contracts).Error; err != nil {
			continue
		}

		if len(contracts) == 0 {
			continue
		}

		fmt.Printf("🏢 Компания: %s (схема: %s)\n", company.Name, company.DatabaseSchema)
		fmt.Printf("   Найдено партнерских договоров: %d\n\n", len(contracts))

		// Создаем сервис снимков
		snapshotService := services.NewPartnerSnapshotService()

		// Создаем снимки для каждого договора
		for _, contract := range contracts {
			fmt.Printf("   📋 Договор ID=%d, Partner Company ID=%d\n", contract.ID, *contract.PartnerCompanyID)

			// Проверяем, есть ли уже снимок за эту дату
			var existingSnapshot models.PartnerDailySnapshot
			if err := tenantDB.
				Where("contract_id = ? AND DATE(snapshot_date AT TIME ZONE 'UTC') = ?", contract.ID, snapshotDate.Format("2006-01-02")).
				First(&existingSnapshot).Error; err == nil {
				fmt.Printf("      ✅ Снимок уже существует (ID=%d, объектов: %d, активных: %d)\n",
					existingSnapshot.ID, existingSnapshot.TotalObjectsCount, existingSnapshot.ActiveObjectsCount)
				totalSnapshotsCreated++
				totalObjects += existingSnapshot.TotalObjectsCount
				totalActiveObjects += existingSnapshot.ActiveObjectsCount
				continue
			}

			// Создаем новый снимок
			if err := snapshotService.CreateSnapshotForContractWithTokenAndDB(&contract, snapshotDate, snapshotSettings.AxentaToken, tenantDB); err != nil {
				errorMsg := fmt.Sprintf("Договор %d: %v", contract.ID, err)
				errors = append(errors, errorMsg)
				fmt.Printf("      ❌ Ошибка создания снимка: %v\n", err)
				continue
			}

			// Получаем созданный снимок
			var newSnapshot models.PartnerDailySnapshot
			if err := tenantDB.
				Where("contract_id = ? AND DATE(snapshot_date AT TIME ZONE 'UTC') = ?", contract.ID, snapshotDate.Format("2006-01-02")).
				First(&newSnapshot).Error; err != nil {
				fmt.Printf("      ⚠️ Снимок создан, но не найден в БД: %v\n", err)
				continue
			}

			fmt.Printf("      ✅ Снимок создан (ID=%d, объектов: %d, активных: %d)\n",
				newSnapshot.ID, newSnapshot.TotalObjectsCount, newSnapshot.ActiveObjectsCount)
			totalSnapshotsCreated++
			totalObjects += newSnapshot.TotalObjectsCount
			totalActiveObjects += newSnapshot.ActiveObjectsCount
		}

		fmt.Printf("\n")
	}

	// Итоговая статистика
	fmt.Printf(strings.Repeat("=", 80) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n")
	fmt.Printf("   - Дата снимка: %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("   - Создано/найдено снимков: %d\n", totalSnapshotsCreated)
	fmt.Printf("   - Всего объектов: %d\n", totalObjects)
	fmt.Printf("   - Активных объектов: %d\n", totalActiveObjects)
	fmt.Printf("   - Неактивных объектов: %d\n", totalObjects-totalActiveObjects)

	if len(errors) > 0 {
		fmt.Printf("\n⚠️ ОШИБКИ (%d):\n", len(errors))
		for i, err := range errors {
			fmt.Printf("   %d. %s\n", i+1, err)
		}
	}

	// Подробная информация по каждому снимку
	fmt.Printf("\n" + strings.Repeat("=", 80) + "\n")
	fmt.Printf("📋 ДЕТАЛИ ПО ВСЕМ СНИМКАМ (включая без договоров):\n\n")

	totalAllSnapshots := 0
	totalAllObjects := 0
	totalAllActiveObjects := 0

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

		totalAllSnapshots += len(snapshots)
		fmt.Printf("🏢 Компания: %s (схема: %s)\n", company.Name, company.DatabaseSchema)
		for _, snapshot := range snapshots {
			contractInfo := fmt.Sprintf("Договор ID=%d", snapshot.ContractID)
			if snapshot.ContractID == 0 {
				contractInfo = "Без договора (contract_id=0)"
			}

			fmt.Printf("   📸 Снимок ID=%d: Partner Company ID=%d, %s\n", snapshot.ID, snapshot.PartnerCompanyID, contractInfo)
			fmt.Printf("      - Всего объектов: %d\n", snapshot.TotalObjectsCount)
			fmt.Printf("      - Активных: %d\n", snapshot.ActiveObjectsCount)
			fmt.Printf("      - Дневная стоимость: %.2f₽\n", snapshot.DailyCost.InexactFloat64())
			fmt.Printf("      - Статус: %s\n", snapshot.Status)
			if snapshot.Notes != "" {
				fmt.Printf("      - Примечания: %s\n", snapshot.Notes)
			}
			fmt.Printf("\n")

			totalAllObjects += snapshot.TotalObjectsCount
			totalAllActiveObjects += snapshot.ActiveObjectsCount
		}
	}

	fmt.Printf(strings.Repeat("=", 80) + "\n")
	fmt.Printf("📊 ОБЩАЯ СТАТИСТИКА ПО ВСЕМ СНИМКАМ:\n")
	fmt.Printf("   - Всего снимков за %s: %d\n", snapshotDate.Format("2006-01-02"), totalAllSnapshots)
	fmt.Printf("   - Всего объектов: %d\n", totalAllObjects)
	fmt.Printf("   - Активных объектов: %d\n", totalAllActiveObjects)
	fmt.Printf("   - Неактивных объектов: %d\n", totalAllObjects-totalAllActiveObjects)
	fmt.Printf(strings.Repeat("=", 80) + "\n")

	fmt.Printf(strings.Repeat("=", 80) + "\n")
}
