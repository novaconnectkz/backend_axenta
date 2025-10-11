package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
)

// Скрипт для создания тестовых пользователей для локальной авторизации
func main() {
	log.Println("Creating test users for local authentication...")

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	cfg.LogConfig()

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Выполняем миграции
	if err := database.DB.AutoMigrate(
		&models.LocalUser{},
		&models.RefreshToken{},
	); err != nil {
		log.Fatalf("Failed to migrate models: %v", err)
	}

	// Тестовая компания ID (используем UUID из демо данных)
	testCompanyID := "4e12b3c9-529c-4fe7-98e1-025eed8cb258"

	// Создаем тестовых пользователей
	testUsers := []struct {
		Username string
		Password string
		Email    string
		Name     string
		Role     string
	}{
		{
			Username: "admin",
			Password: "admin123",
			Email:    "admin@axenta.local",
			Name:     "Администратор",
			Role:     models.RoleAdmin,
		},
		{
			Username: "manager",
			Password: "manager123",
			Email:    "manager@axenta.local",
			Name:     "Менеджер",
			Role:     models.RoleManager,
		},
		{
			Username: "tech",
			Password: "tech123",
			Email:    "tech@axenta.local",
			Name:     "Техник",
			Role:     models.RoleTech,
		},
		{
			Username: "accountant",
			Password: "accountant123",
			Email:    "accountant@axenta.local",
			Name:     "Бухгалтер",
			Role:     models.RoleAccountant,
		},
		{
			Username: "user",
			Password: "user123",
			Email:    "user@axenta.local",
			Name:     "Обычный пользователь",
			Role:     models.RoleUser,
		},
	}

	for _, userData := range testUsers {
		// Проверяем, существует ли пользователь
		var existingUser models.LocalUser
		if err := database.DB.Where("username = ?", userData.Username).First(&existingUser).Error; err == nil {
			log.Printf("User %s already exists, skipping...", userData.Username)
			continue
		}

		// Создаем нового пользователя
		user := models.LocalUser{
			Username:  userData.Username,
			Email:     userData.Email,
			Name:      userData.Name,
			CompanyID: testCompanyID,
			Role:      userData.Role,
			IsActive:  true,
		}

		// Хешируем пароль
		if err := user.SetPassword(userData.Password); err != nil {
			log.Printf("Failed to hash password for user %s: %v", userData.Username, err)
			continue
		}

		// Сохраняем в БД
		if err := database.DB.Create(&user).Error; err != nil {
			log.Printf("Failed to create user %s: %v", userData.Username, err)
			continue
		}

		log.Printf("✅ Created user: %s (ID: %d, Role: %s)", user.Username, user.ID, user.Role)
	}

	log.Println("🎉 Test users creation completed!")
	log.Println("")
	log.Println("📋 Test credentials:")
	log.Println("   Admin:      admin / admin123")
	log.Println("   Manager:    manager / manager123")
	log.Println("   Tech:       tech / tech123")
	log.Println("   Accountant: accountant / accountant123")
	log.Println("   User:       user / user123")
	log.Println("")
	log.Println("🔗 Test with:")
	log.Println("   curl -X POST http://localhost:8080/api/local/login \\")
	log.Println("     -H \"Content-Type: application/json\" \\")
	log.Println("     -d '{\"username\": \"admin\", \"password\": \"admin123\"}'")
}

// Функция для запуска из другого пакета
func CreateTestUsers() error {
	// Проверяем подключение к БД
	if database.DB == nil {
		return fmt.Errorf("database connection not initialized")
	}

	// Выполняем миграции
	if err := database.DB.AutoMigrate(
		&models.LocalUser{},
		&models.RefreshToken{},
	); err != nil {
		return fmt.Errorf("failed to migrate models: %w", err)
	}

	testCompanyID := "4e12b3c9-529c-4fe7-98e1-025eed8cb258"

	testUsers := []struct {
		Username string
		Password string
		Email    string
		Name     string
		Role     string
	}{
		{"admin", "admin123", "admin@axenta.local", "Администратор", models.RoleAdmin},
		{"manager", "manager123", "manager@axenta.local", "Менеджер", models.RoleManager},
		{"tech", "tech123", "tech@axenta.local", "Техник", models.RoleTech},
		{"accountant", "accountant123", "accountant@axenta.local", "Бухгалтер", models.RoleAccountant},
		{"user", "user123", "user@axenta.local", "Обычный пользователь", models.RoleUser},
	}

	for _, userData := range testUsers {
		var existingUser models.LocalUser
		if err := database.DB.Where("username = ?", userData.Username).First(&existingUser).Error; err == nil {
			continue // Пользователь уже существует
		}

		user := models.LocalUser{
			Username:  userData.Username,
			Email:     userData.Email,
			Name:      userData.Name,
			CompanyID: testCompanyID,
			Role:      userData.Role,
			IsActive:  true,
		}

		if err := user.SetPassword(userData.Password); err != nil {
			return fmt.Errorf("failed to hash password for user %s: %w", userData.Username, err)
		}

		if err := database.DB.Create(&user).Error; err != nil {
			return fmt.Errorf("failed to create user %s: %w", userData.Username, err)
		}

		log.Printf("✅ Created user: %s (Role: %s)", user.Username, user.Role)
	}

	return nil
}
