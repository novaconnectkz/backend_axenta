package main

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Подключение к БД
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=postgres dbname=axenta_db port=5432 sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Не удалось подключиться к БД:", err)
	}

	fmt.Println("📊 ДЕТАЛЬНЫЙ ОТЧЕТ ПО ОБЪЕКТАМ")
	fmt.Println("==========================================")
	fmt.Println()

	// Получаем компании
	var companies []struct {
		ID             uint
		DatabaseSchema string
	}
	
	db.Exec("SET search_path TO public")
	db.Table("companies").Select("id, database_schema").Find(&companies)

	for _, company := range companies {
		tenantSchema := company.DatabaseSchema
		db.Exec("SET search_path TO " + tenantSchema)

		fmt.Printf("🏢 Компания: %s (ID=%d)\n", tenantSchema, company.ID)
		fmt.Println("==========================================")

		// Проверяем таблицу axenta_account_snapshots
		var accountCount int64
		err := db.Table("axenta_account_snapshots").Count(&accountCount).Error
		if err != nil {
			fmt.Printf("   ⚠️ Таблица axenta_account_snapshots: %v\n", err)
		} else {
			fmt.Printf("   📋 Снимков аккаунтов: %d\n", accountCount)
			
			if accountCount > 0 {
				var totalObjects int
				db.Table("axenta_account_snapshots").
					Select("COALESCE(SUM(objects_total), 0)").
					Scan(&totalObjects)
				
				var activeObjects int
				db.Table("axenta_account_snapshots").
					Select("COALESCE(SUM(objects_active), 0)").
					Scan(&activeObjects)
				
				fmt.Printf("      - Всего объектов в аккаунтах: %d\n", totalObjects)
				fmt.Printf("      - Активных объектов: %d\n", activeObjects)
			}
		}

		// Проверяем таблицу axenta_object_snapshots
		var objectCount int64
		err = db.Table("axenta_object_snapshots").Count(&objectCount).Error
		if err != nil {
			fmt.Printf("   ⚠️ Таблица axenta_object_snapshots: %v\n", err)
		} else {
			fmt.Printf("   📦 Снимков объектов: %d\n", objectCount)
			
			if objectCount > 0 {
				var activeCount int64
				db.Table("axenta_object_snapshots").
					Where("is_active = ?", true).
					Count(&activeCount)
				fmt.Printf("      - Активных: %d\n", activeCount)
				fmt.Printf("      - Неактивных: %d\n", objectCount-activeCount)
			}
		}

		// Проверяем таблицу partner_daily_snapshots
		var dailyCount int64
		err = db.Table("partner_daily_snapshots").Count(&dailyCount).Error
		if err != nil {
			fmt.Printf("   ⚠️ Таблица partner_daily_snapshots: %v\n", err)
		} else {
			fmt.Printf("   📸 Ежедневных снимков договоров: %d\n", dailyCount)
			
			if dailyCount > 0 {
				// Получаем статистику по последнему снимку
				var lastSnapshot struct {
					ID                 uint
					ContractID         uint
					SnapshotDate       string
					TotalObjectsCount  int
					ActiveObjectsCount int
				}
				
				db.Table("partner_daily_snapshots").
					Select("id, contract_id, snapshot_date, total_objects_count, active_objects_count").
					Order("snapshot_date DESC, id DESC").
					First(&lastSnapshot)
				
				if lastSnapshot.ID > 0 {
					fmt.Printf("\n   📅 Последний снимок (ID=%d, Договор=%d, Дата=%s):\n", 
						lastSnapshot.ID, lastSnapshot.ContractID, lastSnapshot.SnapshotDate)
					fmt.Printf("      - Всего объектов: %d\n", lastSnapshot.TotalObjectsCount)
					fmt.Printf("      - Активных объектов: %d\n", lastSnapshot.ActiveObjectsCount)
				}

				// Получаем общую статистику по всем снимкам
				var totalStats struct {
					TotalObjects  int
					ActiveObjects int
					AvgTotal      float64
					AvgActive     float64
				}
				
				db.Table("partner_daily_snapshots").
					Select(`
						SUM(total_objects_count) as total_objects,
						SUM(active_objects_count) as active_objects,
						AVG(total_objects_count) as avg_total,
						AVG(active_objects_count) as avg_active
					`).
					Scan(&totalStats)
				
				fmt.Printf("\n   📊 Статистика по всем снимкам:\n")
				fmt.Printf("      - Сумма всех объектов: %d\n", totalStats.TotalObjects)
				fmt.Printf("      - Сумма активных объектов: %d\n", totalStats.ActiveObjects)
				fmt.Printf("      - Среднее объектов на снимок: %.0f\n", totalStats.AvgTotal)
				fmt.Printf("      - Среднее активных на снимок: %.0f\n", totalStats.AvgActive)
			}
		}

		// Проверяем таблицу contracts
		var contractCount int64
		err = db.Table("contracts").
			Where("contract_type = ?", "partner").
			Count(&contractCount).Error
		if err != nil {
			fmt.Printf("   ⚠️ Таблица contracts: %v\n", err)
		} else {
			fmt.Printf("\n   📋 Партнерских договоров: %d\n", contractCount)
			
			if contractCount > 0 {
				var contracts []struct {
					ID               uint
					Number           string
					Status           string
					PartnerCompanyID uint
				}
				
				db.Table("contracts").
					Select("id, number, status, partner_company_id").
					Where("contract_type = ?", "partner").
					Find(&contracts)
				
				for _, contract := range contracts {
					fmt.Printf("      - ID=%d, Номер=%s, Статус=%s, PartnerCompanyID=%d\n",
						contract.ID, contract.Number, contract.Status, contract.PartnerCompanyID)
				}
			}
		}

		fmt.Println()
	}

	fmt.Println("==========================================")
}

