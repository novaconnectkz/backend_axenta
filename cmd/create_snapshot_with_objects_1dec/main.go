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

	fmt.Printf("📸 Создание снимков за %s с сохранением всех объектов в БД\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка получения компаний: %v\n", err)
		os.Exit(1)
	}

	totalSnapshotsCreated := 0
	totalSnapshotsUpdated := 0
	totalObjectsSaved := 0
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
			fmt.Printf("⚠️ Компания %s: токен не настроен (пропускаем)\n\n", company.Name)
			continue
		}

		if snapshotSettings.AxentaToken == "" {
			fmt.Printf("⚠️ Компания %s: токен пустой (пропускаем)\n\n", company.Name)
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

			// Удаляем все существующие снимки за эту дату для этого партнера (включая без договора)
			result := tenantDB.Unscoped().
				Where("partner_company_id = ? AND DATE(snapshot_date AT TIME ZONE 'UTC') = ?", *contract.PartnerCompanyID, snapshotDate.Format("2006-01-02")).
				Delete(&models.PartnerDailySnapshot{})
			
			if result.Error != nil {
				errorMsg := fmt.Sprintf("Договор %d: ошибка удаления старых снимков: %v", contract.ID, result.Error)
				errors = append(errors, errorMsg)
				fmt.Printf("      ❌ Ошибка удаления старых снимков: %v\n", result.Error)
				continue
			}
			
			if result.RowsAffected > 0 {
				fmt.Printf("      🗑️ Удалено старых снимков для партнера %d: %d\n", *contract.PartnerCompanyID, result.RowsAffected)
			}

			// Создаем новый снимок (объекты будут автоматически сохранены)
			if err := snapshotService.CreateSnapshotForContractWithTokenAndDB(&contract, snapshotDate, snapshotSettings.AxentaToken, tenantDB); err != nil {
				errorMsg := fmt.Sprintf("Договор %d: %v", contract.ID, err)
				errors = append(errors, errorMsg)
				fmt.Printf("      ❌ Ошибка создания снимка: %v\n", err)
				continue
			}

			// Получаем созданный снимок для проверки
			var newSnapshot models.PartnerDailySnapshot
			if err := tenantDB.
				Where("contract_id = ? AND DATE(snapshot_date AT TIME ZONE 'UTC') = ?", contract.ID, snapshotDate.Format("2006-01-02")).
				First(&newSnapshot).Error; err != nil {
				fmt.Printf("      ⚠️ Снимок создан, но не найден в БД: %v\n", err)
				continue
			}

			// Проверяем, сколько объектов сохранено в БД
			var objectsCount int64
			snapshotEndOfDay := time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 23, 59, 59, 999999999, time.UTC)
			tenantDB.Model(&models.AxentaObjectSnapshot{}).
				Where("account_external_id = ? AND last_synced_at <= ?", int64(*contract.PartnerCompanyID), snapshotEndOfDay).
				Count(&objectsCount)

			totalSnapshotsCreated++
			totalObjectsSaved += int(objectsCount)

			fmt.Printf("      ✅ Снимок создан (ID=%d, объектов: %d, активных: %d)\n", 
				newSnapshot.ID, newSnapshot.TotalObjectsCount, newSnapshot.ActiveObjectsCount)
			fmt.Printf("      💾 Объектов сохранено в БД: %d\n\n", objectsCount)
		}

		fmt.Printf("\n")
	}

	// Итоговая статистика
	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n")
	fmt.Printf("   - Дата снимка: %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("   - Создано новых снимков: %d\n", totalSnapshotsCreated)
	fmt.Printf("   - Обновлено существующих снимков: %d\n", totalSnapshotsUpdated)
	fmt.Printf("   - Всего объектов сохранено в БД: %d\n", totalObjectsSaved)

	if len(errors) > 0 {
		fmt.Printf("\n⚠️ ОШИБКИ (%d):\n", len(errors))
		for i, err := range errors {
			fmt.Printf("   %d. %s\n", i+1, err)
		}
	}

	// Проверяем результаты сохранения
	fmt.Printf("\n" + strings.Repeat("=", 100) + "\n")
	fmt.Printf("🔍 ПРОВЕРКА СОХРАНЕННЫХ ОБЪЕКТОВ:\n\n")

	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		// Находим все снимки за эту дату
		var snapshots []models.PartnerDailySnapshot
		if err := tenantDB.
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Find(&snapshots).Error; err != nil {
			continue
		}

		if len(snapshots) == 0 {
			continue
		}

		fmt.Printf("🏢 Компания: %s\n", company.Name)
		
		totalInSnapshots := 0
		totalInDB := 0
		snapshotEndOfDay := time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 23, 59, 59, 999999999, time.UTC)

		for _, snapshot := range snapshots {
			totalInSnapshots += snapshot.TotalObjectsCount

			var objectsCount int64
			tenantDB.Model(&models.AxentaObjectSnapshot{}).
				Where("account_external_id = ? AND last_synced_at <= ?", int64(snapshot.PartnerCompanyID), snapshotEndOfDay).
				Count(&objectsCount)
			
			totalInDB += int(objectsCount)

			fmt.Printf("   📸 Partner Company ID=%d: в снимке=%d, в БД=%d", 
				snapshot.PartnerCompanyID, snapshot.TotalObjectsCount, objectsCount)
			
			if snapshot.TotalObjectsCount > int(objectsCount) {
				fmt.Printf(" ⚠️ (не все объекты сохранены)\n")
			} else {
				fmt.Printf(" ✅\n")
			}
		}

		fmt.Printf("   Итого: в снимках=%d, в БД=%d", totalInSnapshots, totalInDB)
		if totalInSnapshots > totalInDB {
			fmt.Printf(" ⚠️ (не все объекты сохранены)\n\n")
		} else {
			fmt.Printf(" ✅\n\n")
		}
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("✅ Готово! Снимки созданы, объекты сохранены в БД.\n")
}
