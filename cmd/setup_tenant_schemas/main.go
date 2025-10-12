package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"log"
)

func main() {
	log.Println("🏢 Настройка tenant схем для всех компаний")
	log.Println("==========================================")

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Не удалось загрузить конфигурацию: %v", err)
	}

	log.Printf("🔧 Подключение к базе данных: %s@%s:%s/%s",
		cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

	// Создаем базу данных если её нет
	if err := database.CreateDatabaseIfNotExists(); err != nil {
		log.Fatalf("❌ Не удалось создать базу данных: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Не удалось подключиться к базе данных: %v", err)
	}

	log.Println("✅ Подключение к базе данных установлено")

	// Получаем подключение к БД
	db := database.GetDB()

	// Убеждаемся, что мы в схеме public для работы с таблицей companies
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на схему public: %v", err)
	} else {
		log.Println("✅ Переключились на схему public")
	}

	// Создаем TenantMiddleware для работы со схемами
	tenantMiddleware := middleware.NewTenantMiddleware(db)

	// Получаем список всех компаний
	var companies []models.Company
	if err := db.Find(&companies).Error; err != nil {
		log.Printf("❌ Не удалось получить список компаний: %v", err)

		// Если таблица companies не существует или пуста, создаем компанию по умолчанию
		log.Println("🔧 Создаем компанию по умолчанию...")

		// Сначала убедимся, что таблица companies существует
		if !db.Migrator().HasTable(&models.Company{}) {
			log.Println("🔄 Создаем таблицу companies...")
			if err := db.AutoMigrate(&models.Company{}); err != nil {
				log.Fatalf("❌ Не удалось создать таблицу companies: %v", err)
			}
		}

		// Создаем компанию по умолчанию
		defaultCompany := &models.Company{
			Name:           "Компания по умолчанию",
			DatabaseSchema: "tenant_default",
			AxetnaLogin:    "default",
			AxetnaPassword: "encrypted_password",
			ContactEmail:   "admin@example.com",
			IsActive:       true,
		}

		if err := db.Create(defaultCompany).Error; err != nil {
			log.Printf("⚠️ Не удалось создать компанию по умолчанию: %v", err)
		} else {
			log.Println("✅ Компания по умолчанию создана")
			companies = append(companies, *defaultCompany)
		}
	}

	if len(companies) == 0 {
		log.Println("⚠️ Компании не найдены. Создаем компанию по умолчанию...")

		defaultCompany := &models.Company{
			Name:           "Компания по умолчанию",
			DatabaseSchema: "tenant_default",
			AxetnaLogin:    "default",
			AxetnaPassword: "encrypted_password",
			ContactEmail:   "admin@example.com",
			IsActive:       true,
		}

		if err := db.Create(defaultCompany).Error; err != nil {
			log.Fatalf("❌ Не удалось создать компанию по умолчанию: %v", err)
		}

		companies = append(companies, *defaultCompany)
		log.Println("✅ Компания по умолчанию создана")
	}

	log.Printf("📋 Найдено компаний: %d", len(companies))

	// Создаем схемы для всех активных компаний
	successCount := 0
	errorCount := 0

	for _, company := range companies {
		if !company.IsActive {
			log.Printf("⏭️ Пропускаем деактивированную компанию: %s", company.Name)
			continue
		}

		log.Printf("🏢 Обрабатываем компанию: %s (схема: %s)", company.Name, company.DatabaseSchema)

		// Проверяем, существует ли схема
		var schemaExists bool
		checkQuery := "SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = ?)"
		if err := db.Raw(checkQuery, company.DatabaseSchema).Scan(&schemaExists).Error; err != nil {
			log.Printf("  ❌ Ошибка проверки схемы: %v", err)
			errorCount++
			continue
		}

		if schemaExists {
			log.Printf("  ✅ Схема %s уже существует", company.DatabaseSchema)

			// Проверяем, есть ли в схеме необходимые таблицы
			tenantDB := tenantMiddleware.SwitchToTenantSchema(company.DatabaseSchema)
			if tenantDB == nil {
				log.Printf("  ❌ Не удалось переключиться на схему %s", company.DatabaseSchema)
				errorCount++
				continue
			}

			// Проверяем основные таблицы
			missingTables := []string{}
			requiredTables := map[string]interface{}{
				"roles":          &models.Role{},
				"user_templates": &models.UserTemplate{},
				"permissions":    &models.Permission{},
				"users":          &models.User{},
			}

			for tableName, model := range requiredTables {
				if !tenantDB.Migrator().HasTable(model) {
					missingTables = append(missingTables, tableName)
				}
			}

			if len(missingTables) > 0 {
				log.Printf("  ⚠️ Отсутствуют таблицы: %v", missingTables)
				log.Printf("  🔄 Выполняем миграции для схемы %s...", company.DatabaseSchema)

				// Выполняем миграции для существующей схемы
				if err := database.CreateTenantSchema(company.ID, company.DatabaseSchema); err != nil {
					log.Printf("  ❌ Ошибка миграций для схемы %s: %v", company.DatabaseSchema, err)
					errorCount++
				} else {
					log.Printf("  ✅ Миграции для схемы %s выполнены", company.DatabaseSchema)
					successCount++
				}
			} else {
				log.Printf("  ✅ Все необходимые таблицы присутствуют")
				successCount++
			}
		} else {
			log.Printf("  🔄 Создаем схему %s...", company.DatabaseSchema)

			// Создаем схему и выполняем миграции
			if err := tenantMiddleware.CreateTenantSchema(company.DatabaseSchema); err != nil {
				log.Printf("  ❌ Ошибка создания схемы %s: %v", company.DatabaseSchema, err)
				errorCount++
			} else {
				log.Printf("  ✅ Схема %s создана и настроена", company.DatabaseSchema)
				successCount++
			}
		}
	}

	// Возвращаемся к схеме public
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось вернуться к схеме public: %v", err)
	}

	log.Println("")
	log.Println("📊 Итоговая статистика:")
	log.Printf("  ✅ Успешно обработано компаний: %d", successCount)
	log.Printf("  ❌ Ошибок: %d", errorCount)
	log.Printf("  📋 Всего компаний: %d", len(companies))

	if errorCount > 0 {
		log.Println("")
		log.Println("⚠️ Обнаружены ошибки. Проверьте логи выше.")
		log.Println("💡 Попробуйте запустить снова или проверьте права доступа к БД.")
	} else {
		log.Println("")
		log.Println("🎉 Все tenant схемы настроены успешно!")
		log.Println("✨ Теперь можно запускать приложение без ошибок мультитенантности.")
	}
}
