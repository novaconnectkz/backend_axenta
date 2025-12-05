package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"os"
	"strings"
)

// NullWriter для подавления логов
type NullWriter struct{}

func (w *NullWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func main() {
	// Отключаем лишние логи для чистого вывода
	log.SetOutput(&NullWriter{})

	// Загружаем конфигурацию
	config.LoadConfig()

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Не удалось подключиться к базе данных: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		sqlDB, _ := database.DB.DB()
		sqlDB.Close()
	}()

	searchID := int64(17153)

	fmt.Printf("🔍 Поиск ID %d во всех таблицах и компаниях...\n", searchID)
	fmt.Printf("   Ищем по полям:\n")
	fmt.Printf("   - objects.id\n")
	fmt.Printf("   - axenta_object_snapshots.external_object_id\n")
	fmt.Printf("   - axenta_object_snapshots.account_external_id\n")
	fmt.Printf("   - companies.id\n")
	fmt.Printf("   - axenta_account_snapshots.external_account_id\n")
	fmt.Printf("   - contracts.id, contracts.partner_company_id\n")
	fmt.Printf("   - partner_daily_snapshots.partner_company_id\n")
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Table("public.companies").Find(&companies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "❌ Ошибка получения списка компаний: %v\n", err)
		os.Exit(1)
	}

	found := false
	totalFound := 0

	// Ищем объект в каждой компании
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		// Ищем в таблице objects по внутреннему ID
		var obj models.Object
		if err := tenantDB.Where("id = ?", searchID).First(&obj).Error; err == nil {
			found = true
			totalFound++
			fmt.Printf("✅ Найден в таблице objects (по id) для компании: %s (ID=%d, схема: %s)\n", company.Name, company.ID, company.DatabaseSchema)
			fmt.Printf("   📦 Объект:\n")
			fmt.Printf("      ID: %d\n", obj.ID)
			fmt.Printf("      Название: %s\n", obj.Name)
			fmt.Printf("      IMEI: %s\n", obj.IMEI)
			fmt.Printf("      Телефон: %s\n", obj.PhoneNumber)
			fmt.Printf("      Тип: %s\n", obj.Type)
			fmt.Printf("      Статус: %s\n", obj.Status)
			fmt.Printf("      Активен: %v\n", obj.IsActive)
			fmt.Printf("      Адрес: %s\n", obj.Address)
			fmt.Printf("      Создан: %s\n", obj.CreatedAt.Format("2006-01-02 15:04:05"))
			if obj.DeletedAt.Valid {
				fmt.Printf("      Удален: %s\n", obj.DeletedAt.Time.Format("2006-01-02 15:04:05"))
			}
			fmt.Println()
		}

		// Ищем в таблице axenta_object_snapshots по external_object_id
		var snapshotsByExternalID []models.AxentaObjectSnapshot
		if err := tenantDB.Where("external_object_id = ?", searchID).Order("last_synced_at DESC").Find(&snapshotsByExternalID).Error; err == nil && len(snapshotsByExternalID) > 0 {
			found = true
			totalFound += len(snapshotsByExternalID)
			fmt.Printf("✅ Найдено %d снимков в axenta_object_snapshots (по external_object_id) для компании: %s (ID=%d, схема: %s)\n", len(snapshotsByExternalID), company.Name, company.ID, company.DatabaseSchema)

			for i, snapshot := range snapshotsByExternalID {
				fmt.Printf("   📸 Снимок #%d:\n", i+1)
				fmt.Printf("      Axenta External ID: %d\n", snapshot.ExternalObjectID)
				fmt.Printf("      Account External ID: %d\n", snapshot.AccountExternalID)
				fmt.Printf("      Название: %s\n", snapshot.ObjectName)
				if snapshot.UniqueID != "" {
					fmt.Printf("      Unique ID: %s\n", snapshot.UniqueID)
				}
				if snapshot.PhoneNumbers != nil {
					fmt.Printf("      Телефоны: %s\n", *snapshot.PhoneNumbers)
				}
				if snapshot.DeviceTypeName != "" {
					fmt.Printf("      Тип устройства: %s\n", snapshot.DeviceTypeName)
				}
				if snapshot.AccountName != "" {
					fmt.Printf("      Имя аккаунта: %s\n", snapshot.AccountName)
				}
				fmt.Printf("      Статус: %s\n", snapshot.Status)
				fmt.Printf("      Активен: %v\n", snapshot.IsActive)
				if snapshot.CreatorName != nil {
					fmt.Printf("      Создатель: %s (ID: %v)\n", *snapshot.CreatorName, snapshot.CreatorID)
				}
				if snapshot.AxentaCreatedAt != nil {
					fmt.Printf("      Создан в Axenta: %s\n", snapshot.AxentaCreatedAt.Format("2006-01-02 15:04:05"))
				}
				if snapshot.AxentaDeletedAt != nil {
					fmt.Printf("      Удален в Axenta: %s\n", snapshot.AxentaDeletedAt.Format("2006-01-02 15:04:05"))
				}
				if snapshot.LastCommunicationAt != nil {
					fmt.Printf("      Последняя связь: %s\n", snapshot.LastCommunicationAt.Format("2006-01-02 15:04:05"))
				}
				fmt.Printf("      Последняя синхронизация: %s\n", snapshot.LastSyncedAt.Format("2006-01-02 15:04:05"))
				if snapshot.AdminAccountID != nil {
					fmt.Printf("      Admin Account ID: %d\n", *snapshot.AdminAccountID)
				}
				fmt.Println()
			}
		}

		// Ищем в таблице axenta_object_snapshots по account_external_id (может быть партнер)
		var snapshotsByAccountID []models.AxentaObjectSnapshot
		if err := tenantDB.Where("account_external_id = ?", searchID).Order("last_synced_at DESC").Find(&snapshotsByAccountID).Error; err == nil && len(snapshotsByAccountID) > 0 {
			found = true
			totalFound += len(snapshotsByAccountID)
			fmt.Printf("✅ Найдено %d снимков в axenta_object_snapshots (по account_external_id) для компании: %s (ID=%d, схема: %s)\n", len(snapshotsByAccountID), company.Name, company.ID, company.DatabaseSchema)
			fmt.Printf("   💡 Это объекты, принадлежащие аккаунту с ID %d\n\n", searchID)

			// Показываем только первые 10 объектов, чтобы не перегружать вывод
			maxShow := 10
			if len(snapshotsByAccountID) > maxShow {
				fmt.Printf("   (показаны первые %d из %d объектов)\n\n", maxShow, len(snapshotsByAccountID))
			}

			for i, snapshot := range snapshotsByAccountID {
				if i >= maxShow {
					break
				}
				fmt.Printf("   📸 Объект #%d:\n", i+1)
				fmt.Printf("      Axenta External ID: %d\n", snapshot.ExternalObjectID)
				fmt.Printf("      Account External ID: %d\n", snapshot.AccountExternalID)
				fmt.Printf("      Название: %s\n", snapshot.ObjectName)
				if snapshot.UniqueID != "" {
					fmt.Printf("      Unique ID: %s\n", snapshot.UniqueID)
				}
				fmt.Printf("      Активен: %v\n", snapshot.IsActive)
				fmt.Printf("      Последняя синхронизация: %s\n", snapshot.LastSyncedAt.Format("2006-01-02 15:04:05"))
				fmt.Println()
			}
		}

		// Ищем в таблице axenta_account_snapshots
		var accounts []models.AxentaAccountSnapshot
		if err := tenantDB.Where("external_account_id = ?", searchID).Order("last_synced_at DESC").Find(&accounts).Error; err == nil && len(accounts) > 0 {
			found = true
			totalFound += len(accounts)
			fmt.Printf("✅ Найдено %d аккаунтов в axenta_account_snapshots (по external_account_id) для компании: %s (ID=%d, схема: %s)\n", len(accounts), company.Name, company.ID, company.DatabaseSchema)
			for i, acc := range accounts {
				fmt.Printf("   👤 Аккаунт #%d:\n", i+1)
				fmt.Printf("      External Account ID: %d\n", acc.ExternalAccountID)
				fmt.Printf("      Название: %s\n", acc.AccountName)
				fmt.Printf("      Тип: %s\n", acc.AccountType)
				fmt.Printf("      Активен: %v\n", acc.IsActive)
				fmt.Printf("      Последняя синхронизация: %s\n", acc.LastSyncedAt.Format("2006-01-02 15:04:05"))
				fmt.Println()
			}
		}

		// Ищем в таблице contracts
		var contracts []models.Contract
		if err := tenantDB.Where("id = ? OR partner_company_id = ?", searchID, uint(searchID)).Find(&contracts).Error; err == nil && len(contracts) > 0 {
			found = true
			totalFound += len(contracts)
			fmt.Printf("✅ Найдено %d договоров для компании: %s (ID=%d, схема: %s)\n", len(contracts), company.Name, company.ID, company.DatabaseSchema)
			for i, contract := range contracts {
				fmt.Printf("   📄 Договор #%d:\n", i+1)
				fmt.Printf("      ID: %d\n", contract.ID)
				fmt.Printf("      Номер: %s\n", contract.Number)
				fmt.Printf("      Тип: %s\n", contract.ContractType)
				if contract.PartnerCompanyID != nil {
					fmt.Printf("      Partner Company ID: %d\n", *contract.PartnerCompanyID)
				}
				fmt.Println()
			}
		}

		// Ищем в таблице partner_daily_snapshots
		var snapshots []models.PartnerDailySnapshot
		if err := tenantDB.Where("partner_company_id = ?", searchID).Order("snapshot_date DESC").Find(&snapshots).Error; err == nil && len(snapshots) > 0 {
			found = true
			totalFound += len(snapshots)
			fmt.Printf("✅ Найдено %d снимков в partner_daily_snapshots (по partner_company_id) для компании: %s (ID=%d, схема: %s)\n", len(snapshots), company.Name, company.ID, company.DatabaseSchema)
			// Показываем только первые 5 снимков
			maxShow := 5
			if len(snapshots) > maxShow {
				fmt.Printf("   (показаны первые %d из %d снимков)\n\n", maxShow, len(snapshots))
			}
			for i, snapshot := range snapshots {
				if i >= maxShow {
					break
				}
				fmt.Printf("   📸 Снимок #%d:\n", i+1)
				fmt.Printf("      Дата: %s\n", snapshot.SnapshotDate.Format("2006-01-02"))
				fmt.Printf("      Partner Company ID: %d\n", snapshot.PartnerCompanyID)
				fmt.Printf("      Всего объектов: %d\n", snapshot.TotalObjectsCount)
				fmt.Printf("      Активных: %d\n", snapshot.ActiveObjectsCount)
				fmt.Printf("      Стоимость за день: %.2f₽\n", snapshot.DailyCost.InexactFloat64())
				fmt.Println()
			}
		}
	}

	// Проверяем в таблице companies (глобальная таблица)
	var company models.Company
	if err := database.DB.Where("id = ?", uint(searchID)).First(&company).Error; err == nil {
		found = true
		totalFound++
		fmt.Printf("✅ Найдена компания в таблице companies:\n")
		fmt.Printf("   🏢 Компания:\n")
		fmt.Printf("      ID: %d\n", company.ID)
		fmt.Printf("      Название: %s\n", company.Name)
		fmt.Printf("      Схема БД: %s\n", company.DatabaseSchema)
		fmt.Println()
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	if !found {
		fmt.Printf("❌ Объект с ID %d не найден ни в одной компании\n", searchID)
		fmt.Println("\n💡 Попробуйте проверить:")
		fmt.Println("   - Правильность ID объекта")
		fmt.Println("   - Синхронизацию данных из Axenta Cloud")
		fmt.Printf("   - Поиск выполнялся по полям: objects.id, axenta_object_snapshots.external_object_id, axenta_object_snapshots.account_external_id\n")
	} else {
		fmt.Printf("✅ Найдено записей: %d\n", totalFound)
	}
	fmt.Println("✅ Поиск завершен")
}
