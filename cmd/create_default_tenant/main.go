package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"log"
)

func main() {
	log.Println("🏢 Создание схемы tenant_default")
	log.Println("=================================")

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

	// Убеждаемся, что мы в схеме public
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на схему public: %v", err)
	}

	// Создаем схему tenant_default
	schemaName := "tenant_default"
	log.Printf("🔄 Создаем схему %s...", schemaName)

	if err := db.Exec("CREATE SCHEMA IF NOT EXISTS " + schemaName).Error; err != nil {
		log.Fatalf("❌ Не удалось создать схему %s: %v", schemaName, err)
	}

	log.Printf("✅ Схема %s создана", schemaName)

	// Переключаемся на новую схему
	if err := db.Exec("SET search_path TO " + schemaName).Error; err != nil {
		log.Fatalf("❌ Не удалось переключиться на схему %s: %v", schemaName, err)
	}

	log.Printf("✅ Переключились на схему %s", schemaName)

	// Список тенантных моделей для миграции
	tenantModels := []interface{}{
		// Пользователи и роли
		&models.Permission{},
		&models.Role{},
		&models.User{},
		&models.UserTemplate{},

		// Объекты и шаблоны
		&models.ObjectTemplate{},
		&models.Object{},

		// Договоры
		&models.Contract{},
		&models.ContractAppendix{},

		// Локации и монтажники
		&models.Location{},
		&models.Installer{},
		&models.Installation{},

		// Оборудование
		&models.Equipment{},

		// Тарифы (тенантные версии)
		&models.TariffPlan{},
	}

	log.Printf("🔄 Выполняем миграции для %d моделей...", len(tenantModels))

	successCount := 0
	errorCount := 0

	for _, model := range tenantModels {
		modelName := getModelName(model)
		log.Printf("  🔄 Миграция %s...", modelName)

		if err := db.AutoMigrate(model); err != nil {
			log.Printf("  ❌ Ошибка миграции %s: %v", modelName, err)
			errorCount++
		} else {
			log.Printf("  ✅ %s мигрирован успешно", modelName)
			successCount++
		}
	}

	// Возвращаемся к схеме public
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось вернуться к схеме public: %v", err)
	}

	log.Println("")
	log.Println("📊 Результаты миграции:")
	log.Printf("  ✅ Успешно: %d", successCount)
	log.Printf("  ❌ Ошибок: %d", errorCount)
	log.Printf("  📋 Всего: %d", len(tenantModels))

	// Проверяем созданные таблицы
	log.Println("")
	log.Println("🔍 Проверка созданных таблиц в схеме tenant_default:")

	// Переключаемся обратно на tenant_default для проверки
	if err := db.Exec("SET search_path TO " + schemaName).Error; err != nil {
		log.Printf("❌ Не удалось переключиться на схему %s для проверки: %v", schemaName, err)
	} else {
		criticalTables := map[string]interface{}{
			"roles":          &models.Role{},
			"user_templates": &models.UserTemplate{},
			"permissions":    &models.Permission{},
			"users":          &models.User{},
		}

		for tableName, model := range criticalTables {
			if db.Migrator().HasTable(model) {
				log.Printf("  ✅ %s - существует", tableName)
			} else {
				log.Printf("  ❌ %s - отсутствует", tableName)
			}
		}
	}

	// Возвращаемся к public
	db.Exec("SET search_path TO public")

	if errorCount > 0 {
		log.Println("")
		log.Println("⚠️ Обнаружены ошибки, но критические таблицы должны быть созданы.")
	} else {
		log.Println("")
		log.Println("🎉 Схема tenant_default настроена успешно!")
	}

	log.Println("")
	log.Println("💡 Теперь эндпоинты /api/auth/roles и /api/auth/user-templates должны работать!")
}

// getModelName возвращает имя модели для логирования
func getModelName(model interface{}) string {
	switch model.(type) {
	case *models.Permission:
		return "Permission"
	case *models.Role:
		return "Role"
	case *models.User:
		return "User"
	case *models.UserTemplate:
		return "UserTemplate"
	case *models.ObjectTemplate:
		return "ObjectTemplate"
	case *models.Object:
		return "Object"
	case *models.Contract:
		return "Contract"
	case *models.ContractAppendix:
		return "ContractAppendix"
	case *models.Location:
		return "Location"
	case *models.Installer:
		return "Installer"
	case *models.Installation:
		return "Installation"
	case *models.Equipment:
		return "Equipment"
	case *models.TariffPlan:
		return "TariffPlan"
	default:
		return "Unknown"
	}
}
