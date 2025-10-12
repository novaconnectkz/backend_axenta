//go:build ignore
// +build ignore

package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"fmt"
	"log"
)

func main() {
	fmt.Println("🔍 Проверка ролей в базе данных")
	fmt.Println("================================")

	// Загружаем конфигурацию
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	db := database.GetDB()
	if db == nil {
		log.Fatalf("Database connection is nil")
	}

	// Проверяем существующие роли
	var roles []models.Role
	if err := db.Find(&roles).Error; err != nil {
		log.Fatalf("Failed to get roles: %v", err)
	}

	fmt.Printf("📊 Найдено ролей в базе данных: %d\n", len(roles))

	if len(roles) == 0 {
		fmt.Println("❌ Роли не найдены! Создаем роли по умолчанию...")

		// Создаем роли через сервис
		axentaUserService := services.NewAxentaUserService(db)
		if err := axentaUserService.EnsureDefaultRoles(); err != nil {
			log.Fatalf("Failed to create default roles: %v", err)
		}

		// Проверяем снова
		if err := db.Find(&roles).Error; err != nil {
			log.Fatalf("Failed to get roles after creation: %v", err)
		}

		fmt.Printf("✅ Создано ролей: %d\n", len(roles))
	}

	// Выводим информацию о ролях
	fmt.Println("\n📋 Роли в системе:")
	for _, role := range roles {
		fmt.Printf("  ID: %d | Name: %s | Display: %s | Color: %s | System: %t\n",
			role.ID, role.Name, role.DisplayName, role.Color, role.IsSystem)
	}

	// Тестируем маппинг ролей
	fmt.Println("\n🔄 Тестирование маппинга ролей:")
	testAccountTypes := []string{"partner", "client", "admin", "unknown", ""}

	for _, accountType := range testAccountTypes {
		var roleName string
		switch accountType {
		case "partner":
			roleName = "partner"
		case "client":
			roleName = "client"
		default:
			roleName = "user"
		}

		var role models.Role
		err := db.Where("name = ?", roleName).First(&role).Error
		if err != nil {
			fmt.Printf("  ❌ accountType: '%s' → role: '%s' (НЕ НАЙДЕНА)\n", accountType, roleName)
		} else {
			fmt.Printf("  ✅ accountType: '%s' → role: '%s' (ID: %d, Display: %s)\n",
				accountType, roleName, role.ID, role.DisplayName)
		}
	}

	fmt.Println("\n🎉 Проверка завершена!")
}
