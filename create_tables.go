//go:build ignore
// +build ignore

package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
)

// Скрипт для создания таблиц в базе данных
func main() {
	fmt.Println("🔧 Создание таблиц в базе данных")
	fmt.Println("=================================")

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

	// Выполняем миграции для всех моделей
	models := []interface{}{
		&models.Company{},
		&models.User{},
		&models.Object{},
		&models.Contract{},
		&models.Invoice{},
		&models.Report{},
		&models.Integration{},
		&models.IntegrationError{},
		&models.LocalUser{},
		&models.Role{},
	}

	fmt.Printf("📊 Создание %d таблиц...\n", len(models))

	for _, model := range models {
		if err := db.AutoMigrate(model); err != nil {
			log.Printf("❌ Ошибка создания таблицы для модели %T: %v", model, err)
		} else {
			fmt.Printf("✅ Таблица для модели %T создана успешно\n", model)
		}
	}

	fmt.Println("\n🎉 Создание таблиц завершено!")
}
