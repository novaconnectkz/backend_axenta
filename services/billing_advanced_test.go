package services

import (
	"context"
	"testing"
	"time"

	"backend_axenta/models"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupVATTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("Failed to connect to test database")
	}

	// Создаем таблицы для тестирования VAT
	err = db.Exec(`
		CREATE TABLE countries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			code TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			display_name TEXT,
			is_active BOOLEAN DEFAULT TRUE
		)
	`).Error
	if err != nil {
		panic("Failed to create countries table: " + err.Error())
	}

	err = db.Exec(`
		CREATE TABLE tax_rates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			country_code TEXT NOT NULL,
			rate DECIMAL(5,2) NOT NULL,
			name TEXT,
			effective_from DATETIME NOT NULL,
			effective_to DATETIME,
			priority INTEGER DEFAULT 0,
			is_active BOOLEAN DEFAULT TRUE
		)
	`).Error
	if err != nil {
		panic("Failed to create tax_rates table: " + err.Error())
	}

	err = db.Exec(`
		CREATE TABLE tax_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			seller_country_code TEXT NOT NULL,
			buyer_country_code TEXT NOT NULL,
			service_type TEXT,
			period_start DATETIME,
			period_end DATETIME,
			apply_tax BOOLEAN DEFAULT TRUE,
			tax_rate_override DECIMAL(5,2),
			rate_source TEXT,
			priority INTEGER DEFAULT 0,
			is_active BOOLEAN DEFAULT TRUE
		)
	`).Error
	if err != nil {
		panic("Failed to create tax_rules table: " + err.Error())
	}

	return db
}

func TestVATResolver_ResolveVAT(t *testing.T) {
	db := setupVATTestDB()
	resolver := &VATResolver{db: db}
	ctx := context.Background()
	testPeriod := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	// Создаем страны
	countries := []models.Country{
		{Code: "RU", Name: "Россия", DisplayName: "Российская Федерация", IsActive: true},
		{Code: "KZ", Name: "Казахстан", DisplayName: "Республика Казахстан", IsActive: true},
		{Code: "BY", Name: "Беларусь", DisplayName: "Республика Беларусь", IsActive: true},
	}
	for _, country := range countries {
		db.Create(&country)
	}

	// Создаем ставки НДС
	taxRates := []models.TaxRate{
		{
			CountryCode:   "RU",
			Rate:          decimal.NewFromFloat(20.0),
			Name:          "НДС 20%",
			EffectiveFrom: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Priority:      1,
			IsActive:      true,
		},
		{
			CountryCode:   "KZ",
			Rate:          decimal.NewFromFloat(12.0),
			Name:          "НДС 12%",
			EffectiveFrom: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Priority:      1,
			IsActive:      true,
		},
		{
			CountryCode:   "BY",
			Rate:          decimal.NewFromFloat(20.0),
			Name:          "НДС 20%",
			EffectiveFrom: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Priority:      1,
			IsActive:      true,
		},
	}
	for _, rate := range taxRates {
		db.Create(&rate)
	}

	t.Run("Базовый случай - RU to RU", func(t *testing.T) {
		rate, source, err := resolver.ResolveVAT(ctx, "RU", "RU", "", testPeriod)
		require.NoError(t, err)
		assert.Equal(t, decimal.NewFromFloat(20.0), rate)
		assert.Equal(t, "default_rate", source)
	})

	t.Run("RU to KZ - без правила, используем ставку продавца", func(t *testing.T) {
		rate, source, err := resolver.ResolveVAT(ctx, "RU", "KZ", "", testPeriod)
		require.NoError(t, err)
		assert.Equal(t, decimal.NewFromFloat(20.0), rate) // Ставка продавца (RU)
		assert.Equal(t, "default_rate", source)
	})

	t.Run("RU to KZ - с правилом применения НДС", func(t *testing.T) {
		// Создаем правило: для RU->KZ применяется НДС 20%
		taxRule := models.TaxRule{
			SellerCountryCode: "RU",
			BuyerCountryCode:  "KZ",
			ServiceType:       "",
			PeriodStart:       &testPeriod,
			ApplyTax:          true,
			Priority:          1,
			IsActive:          true,
		}
		db.Create(&taxRule)

		rate, source, err := resolver.ResolveVAT(ctx, "RU", "KZ", "", testPeriod)
		require.NoError(t, err)
		assert.Equal(t, decimal.NewFromFloat(20.0), rate) // Из tax_rate для RU
		assert.Equal(t, "rule_default", source)
	})

	t.Run("RU to KZ - с переопределением ставки", func(t *testing.T) {
		overrideRate := decimal.NewFromFloat(15.0)
		taxRule := models.TaxRule{
			SellerCountryCode: "RU",
			BuyerCountryCode:  "KZ",
			ServiceType:       "software",
			PeriodStart:       &testPeriod,
			ApplyTax:          true,
			TaxRateOverride:   &overrideRate,
			Priority:          2, // Выше приоритет
			IsActive:          true,
		}
		db.Create(&taxRule)

		rate, source, err := resolver.ResolveVAT(ctx, "RU", "KZ", "software", testPeriod)
		require.NoError(t, err)
		assert.Equal(t, decimal.NewFromFloat(15.0), rate) // Переопределенная ставка
		assert.Equal(t, "rule_override", source)
	})

	t.Run("Правило с истекшим периодом", func(t *testing.T) {
		expiredEnd := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)
		taxRule := models.TaxRule{
			SellerCountryCode: "RU",
			BuyerCountryCode:  "BY",
			PeriodStart:       &testPeriod,
			PeriodEnd:         &expiredEnd, // Истекший период
			ApplyTax:          true,
			Priority:          1,
			IsActive:          true,
		}
		db.Create(&taxRule)

		// Правило не должно применяться, используем базовую ставку
		rate, source, err := resolver.ResolveVAT(ctx, "RU", "BY", "", testPeriod)
		require.NoError(t, err)
		assert.Equal(t, decimal.NewFromFloat(20.0), rate)
		assert.Equal(t, "default_rate", source)
	})

	t.Run("Страна без ставки НДС", func(t *testing.T) {
		rate, source, err := resolver.ResolveVAT(ctx, "US", "RU", "", testPeriod)
		assert.Error(t, err)
		assert.Equal(t, decimal.Zero, rate)
		assert.Equal(t, "error", source)
		assert.Contains(t, err.Error(), "не найдена ставка НДС")
	})
}

func TestVATResolver_GetActiveTaxRate(t *testing.T) {
	db := setupVATTestDB()
	resolver := &VATResolver{db: db}
	ctx := context.Background()

	// Создаем страну и ставку
	country := models.Country{Code: "RU", Name: "Россия", IsActive: true}
	db.Create(&country)

	taxRate := models.TaxRate{
		CountryCode:   "RU",
		Rate:          decimal.NewFromFloat(20.0),
		Name:          "НДС 20%",
		EffectiveFrom: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		Priority:      1,
		IsActive:      true,
	}
	db.Create(&taxRate)

	rate, err := resolver.GetActiveTaxRate(ctx, "RU")
	require.NoError(t, err)
	assert.Equal(t, decimal.NewFromFloat(20.0), rate)
}
