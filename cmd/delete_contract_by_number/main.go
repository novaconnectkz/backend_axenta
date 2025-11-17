package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"flag"
	"fmt"
	"log"
	"os"

	"gorm.io/gorm"
)

func main() {
	contractNumber := flag.String("number", "", "Номер договора для удаления (обязательно)")
	adminAccountID := flag.Uint("admin", 0, "ID администратора (admin_account_id)")
	force := flag.Bool("force", false, "Принудительное удаление без подтверждения")
	help := flag.Bool("help", false, "Показать справку")

	flag.Parse()

	if *help {
		printHelp()
		return
	}

	if *contractNumber == "" {
		log.Fatalf("❌ Ошибка: необходимо указать номер договора (--number)")
	}

	log.Printf("🔍 Поиск договора с номером: %s", *contractNumber)

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Не удалось загрузить конфигурацию: %v", err)
	}

	log.Printf("🔧 Подключение к базе данных: %s@%s:%s/%s",
		cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

	// Подключаемся к базе данных
	if err := database.ConnectDatabaseWithoutMigrations(); err != nil {
		log.Fatalf("❌ Не удалось подключиться к базе данных: %v", err)
	}

	log.Println("✅ Подключение к базе данных установлено")

	db := database.GetDB()

	// Получаем список всех компаний для поиска в tenant схемах
	var companies []models.Company
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на схему public: %v", err)
	}
	if err := db.Find(&companies).Error; err != nil {
		log.Printf("⚠️ Не удалось получить список компаний: %v", err)
	}

	var contract models.Contract
	var foundCompany *models.Company
	found := false

	// Сначала пробуем найти в основной схеме
	log.Println("🔍 Поиск в основной схеме...")
	query := db.Where("number = ?", *contractNumber)
	if *adminAccountID > 0 {
		query = query.Where("admin_account_id = ?", *adminAccountID)
	}
	if err := query.First(&contract).Error; err == nil {
		found = true
		log.Println("   ✅ Договор найден в основной схеме")
	}

	// Если не найден, ищем в tenant схемах
	if !found {
		log.Println("🔍 Поиск в tenant схемах...")
		for _, company := range companies {
			if !company.IsActive || company.DatabaseSchema == "" {
				continue
			}
			
			log.Printf("   Проверяем схему: %s (компания: %s)", company.DatabaseSchema, company.Name)
			
			if err := db.Exec(fmt.Sprintf("SET search_path TO %s", company.DatabaseSchema)).Error; err != nil {
				log.Printf("   ⚠️ Не удалось переключиться на схему %s: %v", company.DatabaseSchema, err)
				continue
			}

			query := db.Where("number = ?", *contractNumber)
			if *adminAccountID > 0 {
				query = query.Where("admin_account_id = ?", *adminAccountID)
			}
			
			if err := query.First(&contract).Error; err == nil {
				found = true
				foundCompany = &company
				log.Printf("   ✅ Договор найден в схеме %s (компания: %s)", company.DatabaseSchema, company.Name)
				break
			}
		}
	}

	if !found {
		// Пробуем поиск с разными вариантами номера (латинская/кириллическая T)
		log.Println("🔍 Пробуем поиск с альтернативными вариантами номера...")
		
		// Переключаемся обратно на public для поиска по всем схемам
		db.Exec("SET search_path TO public")
		
		// Пробуем с латинской T
		altNumber := *contractNumber
		if len(altNumber) > 0 {
			runes := []rune(altNumber)
			if len(runes) > 0 {
				firstRune := runes[0]
				if firstRune == 'Т' { // Кириллическая Т
					altNumber = "T" + string(runes[1:])
				} else if firstRune == 'T' { // Латинская T
					altNumber = "Т" + string(runes[1:])
				}
			}
		}
		
		if altNumber != *contractNumber {
			log.Printf("   Пробуем номер: %s", altNumber)
			query := db.Where("number = ?", altNumber)
			if *adminAccountID > 0 {
				query = query.Where("admin_account_id = ?", *adminAccountID)
			}
			if err := query.First(&contract).Error; err == nil {
				found = true
				log.Println("   ✅ Договор найден с альтернативным номером")
			}
		}
		
		// Если все еще не найден, ищем в tenant схемах с альтернативным номером
		if !found {
			for _, company := range companies {
				if !company.IsActive || company.DatabaseSchema == "" {
					continue
				}
				
				if err := db.Exec(fmt.Sprintf("SET search_path TO %s", company.DatabaseSchema)).Error; err != nil {
					continue
				}

				query := db.Where("number = ?", altNumber)
				if *adminAccountID > 0 {
					query = query.Where("admin_account_id = ?", *adminAccountID)
				}
				
				if err := query.First(&contract).Error; err == nil {
					found = true
					foundCompany = &company
					log.Printf("   ✅ Договор найден в схеме %s с альтернативным номером", company.DatabaseSchema)
					break
				}
			}
		}
	}

	if !found {
		log.Fatalf("❌ Договор с номером '%s' не найден ни в одной схеме", *contractNumber)
	}

	// Переключаемся на схему, где найден договор
	if foundCompany != nil {
		if err := db.Exec(fmt.Sprintf("SET search_path TO %s", foundCompany.DatabaseSchema)).Error; err != nil {
			log.Fatalf("❌ Не удалось переключиться на схему %s: %v", foundCompany.DatabaseSchema, err)
		}
	}

	log.Printf("✅ Договор найден:")
	log.Printf("   ID: %d", contract.ID)
	log.Printf("   Номер: %s", contract.Number)
	log.Printf("   Название: %s", contract.Title)
	log.Printf("   Клиент: %s", contract.ClientName)
	log.Printf("   Статус: %s", contract.Status)
	log.Printf("   Admin Account ID: %d", contract.AdminAccountID)
	log.Printf("   Company ID: %d", contract.CompanyID)

	if !*force {
		fmt.Print("\n⚠️  Вы уверены, что хотите удалить этот договор? (yes/no): ")
		var confirmation string
		fmt.Scanln(&confirmation)
		if confirmation != "yes" && confirmation != "y" {
			log.Println("❌ Удаление отменено")
			os.Exit(0)
		}
	}

	log.Println("\n🗑️  Начинаем удаление договора...")

	// 1. Отвязываем объекты от договора
	log.Println("1. Отвязываем объекты от договора...")
	if err := db.Model(&models.Object{}).Where("contract_id = ?", contract.ID).UpdateColumn("contract_id", gorm.Expr("NULL")).Error; err != nil {
		log.Printf("⚠️  Не удалось отвязать объекты: %v", err)
	} else {
		log.Println("   ✅ Объекты отвязаны")
	}

	// 2. Удаляем связи из junction table
	log.Println("2. Удаляем связи из junction table...")
	if err := db.Where("contract_id = ?", contract.ID).Delete(&models.ContractObject{}).Error; err != nil {
		log.Printf("⚠️  Не удалось удалить связи: %v (возможно, таблица отсутствует)", err)
	} else {
		log.Println("   ✅ Связи удалены")
	}

	// 3. Удаляем приложения к договору
	log.Println("3. Удаляем приложения к договору...")
	if err := db.Where("contract_id = ?", contract.ID).Delete(&models.ContractAppendix{}).Error; err != nil {
		log.Printf("⚠️  Не удалось удалить приложения: %v (возможно, таблица отсутствует)", err)
	} else {
		log.Println("   ✅ Приложения удалены")
	}

	// 4. Удаляем записи из истории биллинга
	log.Println("4. Удаляем записи из истории биллинга...")
	if err := db.Where("contract_id = ?", contract.ID).Delete(&models.BillingHistory{}).Error; err != nil {
		log.Printf("⚠️  Не удалось удалить записи истории: %v (возможно, таблица отсутствует)", err)
	} else {
		log.Println("   ✅ Записи истории удалены")
	}

	// 5. Удаляем счета, связанные с договором
	log.Println("5. Удаляем счета, связанные с договором...")
	if err := db.Where("contract_id = ?", contract.ID).Delete(&models.Invoice{}).Error; err != nil {
		log.Printf("⚠️  Не удалось удалить счета: %v (возможно, таблица отсутствует)", err)
	} else {
		log.Println("   ✅ Счета удалены")
	}

	// 6. Удаляем сам договор (жесткое удаление)
	log.Println("6. Удаляем договор...")
	if err := db.Unscoped().Delete(&contract).Error; err != nil {
		log.Fatalf("❌ Ошибка при удалении договора: %v", err)
	}

	log.Println("\n✅ Договор успешно удален!")
}

func printHelp() {
	fmt.Println("🗑️  Удаление договора по номеру")
	fmt.Println("")
	fmt.Println("Использование:")
	fmt.Println("  go run cmd/delete_contract_by_number/main.go [флаги]")
	fmt.Println("")
	fmt.Println("Флаги:")
	fmt.Println("  --number STRING   Номер договора для удаления (обязательно)")
	fmt.Println("  --admin UINT      ID администратора (admin_account_id) - опционально")
	fmt.Println("  --force           Принудительное удаление без подтверждения")
	fmt.Println("  --help            Показать эту справку")
	fmt.Println("")
	fmt.Println("Примеры:")
	fmt.Println("  # Удалить договор с подтверждением")
	fmt.Println("  go run cmd/delete_contract_by_number/main.go --number 'Т-024-2025'")
	fmt.Println("")
	fmt.Println("  # Удалить договор без подтверждения")
	fmt.Println("  go run cmd/delete_contract_by_number/main.go --number 'Т-024-2025' --force")
	fmt.Println("")
	fmt.Println("  # Удалить договор с указанием admin_account_id")
	fmt.Println("  go run cmd/delete_contract_by_number/main.go --number 'Т-024-2025' --admin 1")
}

