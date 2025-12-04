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
	log.Println("🔍 Проверка статистики задач 30, 31, 32...")

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

	// Переключаемся на схему public
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Fatalf("❌ Ошибка переключения на схему public: %v", err)
	}

	// Получаем последние 5 задач
	var jobs []models.SnapshotJob
	if err := publicDB.Order("id DESC").Limit(5).Find(&jobs).Error; err != nil {
		log.Fatalf("❌ Ошибка получения задач: %v", err)
	}

	fmt.Printf("\n📋 Найдено задач: %d\n\n", len(jobs))

	for _, job := range jobs {
		fmt.Printf("📋 Задача #%d:\n", job.ID)
		fmt.Printf("   Статус: %s\n", job.Status)
		fmt.Printf("   Тип: %s\n", job.JobType)
		fmt.Printf("   Дата: %s - %s\n", job.DateFrom.Format("2006-01-02"), job.DateTo.Format("2006-01-02"))
		fmt.Printf("   Начало: %s\n", job.StartedAt.Format("2006-01-02 15:04:05"))
		if job.FinishedAt != nil {
			fmt.Printf("   Завершение: %s\n", job.FinishedAt.Format("2006-01-02 15:04:05"))
		}
		if job.DurationSeconds != nil {
			fmt.Printf("   Длительность: %d сек\n", *job.DurationSeconds)
		}
		fmt.Printf("   Компаний: %d\n", job.TotalCompanies)
		fmt.Printf("   Договоров: %d\n", job.TotalContracts)
		fmt.Printf("   Успешно: %d\n", job.SuccessCount)
		fmt.Printf("   Ошибок: %d\n", job.ErrorCount)
		fmt.Printf("   Пропущено: %d\n", job.SkippedCount)
		fmt.Printf("   Всего объектов: %d\n", job.TotalObjects)
		fmt.Printf("   Активных объектов: %d\n", job.ActiveObjects)
		
		// Проверяем детали компаний
		if len(job.Details.Companies) > 0 {
			fmt.Printf("\n   🏢 Детали компаний (%d):\n", len(job.Details.Companies))
			for _, company := range job.Details.Companies {
				fmt.Printf("      - Компания ID: %d, Название: %s\n", company.CompanyID, company.CompanyName)
				fmt.Printf("        Договоров: %d, Успешно: %d, Ошибок: %d\n", 
					company.ContractsCount, company.SuccessCount, company.ErrorCount)
			}
		}
		
		// Проверяем детали договоров
		if len(job.Details.Contracts) > 0 {
			fmt.Printf("\n   📋 Детали договоров (%d):\n", len(job.Details.Contracts))
			successContracts := 0
			errorContracts := 0
			for _, contract := range job.Details.Contracts {
				if contract.SuccessCount > 0 {
					successContracts++
				}
				if contract.ErrorCount > 0 {
					errorContracts++
				}
			}
			fmt.Printf("      Успешных: %d, С ошибками: %d\n", successContracts, errorContracts)
		} else {
			fmt.Printf("\n   ⚠️ Нет деталей договоров!\n")
		}
		
		fmt.Println()
	}

	fmt.Println("✅ Проверка завершена")
}

