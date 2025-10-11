package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
)

func main() {
	log.Println("Creating admin user...")

	// Загружаем конфигурацию
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Создаем таблицы если их нет
	if err := database.DB.Exec(`
		CREATE TABLE IF NOT EXISTS local_users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(64) UNIQUE NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			company_id VARCHAR(36) NOT NULL,
			role VARCHAR(32) NOT NULL DEFAULT 'user',
			email VARCHAR(255),
			name VARCHAR(255),
			is_active BOOLEAN DEFAULT true,
			last_login TIMESTAMP,
			login_count INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP
		);
	`).Error; err != nil {
		log.Printf("Warning: %v", err)
	}

	if err := database.DB.Exec(`
		CREATE TABLE IF NOT EXISTS refresh_tokens (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL,
			token VARCHAR(255) UNIQUE NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			is_revoked BOOLEAN DEFAULT false,
			FOREIGN KEY (user_id) REFERENCES local_users(id)
		);
	`).Error; err != nil {
		log.Printf("Warning: %v", err)
	}

	// Создаем админа
	user := models.LocalUser{
		Username:  "admin",
		Email:     "admin@axenta.local",
		Name:      "Администратор",
		CompanyID: "4e12b3c9-529c-4fe7-98e1-025eed8cb258",
		Role:      "admin",
		IsActive:  true,
	}

	// Хешируем пароль
	if err := user.SetPassword("admin123"); err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	// Проверяем, существует ли пользователь
	var existingUser models.LocalUser
	if err := database.DB.Where("username = ?", user.Username).First(&existingUser).Error; err == nil {
		log.Printf("User %s already exists", user.Username)
		return
	}

	// Создаем пользователя
	if err := database.DB.Create(&user).Error; err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}

	fmt.Printf("✅ Admin user created successfully!\n")
	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("Password: admin123\n")
	fmt.Printf("Role: %s\n", user.Role)
	fmt.Printf("Company ID: %s\n", user.CompanyID)
}
