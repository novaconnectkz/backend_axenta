package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupDiscountTestDB создает тестовую базу данных для discount service
func setupDiscountTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Discount{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// TestNewDiscountService тестирует создание нового сервиса
func TestNewDiscountService(t *testing.T) {
	setupDiscountTestDB(t)

	service := NewDiscountService()
	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
}

// TestDiscountService_GetActiveDiscounts_NoDiscounts тестирует GetActiveDiscounts без скидок
func TestDiscountService_GetActiveDiscounts_NoDiscounts(t *testing.T) {
	setupDiscountTestDB(t)
	service := NewDiscountService()

	ctx := context.Background()
	discounts, err := service.GetActiveDiscounts(ctx, "object", 1, time.Now())
	require.NoError(t, err)
	assert.Empty(t, discounts)
}

// TestDiscountService_GetActiveDiscounts_ActiveDiscount тестирует GetActiveDiscounts с активной скидкой
func TestDiscountService_GetActiveDiscounts_ActiveDiscount(t *testing.T) {
	db := setupDiscountTestDB(t)
	service := NewDiscountService()

	// Создаем активную скидку
	discount := models.Discount{
		Level:     "object",
		EntityID:  1,
		Type:      "percent",
		Value:     decimal.NewFromInt(10),
		IsActive:  true,
		StartDate: time.Now().AddDate(0, -1, 0), // Началась месяц назад
		EndDate:   nil,                          // Без даты окончания
	}
	db.Create(&discount)

	ctx := context.Background()
	discounts, err := service.GetActiveDiscounts(ctx, "object", 1, time.Now())
	require.NoError(t, err)
	assert.Len(t, discounts, 1)
	assert.Equal(t, "object", discounts[0].Level)
	assert.Equal(t, uint(1), discounts[0].EntityID)
}

// TestDiscountService_GetActiveDiscounts_ExpiredDiscount тестирует GetActiveDiscounts с истекшей скидкой
func TestDiscountService_GetActiveDiscounts_ExpiredDiscount(t *testing.T) {
	db := setupDiscountTestDB(t)
	service := NewDiscountService()

	// Создаем истекшую скидку
	endDate := time.Now().AddDate(0, -1, 0) // Истекла месяц назад
	discount := models.Discount{
		Level:     "object",
		EntityID:  1,
		Type:      "percent",
		Value:     decimal.NewFromInt(10),
		IsActive:  true,
		StartDate: time.Now().AddDate(0, -2, 0),
		EndDate:   &endDate,
	}
	db.Create(&discount)

	ctx := context.Background()
	discounts, err := service.GetActiveDiscounts(ctx, "object", 1, time.Now())
	require.NoError(t, err)
	assert.Empty(t, discounts) // Истекшая скидка не должна быть возвращена
}

// TestDiscountService_GetActiveDiscounts_FutureDiscount тестирует GetActiveDiscounts с будущей скидкой
func TestDiscountService_GetActiveDiscounts_FutureDiscount(t *testing.T) {
	db := setupDiscountTestDB(t)
	service := NewDiscountService()

	// Создаем скидку, которая начнется в будущем
	startDate := time.Now().AddDate(0, 1, 0) // Начнется через месяц
	discount := models.Discount{
		Level:     "object",
		EntityID:  1,
		Type:      "percent",
		Value:     decimal.NewFromInt(10),
		IsActive:  true,
		StartDate: startDate,
		EndDate:   nil,
	}
	db.Create(&discount)

	ctx := context.Background()
	discounts, err := service.GetActiveDiscounts(ctx, "object", 1, time.Now())
	require.NoError(t, err)
	assert.Empty(t, discounts) // Будущая скидка не должна быть возвращена
}

// TestDiscountService_FindDiscountsForHierarchy тестирует FindDiscountsForHierarchy
func TestDiscountService_FindDiscountsForHierarchy(t *testing.T) {
	db := setupDiscountTestDB(t)
	service := NewDiscountService()

	// Создаем скидки на разных уровнях
	objectDiscount := models.Discount{
		Level:     "object",
		EntityID:  1,
		Type:      "percent",
		Value:     decimal.NewFromInt(5),
		IsActive:  true,
		StartDate: time.Now().AddDate(0, -1, 0),
		EndDate:   nil,
	}
	contractDiscount := models.Discount{
		Level:     "contract",
		EntityID:  10,
		Type:      "percent",
		Value:     decimal.NewFromInt(10),
		IsActive:  true,
		StartDate: time.Now().AddDate(0, -1, 0),
		EndDate:   nil,
	}
	db.Create(&objectDiscount)
	db.Create(&contractDiscount)

	ctx := context.Background()
	objectID := uint(1)
	contractID := uint(10)
	discounts, err := service.FindDiscountsForHierarchy(ctx, &objectID, nil, nil, nil, &contractID, time.Now())
	require.NoError(t, err)
	assert.Len(t, discounts, 2)
}

// TestDiscountService_ApplyDiscounts_PercentDiscount тестирует ApplyDiscounts с процентной скидкой
func TestDiscountService_ApplyDiscounts_PercentDiscount(t *testing.T) {
	db := setupDiscountTestDB(t)
	service := NewDiscountService()

	// Создаем процентную скидку
	discount := models.Discount{
		Level:     "object",
		EntityID:  1,
		Type:      "percent",
		Value:     decimal.NewFromInt(10), // 10%
		IsActive:  true,
		StartDate: time.Now().AddDate(0, -1, 0),
		EndDate:   nil,
	}
	db.Create(&discount)

	ctx := context.Background()
	originalAmount := decimal.NewFromInt(1000)
	objectID := uint(1)

	finalAmount, applications, err := service.ApplyDiscounts(ctx, originalAmount, &objectID, nil, nil, nil, nil, time.Now())
	require.NoError(t, err)
	assert.Len(t, applications, 1)
	assert.Equal(t, decimal.NewFromInt(900), finalAmount)            // 1000 - 10% = 900
	assert.Equal(t, decimal.NewFromInt(100), applications[0].Amount) // 10% от 1000 = 100
}

// TestDiscountService_ApplyDiscounts_FixedDiscount тестирует ApplyDiscounts с фиксированной скидкой
func TestDiscountService_ApplyDiscounts_FixedDiscount(t *testing.T) {
	db := setupDiscountTestDB(t)
	service := NewDiscountService()

	// Создаем фиксированную скидку
	discount := models.Discount{
		Level:     "object",
		EntityID:  1,
		Type:      "fixed",
		Value:     decimal.NewFromInt(100), // 100 рублей
		IsActive:  true,
		StartDate: time.Now().AddDate(0, -1, 0),
		EndDate:   nil,
	}
	db.Create(&discount)

	ctx := context.Background()
	originalAmount := decimal.NewFromInt(1000)
	objectID := uint(1)

	finalAmount, applications, err := service.ApplyDiscounts(ctx, originalAmount, &objectID, nil, nil, nil, nil, time.Now())
	require.NoError(t, err)
	assert.Len(t, applications, 1)
	assert.Equal(t, decimal.NewFromInt(900), finalAmount) // 1000 - 100 = 900
	assert.Equal(t, decimal.NewFromInt(100), applications[0].Amount)
}

// TestDiscountService_ApplyDiscounts_MultipleDiscounts тестирует ApplyDiscounts с несколькими скидками
func TestDiscountService_ApplyDiscounts_MultipleDiscounts(t *testing.T) {
	db := setupDiscountTestDB(t)
	service := NewDiscountService()

	// Создаем скидки на разных уровнях
	objectDiscount := models.Discount{
		Level:     "object",
		EntityID:  1,
		Type:      "percent",
		Value:     decimal.NewFromInt(5), // 5%
		IsActive:  true,
		StartDate: time.Now().AddDate(0, -1, 0),
		EndDate:   nil,
	}
	contractDiscount := models.Discount{
		Level:     "contract",
		EntityID:  10,
		Type:      "percent",
		Value:     decimal.NewFromInt(10), // 10%
		IsActive:  true,
		StartDate: time.Now().AddDate(0, -1, 0),
		EndDate:   nil,
	}
	db.Create(&objectDiscount)
	db.Create(&contractDiscount)

	ctx := context.Background()
	originalAmount := decimal.NewFromInt(1000)
	objectID := uint(1)
	contractID := uint(10)

	finalAmount, applications, err := service.ApplyDiscounts(ctx, originalAmount, &objectID, nil, nil, nil, &contractID, time.Now())
	require.NoError(t, err)
	assert.Len(t, applications, 2)

	// Сначала применяется скидка на объект (5%), затем на договор (10% от оставшейся суммы)
	// 1000 - 5% = 950, затем 950 - 10% = 855
	assert.True(t, finalAmount.LessThan(originalAmount))
}

// TestDiscountService_ApplyDiscounts_FixedDiscountExceedsAmount тестирует ApplyDiscounts когда фиксированная скидка превышает сумму
func TestDiscountService_ApplyDiscounts_FixedDiscountExceedsAmount(t *testing.T) {
	db := setupDiscountTestDB(t)
	service := NewDiscountService()

	// Создаем фиксированную скидку, которая превышает сумму
	discount := models.Discount{
		Level:     "object",
		EntityID:  1,
		Type:      "fixed",
		Value:     decimal.NewFromInt(1500), // Больше чем 1000
		IsActive:  true,
		StartDate: time.Now().AddDate(0, -1, 0),
		EndDate:   nil,
	}
	db.Create(&discount)

	ctx := context.Background()
	originalAmount := decimal.NewFromInt(1000)
	objectID := uint(1)

	finalAmount, applications, err := service.ApplyDiscounts(ctx, originalAmount, &objectID, nil, nil, nil, nil, time.Now())
	require.NoError(t, err)
	assert.Len(t, applications, 1)
	assert.True(t, finalAmount.IsZero())                    // Скидка не должна превышать сумму
	assert.Equal(t, originalAmount, applications[0].Amount) // Применена скидка только на сумму
}

// TestGetTotalDiscountAmount тестирует GetTotalDiscountAmount
func TestGetTotalDiscountAmount(t *testing.T) {
	applications := []DiscountApplication{
		{Amount: decimal.NewFromInt(100)},
		{Amount: decimal.NewFromInt(200)},
		{Amount: decimal.NewFromInt(50)},
	}

	total := GetTotalDiscountAmount(applications)
	assert.Equal(t, decimal.NewFromInt(350), total)
}

// TestGetTotalDiscountAmount_Empty тестирует GetTotalDiscountAmount с пустым списком
func TestGetTotalDiscountAmount_Empty(t *testing.T) {
	applications := []DiscountApplication{}

	total := GetTotalDiscountAmount(applications)
	assert.True(t, total.IsZero())
}
