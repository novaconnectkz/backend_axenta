package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
)

func main() {
	log.Println("Creating glomos user...")

	// Загружаем конфигурацию
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Создаем пользователя glomos
	user := models.LocalUser{
		Username:  "glomos",
		Email:     "glomos@axenta.cloud",
		Name:      "Glomos User",
		CompanyID: "partner-company-id", // Партнерский аккаунт
		Role:      "admin",
		IsActive:  true,
	}

	// Хешируем пароль A51ewweB
	if err := user.SetPassword("A51ewweB"); err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	// Проверяем, существует ли пользователь
	var existingUser models.LocalUser
	if err := database.DB.Where("username = ?", user.Username).First(&existingUser).Error; err == nil {
		log.Printf("User %s already exists, updating password...", user.Username)

		// Обновляем пароль существующего пользователя
		if err := existingUser.SetPassword("A51ewweB"); err != nil {
			log.Fatalf("Failed to hash password: %v", err)
		}

		if err := database.DB.Save(&existingUser).Error; err != nil {
			log.Fatalf("Failed to update user: %v", err)
		}

		fmt.Printf("✅ User %s password updated!\n", user.Username)
		return
	}

	// Создаем пользователя
	if err := database.DB.Create(&user).Error; err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}

	fmt.Printf("✅ Glomos user created successfully!\n")
	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("Password: A51ewweB\n")
	fmt.Printf("Role: %s\n", user.Role)
	fmt.Printf("Company ID: %s\n", user.CompanyID)
}
