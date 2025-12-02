package main

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type SnapshotStats struct {
	TotalAccountSnapshots int64
	TotalObjectSnapshots  int64
	TotalDailySnapshots   int64
	TotalJobs             int64
	CompletedJobs         int64
	FailedJobs            int64
}

type AccountSnapshot struct {
	ID                uint   `gorm:"primarykey"`
	AdminAccountID    uint   `gorm:"index"`
	ExternalAccountID int64  
	AccountName       string 
	AccountType       string 
	IsActive          bool   
	ObjectsActive     int    
	ObjectsTotal      int    
}

type ObjectSnapshot struct {
	ID                uint  `gorm:"primarykey"`
	AdminAccountID    uint  `gorm:"index"`
	AccountExternalID int64 
	ExternalObjectID  int64 
	ObjectName        string
	IsActive          bool  
}

type DailySnapshot struct {
	ID                  uint `gorm:"primarykey"`
	ContractID          uint
	TotalObjectsCount   int
	ActiveObjectsCount  int
}

type SnapshotJob struct {
	ID           uint   `gorm:"primarykey"`
	Status       string
	TotalContracts int
	SuccessCount int
	ErrorCount   int
}

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

	stats := SnapshotStats{}

	// Получаем статистику из основной БД (public схема)
	db.Exec("SET search_path TO public")
	
	db.Model(&SnapshotJob{}).Count(&stats.TotalJobs)
	db.Model(&SnapshotJob{}).Where("status = ?", "completed").Count(&stats.CompletedJobs)
	db.Model(&SnapshotJob{}).Where("status = ?", "failed").Count(&stats.FailedJobs)

	fmt.Println("📊 ОТЧЕТ ПО СНИМКАМ")
	fmt.Println("==========================================")
	fmt.Println()
	
	fmt.Printf("📋 Задачи создания снимков (public.snapshot_jobs):\n")
	fmt.Printf("   Всего задач: %d\n", stats.TotalJobs)
	fmt.Printf("   ✅ Завершено успешно: %d\n", stats.CompletedJobs)
	fmt.Printf("   ❌ Завершено с ошибками: %d\n", stats.FailedJobs)
	fmt.Println()

	// Получаем последние задачи
	var jobs []SnapshotJob
	db.Order("id DESC").Limit(5).Find(&jobs)
	
	if len(jobs) > 0 {
		fmt.Println("🕐 Последние 5 задач:")
		for _, job := range jobs {
			fmt.Printf("   ID=%d, Статус=%s, Договоров=%d, Успешно=%d, Ошибок=%d\n",
				job.ID, job.Status, job.TotalContracts, job.SuccessCount, job.ErrorCount)
		}
		fmt.Println()
	}

	// Статистика по тенантам
	var companies []struct {
		ID             uint
		DatabaseSchema string
	}
	
	db.Table("companies").Select("id, database_schema").Find(&companies)
	
	fmt.Printf("🏢 Тенанты (всего: %d):\n", len(companies))
	fmt.Println()

	for _, company := range companies {
		// Переключаемся на схему тенанта
		tenantSchema := company.DatabaseSchema
		db.Exec("SET search_path TO " + tenantSchema)

		var accountCount, objectCount, dailyCount int64
		
		db.Table("axenta_account_snapshots").Count(&accountCount)
		db.Table("axenta_object_snapshots").Count(&objectCount)
		db.Table("partner_daily_snapshots").Count(&dailyCount)

		if accountCount > 0 || objectCount > 0 || dailyCount > 0 {
			fmt.Printf("   📂 %s (ID=%d):\n", tenantSchema, company.ID)
			fmt.Printf("      - Снимков аккаунтов (axenta_account_snapshots): %d\n", accountCount)
			fmt.Printf("      - Снимков объектов (axenta_object_snapshots): %d\n", objectCount)
			fmt.Printf("      - Ежедневных снимков (partner_daily_snapshots): %d\n", dailyCount)
			
			stats.TotalAccountSnapshots += accountCount
			stats.TotalObjectSnapshots += objectCount
			stats.TotalDailySnapshots += dailyCount

			// Показываем примеры аккаунтов
			if accountCount > 0 {
				var accounts []AccountSnapshot
				db.Table("axenta_account_snapshots").Limit(3).Find(&accounts)
				if len(accounts) > 0 {
					fmt.Printf("      Примеры аккаунтов:\n")
					for _, acc := range accounts {
						status := "❌"
						if acc.IsActive {
							status = "✅"
						}
						fmt.Printf("         %s ID=%d, Name=%s, Type=%s, Объектов=%d/%d\n",
							status, acc.ExternalAccountID, acc.AccountName, acc.AccountType, 
							acc.ObjectsActive, acc.ObjectsTotal)
					}
				}
			}
			fmt.Println()
		}
	}

	fmt.Println("==========================================")
	fmt.Printf("📊 ИТОГО:\n")
	fmt.Printf("   Всего снимков аккаунтов: %d\n", stats.TotalAccountSnapshots)
	fmt.Printf("   Всего снимков объектов: %d\n", stats.TotalObjectSnapshots)
	fmt.Printf("   Всего ежедневных снимков: %d\n", stats.TotalDailySnapshots)
	fmt.Println("==========================================")
}

