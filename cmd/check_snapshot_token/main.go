package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/gorm"
)

func main() {
	log.Println("🔍 Проверка токена для создания снимков...")

	// Загружаем конфиг
	config.LoadConfig()

	// Инициализируем БД
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Table("public.companies").Where("is_active = ?", true).Find(&companies).Error; err != nil {
		log.Fatalf("Ошибка загрузки компаний: %v", err)
	}

	log.Printf("📋 Найдено %d активных компаний\n", len(companies))

	// Проверяем токен для каждой компании
	for _, company := range companies {
		fmt.Printf("\n🏢 Компания: %s (ID=%d, схема: %s)\n", company.Name, company.ID, company.DatabaseSchema)
		
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			fmt.Printf("  ❌ Не удалось получить tenant DB\n")
			continue
		}

		token, source := getTokenForCompany(company.ID, tenantDB)
		if token != "" {
			tokenPreview := token
			if len(tokenPreview) > 20 {
				tokenPreview = tokenPreview[:20] + "..."
			}
			fmt.Printf("  ✅ Токен найден из источника: %s\n", source)
			fmt.Printf("  🔑 Токен (первые 20 символов): %s\n", tokenPreview)
			fmt.Printf("  📏 Длина токена: %d символов\n", len(token))
		} else {
			fmt.Printf("  ❌ Токен не найден\n")
		}
	}
}

func getTokenForCompany(companyID uint, db *gorm.DB) (string, string) {
	fmt.Printf("  🔍 Проверяем источники токена...\n")
	
	// ПРИОРИТЕТ 1: Пробуем токен из настроек снимков (для суперадмина, ID=1)
	fmt.Printf("    1️⃣ Проверяем настройки снимков (snapshot_settings)...\n")
	const superAdminCompanyID = 1
	superAdminTenantDB := database.GetTenantDBByID(superAdminCompanyID)
	if superAdminTenantDB != nil {
		var snapshotSettings models.SnapshotSettings
		if err := superAdminTenantDB.
			Where("company_id = ? AND is_active = ?", superAdminCompanyID, true).
			First(&snapshotSettings).Error; err == nil {
			if snapshotSettings.AxentaToken != "" {
				fmt.Printf("       ✅ Токен найден в настройках снимков!\n")
				return snapshotSettings.AxentaToken, "настройки снимков (snapshot_settings)"
			} else {
				fmt.Printf("       ⚠️ Настройки найдены, но токен пустой\n")
			}
		} else {
			fmt.Printf("       ❌ Настройки не найдены: %v\n", err)
		}
	} else {
		fmt.Printf("       ❌ Не удалось получить tenant DB для суперадмина\n")
	}

	// ПРИОРИТЕТ 2: Пробуем системный токен из env
	fmt.Printf("    2️⃣ Проверяем переменную окружения AXENTA_ADMIN_TOKEN...\n")
	systemToken := os.Getenv("AXENTA_ADMIN_TOKEN")
	if systemToken != "" {
		fmt.Printf("       ✅ Токен найден в переменной окружения!\n")
		return systemToken, "переменная окружения AXENTA_ADMIN_TOKEN"
	} else {
		fmt.Printf("       ❌ Переменная окружения не установлена\n")
	}

	// ПРИОРИТЕТ 3: Берем любой активный токен из БД текущего тенанта
	fmt.Printf("    3️⃣ Проверяем user_tokens в схеме компании...\n")
	var token models.UserToken
	if err := db.
		Where("is_active = ? AND expires_at > ?", true, time.Now()).
		Order("updated_at DESC").
		First(&token).Error; err == nil {
		fmt.Printf("       ✅ Токен найден в user_tokens!\n")
		return token.Token, fmt.Sprintf("user_tokens (account_id=%d, username=%s)", token.AccountID, token.Username)
	} else {
		fmt.Printf("       ❌ Активных токенов не найдено: %v\n", err)
	}

	return "", "не найден"
}

