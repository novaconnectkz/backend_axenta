package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
)

func main() {
	// Загружаем конфигурацию
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	fmt.Println("🔄 Обновление сроков привязки объектов к договорам...")

	// Получаем список всех tenant схем
	var schemas []string
	if err := database.DB.Raw(`
		SELECT schema_name 
		FROM information_schema.schemata 
		WHERE schema_name LIKE 'tenant_%'
		ORDER BY schema_name
	`).Scan(&schemas).Error; err != nil {
		log.Fatalf("❌ Не удалось получить список схем: %v", err)
	}

	totalUpdated := 0

	for _, schema := range schemas {
		fmt.Printf("\n📋 Обрабатываем схему: %s\n", schema)

		// Переключаемся на схему
		if err := database.DB.Exec(fmt.Sprintf("SET search_path TO %s", schema)).Error; err != nil {
			fmt.Printf("⚠️ Не удалось переключиться на схему %s: %v\n", schema, err)
			continue
		}

		// Проверяем, существует ли таблица contract_objects
		var tableExists bool
		if err := database.DB.Raw(`
			SELECT EXISTS (
				SELECT FROM information_schema.tables 
				WHERE table_schema = ? AND table_name = 'contract_objects'
			)
		`, schema).Scan(&tableExists).Error; err != nil {
			fmt.Printf("⚠️ Ошибка проверки таблицы contract_objects: %v\n", err)
			continue
		}

		if !tableExists {
			fmt.Printf("ℹ️ Таблица contract_objects не существует в схеме %s, пропускаем\n", schema)
			continue
		}

		// Проверяем, есть ли поля start_date и end_date
		var hasStartDate, hasEndDate bool
		database.DB.Raw(`
			SELECT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = ? AND table_name = 'contract_objects' AND column_name = 'start_date'
			)
		`, schema).Scan(&hasStartDate)
		database.DB.Raw(`
			SELECT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = ? AND table_name = 'contract_objects' AND column_name = 'end_date'
			)
		`, schema).Scan(&hasEndDate)

		if !hasStartDate || !hasEndDate {
			fmt.Printf("🔧 Добавляем поля start_date и end_date в таблицу contract_objects...\n")
			if !hasStartDate {
				if err := database.DB.Exec(`
					ALTER TABLE contract_objects 
					ADD COLUMN IF NOT EXISTS start_date TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
				`).Error; err != nil {
					fmt.Printf("⚠️ Ошибка добавления start_date: %v\n", err)
					continue
				}
			}
			if !hasEndDate {
				if err := database.DB.Exec(`
					ALTER TABLE contract_objects 
					ADD COLUMN IF NOT EXISTS end_date TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
				`).Error; err != nil {
					fmt.Printf("⚠️ Ошибка добавления end_date: %v\n", err)
					continue
				}
			}
			fmt.Printf("✅ Поля start_date и end_date добавлены\n")
		}

		// Получаем все связи contract_objects, у которых сроки не установлены или равны нулю
		var contractObjects []models.ContractObject
		if err := database.DB.Find(&contractObjects).Error; err != nil {
			fmt.Printf("⚠️ Ошибка получения contract_objects: %v\n", err)
			continue
		}

		updatedInSchema := 0
		for _, co := range contractObjects {
			// Если сроки не установлены (нулевые), получаем их из договора
			if co.StartDate.IsZero() || co.EndDate.IsZero() {
				var contract models.Contract
				if err := database.DB.First(&contract, co.ContractID).Error; err != nil {
					fmt.Printf("⚠️ Договор %d не найден для contract_object %d: %v\n", co.ContractID, co.ID, err)
					continue
				}

				// Обновляем сроки из договора
				updates := map[string]interface{}{
					"start_date": contract.StartDate,
					"end_date":   contract.EndDate,
				}
				if err := database.DB.Model(&co).Updates(updates).Error; err != nil {
					fmt.Printf("⚠️ Ошибка обновления contract_object %d: %v\n", co.ID, err)
					continue
				}

				updatedInSchema++
				fmt.Printf("✅ Обновлен contract_object %d: сроки установлены из договора %d (%s - %s)\n",
					co.ID, contract.ID,
					contract.StartDate.Format("2006-01-02"),
					contract.EndDate.Format("2006-01-02"))
			}
		}

		if updatedInSchema > 0 {
			fmt.Printf("✅ Обновлено записей в схеме %s: %d\n", schema, updatedInSchema)
			totalUpdated += updatedInSchema
		} else {
			fmt.Printf("ℹ️ Все записи в схеме %s уже имеют установленные сроки\n", schema)
		}
	}

	fmt.Printf("\n✅ Обновление завершено. Всего обновлено записей: %d\n", totalUpdated)
}

