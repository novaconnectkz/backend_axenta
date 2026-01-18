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

// VATResolver предоставляет функции для определения ставки НДС
type VATResolver struct {
	db *gorm.DB
}

// NewVATResolver создает новый экземпляр VATResolver
func NewVATResolver() *VATResolver {
	return &VATResolver{
		db: database.DB,
	}
}

// ResolveVAT возвращает ставку НДС для транзакции между странами
// Сначала ищет правило в tax_rules, затем использует tax_rates
func (vr *VATResolver) ResolveVAT(ctx context.Context, sellerCountry, buyerCountry, serviceType string, period time.Time) (decimal.Decimal, string, error) {
	// 1. Ищем правило в tax_rules
	var taxRule models.TaxRule
	query := vr.db.Where("seller_country_code = ? AND buyer_country_code = ? AND is_active = true", sellerCountry, buyerCountry)

	// Если указан тип услуги, добавляем фильтр
	if serviceType != "" {
		query = query.Where("service_type = ? OR service_type = ''", serviceType)
	}

	// Проверяем период действия правила
	query = query.Where("(period_start IS NULL OR period_start <= ?) AND (period_end IS NULL OR period_end >= ?)", period, period)

	// Сортируем по приоритету (выше приоритет = выше приоритет)
	err := query.Order("priority DESC").First(&taxRule).Error

	if err == nil && taxRule.ApplyTax {
		// Нашли правило - проверяем переопределение ставки
		if taxRule.TaxRateOverride != nil {
			return *taxRule.TaxRateOverride, "rule_override", nil
		}

		// Используем базовую ставку из tax_rates
		rate, err := vr.findTaxRateForCountry(sellerCountry, period)
		if err != nil {
			return decimal.Zero, "error", fmt.Errorf("не найдена ставка НДС для страны %s: %w", sellerCountry, err)
		}
		return rate, "rule_default", nil
	}

	// 2. Правило не найдено - используем базовую ставку страны продавца
	if err != nil && err != gorm.ErrRecordNotFound {
		return decimal.Zero, "error", fmt.Errorf("ошибка поиска правила НДС: %w", err)
	}

	rate, err := vr.findTaxRateForCountry(sellerCountry, period)
	if err != nil {
		return decimal.Zero, "error", fmt.Errorf("не найдена ставка НДС для страны %s: %w", sellerCountry, err)
	}

	return rate, "default_rate", nil
}

// findTaxRateForCountry находит актуальную ставку НДС для страны
func (vr *VATResolver) findTaxRateForCountry(countryCode string, period time.Time) (decimal.Decimal, error) {
	var taxRate models.TaxRate

	err := vr.db.Where("country_code = ? AND is_active = true", countryCode).
		Where("effective_from <= ? AND (effective_to IS NULL OR effective_to >= ?)", period, period).
		Order("priority DESC").
		First(&taxRate).Error

	if err != nil {
		// Ставка не найдена - возвращаем 0
		return decimal.Zero, err
	}

	return taxRate.Rate, nil
}

// GetActiveTaxRate возвращает текущую активную ставку НДС для страны
func (vr *VATResolver) GetActiveTaxRate(ctx context.Context, countryCode string) (decimal.Decimal, error) {
	return vr.findTaxRateForCountry(countryCode, time.Now())
}

// BillingCalculatorAdvanced предоставляет расширенный калькулятор биллинга
type BillingCalculatorAdvanced struct {
	db             *gorm.DB
	companyID      uint
	adminAccountID uint
}

// NewBillingCalculatorAdvanced создает новый экземпляр BillingCalculatorAdvanced
func NewBillingCalculatorAdvanced() *BillingCalculatorAdvanced {
	return &BillingCalculatorAdvanced{
		db: database.DB,
	}
}

// NewBillingCalculatorAdvancedWithContext создает новый экземпляр BillingCalculatorAdvanced с контекстом компании
func NewBillingCalculatorAdvancedWithContext(companyID, adminAccountID uint) *BillingCalculatorAdvanced {
	return &BillingCalculatorAdvanced{
		db:             database.DB,
		companyID:      companyID,
		adminAccountID: adminAccountID,
	}
}

// CalculateBillingForMonth рассчитывает биллинг за месяц с учетом заморозок и использования
func (bc *BillingCalculatorAdvanced) CalculateBillingForMonth(ctx context.Context, companyID uint, period string) (map[string]interface{}, error) {
	// Парсим период (формат YYYY-MM)
	periodTime, err := time.Parse("2006-01", period)
	if err != nil {
		return nil, fmt.Errorf("некорректный формат периода: %w", err)
	}

	// Определяем начало и конец месяца
	startDate := time.Date(periodTime.Year(), periodTime.Month(), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0).Add(-time.Second) // Последний день месяца

	// Получаем активные подписки компании
	var subscriptions []models.Subscription
	if err := bc.db.Where("company_id = ? AND status = ?", companyID, "active").
		Preload("BillingPlan").
		Find(&subscriptions).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения подписок: %w", err)
	}

	if len(subscriptions) == 0 {
		return map[string]interface{}{
			"period":     period,
			"start_date": startDate,
			"end_date":   endDate,
			"total":      decimal.Zero,
			"items":      []interface{}{},
		}, nil
	}

	// Рассчитываем для каждой подписки
	var totalAmount decimal.Decimal
	var allItems []map[string]interface{}

	for _, sub := range subscriptions {
		items, amount, err := bc.calculateSubscriptionBilling(ctx, sub, startDate, endDate)
		if err != nil {
			return nil, fmt.Errorf("ошибка расчета для подписки %d: %w", sub.ID, err)
		}

		allItems = append(allItems, items...)
		totalAmount = totalAmount.Add(amount)
	}

	return map[string]interface{}{
		"period":     period,
		"start_date": startDate,
		"end_date":   endDate,
		"total":      totalAmount,
		"items":      allItems,
	}, nil
}

// calculateSubscriptionBilling рассчитывает биллинг для конкретной подписки
func (bc *BillingCalculatorAdvanced) calculateSubscriptionBilling(ctx context.Context, sub models.Subscription, startDate, endDate time.Time) ([]map[string]interface{}, decimal.Decimal, error) {
	var items []map[string]interface{}
	var totalAmount decimal.Decimal

	// Получаем привязки объектов к подписке
	var assignments []models.Assignment
	if err := bc.db.Where("subscription_id = ? AND status = ?", sub.ID, "active").
		Preload("Object").
		Preload("TariffPlan").
		Find(&assignments).Error; err != nil {
		return nil, decimal.Zero, fmt.Errorf("ошибка получения привязок: %w", err)
	}

	// Для каждого объекта рассчитываем стоимость
	for _, assignment := range assignments {
		itemAmount, itemDetails, err := bc.calculateObjectBilling(ctx, assignment, startDate, endDate)
		if err != nil {
			return nil, decimal.Zero, fmt.Errorf("ошибка расчета для объекта %d: %w", assignment.ObjectID, err)
		}

		if itemAmount.GreaterThan(decimal.Zero) {
			items = append(items, map[string]interface{}{
				"object_id":   assignment.ObjectID,
				"object_name": assignment.Object.Name,
				"amount":      itemAmount,
				"details":     itemDetails,
			})
			totalAmount = totalAmount.Add(itemAmount)
		}
	}

	// TODO: Добавить расчет базы подписки (recurring компоненты)

	return items, totalAmount, nil
}

// calculateObjectBilling рассчитывает стоимость объекта с учетом заморозок и использования
func (bc *BillingCalculatorAdvanced) calculateObjectBilling(_ context.Context, assignment models.Assignment, startDate, endDate time.Time) (decimal.Decimal, map[string]interface{}, error) {
	// Получаем настройки биллинга для компании
	var settings models.BillingSettings
	if err := bc.db.Where("company_id = ? AND admin_account_id = ?", bc.companyID, bc.adminAccountID).
		First(&settings).Error; err != nil {
		// Если настройки не найдены, используем дефолтные значения
		settings = models.BillingSettings{
			MinDaysForFullMonth: 5, // Значение по умолчанию
		}
	}

	// Получаем заморозки объекта за период
	var freezes []models.Freeze
	if err := bc.db.Where("object_id = ? AND (start_date <= ? AND end_date >= ?)", assignment.ObjectID, endDate, startDate).
		Find(&freezes).Error; err != nil {
		return decimal.Zero, nil, fmt.Errorf("ошибка получения заморозок: %w", err)
	}

	// Рассчитываем количество биллинговых дней
	totalDays := int(endDate.Sub(startDate).Hours()/24) + 1
	billableDays := totalDays

	// Обрабатываем заморозки согласно roadmap Этап 5.3
	var freezeDays int
	var frozenTariffPrice *decimal.Decimal

	for _, freeze := range freezes {
		freezeStart := maxTime(freeze.StartDate, startDate)
		freezeEnd := minTime(freeze.EndDate, endDate)
		currentFreezeDays := int(freezeEnd.Sub(freezeStart).Hours()/24) + 1

		if !freeze.Billable {
			// billable=false — исключаем дни из биллинга
			freezeDays += currentFreezeDays
		} else {
			// billable=true — используем альтернативный тариф (assignment.status='frozen')
			if assignment.Status == "frozen" {
				// Получаем альтернативный тариф для замороженных объектов
				// TODO: Добавить логику получения альтернативного тарифа
				// Пока используем уменьшенную цену (например, 50% от базовой)
				if frozenTariffPrice == nil {
					// Получаем базовую цену из тарифного плана
					if assignment.TariffPlan != nil && assignment.TariffPlan.PricePerObject.GreaterThan(decimal.Zero) {
						frozenPrice := assignment.TariffPlan.PricePerObject.Mul(decimal.NewFromFloat(0.5)) // 50% от базовой
						frozenTariffPrice = &frozenPrice
					}
				}
			}
		}
	}

	// Вычитаем дни заморозок (billable=false)
	billableDays -= freezeDays

	if billableDays <= 0 {
		return decimal.Zero, map[string]interface{}{
			"total_days":    totalDays,
			"freeze_days":   freezeDays,
			"billable_days": 0,
		}, nil
	}

	// Получаем тарифные компоненты для расчета стоимости
	var tariffComponents []models.TariffComponent
	if err := bc.db.Where("tariff_plan_id = ? AND type = ? AND is_active = true AND deleted_at IS NULL",
		assignment.TariffPlanID, "recurring").Find(&tariffComponents).Error; err == nil && len(tariffComponents) > 0 {

		// Используем первый recurring компонент
		component := tariffComponents[0]
		var dailyPrice decimal.Decimal
		var amount decimal.Decimal
		var chargedAsFullMonth bool

		if component.BillingPeriod == "monthly" {
			// Цена за месяц / количество дней в месяце
			daysInMonth := int(endDate.Sub(startDate).Hours()/24) + 1
			if daysInMonth > 0 {
				dailyPrice = component.Price.Div(decimal.NewFromInt(int64(daysInMonth)))
			}

			// Применяем логику минимального количества дней для полного месяца
			minDays := settings.MinDaysForFullMonth
			if minDays == 0 {
				minDays = 5 // Дефолтное значение, если не задано
			}

			if billableDays >= minDays {
				// Если объект присутствует >= минимального количества дней, списываем полный месяц
				amount = component.Price
				chargedAsFullMonth = true
			} else {
				// Иначе списываем пропорционально дням
				amount = dailyPrice.Mul(decimal.NewFromInt(int64(billableDays)))
				chargedAsFullMonth = false
			}
		} else if component.BillingPeriod == "yearly" {
			dailyPrice = component.Price.Div(decimal.NewFromInt(365))
			amount = dailyPrice.Mul(decimal.NewFromInt(int64(billableDays)))
		} else {
			dailyPrice = component.Price
			amount = dailyPrice.Mul(decimal.NewFromInt(int64(billableDays)))
		}

		return amount, map[string]interface{}{
			"total_days":              totalDays,
			"freeze_days":             freezeDays,
			"billable_days":           billableDays,
			"price_per_day":           dailyPrice,
			"component_id":            component.ID,
			"charged_as_full_month":   chargedAsFullMonth,
			"min_days_for_full_month": settings.MinDaysForFullMonth,
		}, nil
	}

	// Fallback: используем фиксированную цену (если нет тарифных компонентов)
	price := decimal.NewFromInt(100) // Пример
	amount := price.Mul(decimal.NewFromInt(int64(billableDays)))

	return amount, map[string]interface{}{
		"total_days":    totalDays,
		"freeze_days":   freezeDays,
		"billable_days": billableDays,
		"price_per_day": price,
	}, nil
}

// Вспомогательные функции для работы с датами
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
