package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"

	"gorm.io/gorm"
)

func main() {
	log.Println("🔍 Проверка таблицы snapshot_jobs в БД...")

	// Загружаем конфигурацию
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Не удалось загрузить конфигурацию: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Не удалось подключиться к базе данных: %v", err)
	}
	defer func() {
		sqlDB, _ := database.DB.DB()
		sqlDB.Close()
	}()

	// Проверяем в схеме public
	fmt.Println("\n📋 Проверка схемы public:")
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Ошибка переключения на схему public: %v", err)
	} else {
		if publicDB.Migrator().HasTable(&models.SnapshotJob{}) {
			var count int64
			publicDB.Model(&models.SnapshotJob{}).Count(&count)
			fmt.Printf("   ✅ Таблица snapshot_jobs существует в public\n")
			fmt.Printf("   📊 Количество записей: %d\n", count)
			
			if count > 0 {
				var jobs []models.SnapshotJob
				publicDB.Order("id DESC").Limit(5).Find(&jobs)
				fmt.Println("   Последние 5 записей:")
				for _, job := range jobs {
					fmt.Printf("      - ID: %d, Статус: %s, Тип: %s, Начало: %s\n", 
						job.ID, job.Status, job.JobType, job.StartedAt.Format("2006-01-02 15:04:05"))
				}
			}
		} else {
			fmt.Printf("   ❌ Таблица snapshot_jobs НЕ существует в public\n")
		}
	}

	// Получаем все компании и проверяем в их схемах
	fmt.Println("\n📋 Проверка тенантных схем:")
	var companies []models.Company
	mainDB := database.DB.Session(&gorm.Session{})
	if err := mainDB.Exec("SET search_path TO public").Error; err != nil {
		log.Fatalf("❌ Ошибка переключения на схему public: %v", err)
	}
	
	if err := mainDB.Find(&companies).Error; err != nil {
		log.Fatalf("❌ Ошибка получения списка компаний: %v", err)
	}

	fmt.Printf("   Найдено компаний: %d\n", len(companies))

	for _, company := range companies {
		fmt.Printf("\n   🏢 Компания: %s (ID=%d, схема: %s)\n", company.Name, company.ID, company.DatabaseSchema)
		
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			fmt.Printf("      ⚠️ Не удалось получить DB для тенанта\n")
			continue
		}

		if tenantDB.Migrator().HasTable(&models.SnapshotJob{}) {
			var count int64
			tenantDB.Model(&models.SnapshotJob{}).Count(&count)
			fmt.Printf("      ✅ Таблица snapshot_jobs существует\n")
			fmt.Printf("      📊 Количество записей: %d\n", count)
			
			if count > 0 {
				var jobs []models.SnapshotJob
				tenantDB.Order("id DESC").Limit(5).Find(&jobs)
				fmt.Println("      Последние 5 записей:")
				for _, job := range jobs {
					fmt.Printf("         - ID: %d, Статус: %s, Тип: %s, Начало: %s\n", 
						job.ID, job.Status, job.JobType, job.StartedAt.Format("2006-01-02 15:04:05"))
				}
			}
		} else {
			fmt.Printf("      ❌ Таблица snapshot_jobs НЕ существует\n")
		}
	}

	fmt.Println("\n✅ Проверка завершена")
}

