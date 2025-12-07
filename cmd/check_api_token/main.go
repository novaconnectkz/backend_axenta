package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"strings"
)

const apiTokenFromRules = "5e515a8f2874fc78f31c74af45260333f2c84c35" // Токен из .cursorrules

func main() {
	log.Println("🔍 Проверка токена API в БД...")

	// Загружаем конфигурацию
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Не удалось загрузить конфигурацию: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Не удалось подключиться к базе данных: %v", err)
	}
	defer func() {
		sqlDB, _ := database.DB.DB()
		sqlDB.Close()
	}()

	// Получаем первую активную компанию
	var firstCompany models.Company
	if err := database.DB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		log.Fatalf("❌ Не найдено активных компаний: %v", err)
	}

	fmt.Printf("🏢 Компания: %s (ID=%d, схема: %s)\n\n", firstCompany.Name, firstCompany.ID, firstCompany.DatabaseSchema)

	// Получаем tenant DB
	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		log.Fatalf("❌ Не удалось получить tenant DB для компании %d", firstCompany.ID)
	}

	// Проверяем настройки снимков
	var snapshotSettings models.SnapshotSettings
	if err := tenantDB.Where("company_id = ? AND is_active = ?", 1, true).First(&snapshotSettings).Error; err != nil {
		fmt.Printf("⚠️ Настройки снимков не найдены: %v\n", err)
		fmt.Printf("💡 Нужно создать настройки с токеном из .cursorrules\n\n")
	} else {
		fmt.Printf("✅ Настройки снимков найдены:\n")
		fmt.Printf("   ID: %d\n", snapshotSettings.ID)
		fmt.Printf("   Company ID: %d\n", snapshotSettings.CompanyID)
		fmt.Printf("   Is Active: %v\n", snapshotSettings.IsActive)
		fmt.Printf("   Токен в БД: %s...%s (длина: %d)\n",
			snapshotSettings.AxentaToken[:min(10, len(snapshotSettings.AxentaToken))],
			snapshotSettings.AxentaToken[max(0, len(snapshotSettings.AxentaToken)-10):],
			len(snapshotSettings.AxentaToken))
		fmt.Printf("   Токен из .cursorrules: %s...%s (длина: %d)\n\n",
			apiTokenFromRules[:10],
			apiTokenFromRules[len(apiTokenFromRules)-10:],
			len(apiTokenFromRules))

		if snapshotSettings.AxentaToken == apiTokenFromRules {
			fmt.Println("✅ Токены совпадают!")
		} else {
			fmt.Println("⚠️ Токены НЕ совпадают!")
			fmt.Println("💡 Обновить токен в БД на токен из .cursorrules?")
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
