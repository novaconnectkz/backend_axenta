package services

import (
	"context"
	"fmt"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// DiscountService предоставляет функции для работы со скидками
type DiscountService struct {
	db *gorm.DB
}

// NewDiscountService создает новый экземпляр DiscountService
func NewDiscountService() *DiscountService {
	return &DiscountService{
		db: database.DB,
	}
}

// DiscountLevel представляет приоритет уровня скидки (меньше = выше приоритет)
var DiscountLevel = map[string]int{
	"object":      1, // Самый высокий приоритет
	"tariff":      2,
	"subscription": 3,
	"appendix":    4,
	"contract":    5, // Самый низкий приоритет
}

// GetActiveDiscounts возвращает активные скидки для указанных сущностей на определенную дату
func (ds *DiscountService) GetActiveDiscounts(ctx context.Context, level string, entityID uint, date time.Time) ([]models.Discount, error) {
	var discounts []models.Discount

	query := ds.db.Where("level = ? AND entity_id = ? AND is_active = true", level, entityID).
		Where("start_date <= ?", date).
		Where("(end_date IS NULL OR end_date >= ?)", date)

	if err := query.Find(&discounts).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения скидок: %w", err)
	}

	return discounts, nil
}

// FindDiscountsForHierarchy находит все активные скидки для иерархии сущностей
// Применяются скидки по уровням: object → tariff → subscription → appendix → contract
// Возвращает скидки в порядке приоритета (от высшего к низшему)
func (ds *DiscountService) FindDiscountsForHierarchy(ctx context.Context, 
	objectID, tariffID, subscriptionID, appendixID, contractID *uint, 
	date time.Time) ([]models.Discount, error) {
	
	var allDiscounts []models.Discount

	// Получаем скидки на каждом уровне (в порядке приоритета)
	if objectID != nil {
		discounts, err := ds.GetActiveDiscounts(ctx, "object", *objectID, date)
		if err != nil {
			return nil, err
		}
		allDiscounts = append(allDiscounts, discounts...)
	}

	if tariffID != nil {
		discounts, err := ds.GetActiveDiscounts(ctx, "tariff", *tariffID, date)
		if err != nil {
			return nil, err
		}
		allDiscounts = append(allDiscounts, discounts...)
	}

	if subscriptionID != nil {
		discounts, err := ds.GetActiveDiscounts(ctx, "subscription", *subscriptionID, date)
		if err != nil {
			return nil, err
		}
		allDiscounts = append(allDiscounts, discounts...)
	}

	if appendixID != nil {
		discounts, err := ds.GetActiveDiscounts(ctx, "appendix", *appendixID, date)
		if err != nil {
			return nil, err
		}
		allDiscounts = append(allDiscounts, discounts...)
	}

	if contractID != nil {
		discounts, err := ds.GetActiveDiscounts(ctx, "contract", *contractID, date)
		if err != nil {
			return nil, err
		}
		allDiscounts = append(allDiscounts, discounts...)
	}

	return allDiscounts, nil
}

// ApplyDiscounts применяет скидки к сумме согласно приоритету уровней
// Применяются все скидки в порядке приоритета (от высшего к низшему)
func (ds *DiscountService) ApplyDiscounts(ctx context.Context, 
	originalAmount decimal.Decimal,
	objectID, tariffID, subscriptionID, appendixID, contractID *uint,
	date time.Time) (decimal.Decimal, []DiscountApplication, error) {
	
	discounts, err := ds.FindDiscountsForHierarchy(ctx, objectID, tariffID, subscriptionID, appendixID, contractID, date)
	if err != nil {
		return originalAmount, nil, err
	}

	if len(discounts) == 0 {
		return originalAmount, []DiscountApplication{}, nil
	}

	// Сортируем скидки по приоритету уровня
	discountsByPriority := make(map[int][]models.Discount)
	for _, discount := range discounts {
		priority, exists := DiscountLevel[discount.Level]
		if !exists {
			continue // Пропускаем неизвестный уровень
		}
		discountsByPriority[priority] = append(discountsByPriority[priority], discount)
	}

	var applications []DiscountApplication
	currentAmount := originalAmount

	// Применяем скидки по уровням (от высшего приоритета к низшему)
	for priority := 1; priority <= 5; priority++ {
		levelDiscounts, exists := discountsByPriority[priority]
		if !exists {
			continue
		}

		// Применяем все скидки на этом уровне
		for _, discount := range levelDiscounts {
			var discountAmount decimal.Decimal
			
			if discount.Type == "percent" {
				// Процентная скидка
				discountAmount = currentAmount.Mul(discount.Value).Div(decimal.NewFromInt(100))
			} else if discount.Type == "fixed" {
				// Фиксированная скидка
				discountAmount = discount.Value
				// Не позволяем скидке превышать текущую сумму
				if discountAmount.GreaterThan(currentAmount) {
					discountAmount = currentAmount
				}
			}

			if discountAmount.GreaterThan(decimal.Zero) {
				currentAmount = currentAmount.Sub(discountAmount)
				
				applications = append(applications, DiscountApplication{
					Level:        discount.Level,
					EntityID:     discount.EntityID,
					Type:         discount.Type,
					Value:        discount.Value,
					Amount:       discountAmount,
					Reason:       discount.Reason,
				})
			}
		}
	}

	return currentAmount, applications, nil
}

// DiscountApplication представляет применение одной скидки
type DiscountApplication struct {
	Level    string          `json:"level"`    // Уровень скидки
	EntityID uint            `json:"entity_id"` // ID сущности
	Type     string          `json:"type"`     // percent или fixed
	Value    decimal.Decimal `json:"value"`    // Значение скидки
	Amount   decimal.Decimal `json:"amount"`   // Сумма скидки
	Reason   string          `json:"reason"`   // Причина скидки
}

// GetTotalDiscountAmount возвращает общую сумму всех примененных скидок
func GetTotalDiscountAmount(applications []DiscountApplication) decimal.Decimal {
	total := decimal.Zero
	for _, app := range applications {
		total = total.Add(app.Amount)
	}
	return total
}

