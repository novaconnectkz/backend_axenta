package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"log"
	"time"
)

func main() {
	log.Println("🔍 Поиск токенов во всех компаниях...")

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

	// Проверяем токены в каждой компании
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			log.Printf("⚠️ Компания %s (ID=%d): не удалось получить DB\n", company.Name, company.ID)
			continue
		}

		var tokens []models.UserToken
		if err := tenantDB.
			Where("is_active = ? AND expires_at > ?", true, time.Now()).
			Order("updated_at DESC").
			Find(&tokens).Error; err != nil {
			log.Printf("❌ Компания %s (ID=%d): ошибка поиска токенов: %v\n", company.Name, company.ID, err)
			continue
		}

		if len(tokens) > 0 {
			log.Printf("✅ Компания %s (ID=%d, схема: %s): найдено %d активных токенов\n", 
				company.Name, company.ID, company.DatabaseSchema, len(tokens))
			for i, token := range tokens {
				log.Printf("   Токен %d: account_id=%d, username=%s, expires_at=%v\n", 
					i+1, token.AccountID, token.Username, token.ExpiresAt)
			}
		} else {
			log.Printf("⚠️ Компания %s (ID=%d): активных токенов не найдено\n", company.Name, company.ID)
		}
	}
}

