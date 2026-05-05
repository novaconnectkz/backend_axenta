package main

import (
	"log"
	"time"

	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/shopspring/decimal"
)

func main() {
	log.Println("🌱 Запуск заполнения базы данных seed данными для биллинга")

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("🔧 Подключение к базе данных: %s@%s:%s/%s",
		cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

	// Создаем базу данных если её нет
	if err := database.CreateDatabaseIfNotExists(); err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabaseWithoutMigrations(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("✅ Подключение к базе данных установлено")

	db := database.GetDB()

	// Создаем страны
	log.Println("📋 Создание стран...")
	countries := []models.Country{
		{
			Code:        "RU",
			Name:        "Russia",
			DisplayName: "Российская Федерация",
			IsActive:    true,
		},
		{
			Code:        "KZ",
			Name:        "Kazakhstan",
			DisplayName: "Республика Казахстан",
			IsActive:    true,
		},
		{
			Code:        "BY",
			Name:        "Belarus",
			DisplayName: "Республика Беларусь",
			IsActive:    true,
		},
		{
			Code:        "UA",
			Name:        "Ukraine",
			DisplayName: "Украина",
			IsActive:    false,
		},
	}

	for _, country := range countries {
		var exists models.Country
		if err := db.Where("code = ?", country.Code).First(&exists).Error; err != nil {
			if err.Error() == "record not found" {
				if err := db.Create(&country).Error; err != nil {
					log.Printf("⚠️ Ошибка создания страны %s: %v", country.Code, err)
				} else {
					log.Printf("✅ Создана страна: %s (%s)", country.Code, country.DisplayName)
				}
			}
		} else {
			log.Printf("⏭️ Страна %s уже существует", country.Code)
		}
	}

	// Создаем ставки НДС
	log.Println("📋 Создание ставок НДС...")
	taxRates := []models.TaxRate{
		{
			CountryCode:   "RU",
			Rate:          decimal.NewFromInt(20),
			Name:          "НДС 20%",
			EffectiveFrom: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
			Priority:      1,
			IsActive:      true,
		},
		{
			CountryCode:   "RU",
			Rate:          decimal.NewFromInt(10),
			Name:          "НДС 10% (льготная)",
			EffectiveFrom: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
			Priority:      0,
			IsActive:      true,
		},
		{
			CountryCode:   "KZ",
			Rate:          decimal.NewFromInt(12),
			Name:          "НДС 12%",
			EffectiveFrom: time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC),
			Priority:      1,
			IsActive:      true,
		},
		{
			CountryCode:   "BY",
			Rate:          decimal.NewFromInt(20),
			Name:          "НДС 20%",
			EffectiveFrom: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			Priority:      1,
			IsActive:      true,
		},
	}

	for _, taxRate := range taxRates {
		var exists models.TaxRate
		if err := db.Where("country_code = ? AND rate = ? AND name = ?", taxRate.CountryCode, taxRate.Rate, taxRate.Name).First(&exists).Error; err != nil {
			if err.Error() == "record not found" {
				if err := db.Create(&taxRate).Error; err != nil {
					log.Printf("⚠️ Ошибка создания ставки НДС для %s: %v", taxRate.CountryCode, err)
				} else {
					log.Printf("✅ Создана ставка НДС: %s - %s", taxRate.CountryCode, taxRate.Name)
				}
			}
		} else {
			log.Printf("⏭️ Ставка НДС %s (%s) уже существует", taxRate.CountryCode, taxRate.Name)
		}
	}

	// Создаем правила НДС между странами
	log.Println("📋 Создание правил НДС между странами...")
	taxRules := []models.TaxRule{
		{
			SellerCountryCode: "RU",
			BuyerCountryCode:  "RU",
			ServiceType:       "monitoring",
			PeriodStart:       &[]time.Time{time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}[0],
			ApplyTax:          true,
			RateSource:        "default",
			Priority:          1,
			IsActive:          true,
		},
		{
			SellerCountryCode: "RU",
			BuyerCountryCode:  "KZ",
			ServiceType:       "monitoring",
			PeriodStart:       &[]time.Time{time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}[0],
			ApplyTax:          true,
			RateSource:        "default",
			Priority:          1,
			IsActive:          true,
		},
		{
			SellerCountryCode: "RU",
			BuyerCountryCode:  "BY",
			ServiceType:       "monitoring",
			PeriodStart:       &[]time.Time{time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}[0],
			ApplyTax:          true,
			RateSource:        "default",
			Priority:          1,
			IsActive:          true,
		},
		{
			SellerCountryCode: "KZ",
			BuyerCountryCode:  "KZ",
			ServiceType:       "monitoring",
			PeriodStart:       &[]time.Time{time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}[0],
			ApplyTax:          true,
			RateSource:        "default",
			Priority:          1,
			IsActive:          true,
		},
	}

	for _, taxRule := range taxRules {
		var exists models.TaxRule
		if err := db.Where("seller_country_code = ? AND buyer_country_code = ? AND service_type = ?",
			taxRule.SellerCountryCode, taxRule.BuyerCountryCode, taxRule.ServiceType).First(&exists).Error; err != nil {
			if err.Error() == "record not found" {
				if err := db.Create(&taxRule).Error; err != nil {
					log.Printf("⚠️ Ошибка создания правила НДС %s->%s: %v", taxRule.SellerCountryCode, taxRule.BuyerCountryCode, err)
				} else {
					log.Printf("✅ Создано правило НДС: %s -> %s", taxRule.SellerCountryCode, taxRule.BuyerCountryCode)
				}
			}
		} else {
			log.Printf("⏭️ Правило НДС %s->%s уже существует", taxRule.SellerCountryCode, taxRule.BuyerCountryCode)
		}
	}

	log.Println("")
	log.Println("🎉 Seed данные успешно созданы!")

	// Выводим статистику
	var countryCount, taxRateCount, taxRuleCount int64
	db.Model(&models.Country{}).Count(&countryCount)
	db.Model(&models.TaxRate{}).Count(&taxRateCount)
	db.Model(&models.TaxRule{}).Count(&taxRuleCount)

	log.Println("📊 Статистика:")
	log.Printf("   Страны: %d", countryCount)
	log.Printf("   Ставки НДС: %d", taxRateCount)
	log.Printf("   Правила НДС: %d", taxRuleCount)
}
