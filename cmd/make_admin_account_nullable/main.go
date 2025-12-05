package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"strings"
)

func main() {
	// Загружаем конфиг
	config.LoadConfig()

	// Инициализируем БД
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}

	fmt.Printf("🔧 Делаем admin_account_id nullable в таблице axenta_object_snapshots\n")
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		log.Fatalf("Ошибка получения компаний: %v", err)
	}

	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		fmt.Printf("🏢 Компания: %s (схема: %s)\n", company.Name, company.DatabaseSchema)

		// Делаем колонку admin_account_id nullable
		sql := fmt.Sprintf(`
			ALTER TABLE %s.axenta_object_snapshots 
			ALTER COLUMN admin_account_id DROP NOT NULL
		`, company.DatabaseSchema)

		if err := tenantDB.Exec(sql).Error; err != nil {
			fmt.Printf("   ⚠️ Ошибка изменения колонки: %v\n", err)
			// Возможно, колонка уже nullable, продолжаем
		} else {
			fmt.Printf("   ✅ Колонка admin_account_id теперь nullable\n")
		}

		fmt.Printf("\n")
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("✅ Изменение структуры таблицы завершено\n")
	fmt.Printf(strings.Repeat("=", 100) + "\n")
}

