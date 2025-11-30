package models

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// PartnerDailySnapshot представляет ежедневный снимок объектов партнера
// Снимки делаются каждый день в 00:00 UTC для тарификации партнерских договоров
type PartnerDailySnapshot struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Привязка
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index:idx_partner_snapshots_admin"`
	CompanyID      uint `json:"company_id" gorm:"not null;index:idx_partner_snapshots_company"`
	ContractID     uint `json:"contract_id" gorm:"not null;index:idx_partner_snapshots_contract"`

	// Дата снимка (начало дня в UTC)
	SnapshotDate time.Time `json:"snapshot_date" gorm:"not null;index:idx_partner_snapshots_date;uniqueIndex:idx_partner_snapshot_unique"`

	// ID учетной записи партнера
	PartnerCompanyID uint `json:"partner_company_id" gorm:"not null;index:idx_partner_snapshots_partner"`

	// Тарифный план на момент снимка
	TariffPlanID uint            `json:"tariff_plan_id" gorm:"not null"`
	TariffPlan   BillingPlan     `json:"tariff_plan,omitempty" gorm:"foreignKey:TariffPlanID;references:ID;constraint:-"`
	
	// Месячная цена тарифа (сохраняем для истории)
	MonthlyPrice decimal.Decimal `json:"monthly_price" gorm:"type:decimal(10,2);not null"`
	
	// Дневная цена = monthly_price / 30 (храним с высокой точностью для расчетов)
	DailyPrice decimal.Decimal `json:"daily_price" gorm:"type:decimal(12,6);not null"`

	// Количество объектов
	TotalObjectsCount  int `json:"total_objects_count" gorm:"not null;default:0"`  // Всего объектов в учетной записи
	ActiveObjectsCount int `json:"active_objects_count" gorm:"not null;default:0"` // Активных объектов (для тарификации)

	// Скидки
	DiscountType    string          `json:"discount_type" gorm:"type:varchar(20);default:'none'"` // none, manual, auto
	DiscountPercent decimal.Decimal `json:"discount_percent" gorm:"type:decimal(5,2);default:0"` // Процент скидки (0-100)
	
	// Стоимость до скидки = daily_price * active_objects_count
	CostBeforeDiscount decimal.Decimal `json:"cost_before_discount" gorm:"type:decimal(12,4);not null;default:0"`
	
	// Сумма скидки
	DiscountAmount decimal.Decimal `json:"discount_amount" gorm:"type:decimal(12,4);not null;default:0"`
	
	// Стоимость за день после применения скидки
	DailyCost decimal.Decimal `json:"daily_cost" gorm:"type:decimal(12,4);not null"`

	// Статус снимка
	Status string `json:"status" gorm:"type:varchar(20);default:'completed'"` // pending, completed, failed

	// Дополнительная информация
	Notes string `json:"notes" gorm:"type:text"` // Примечания (например, ошибки)
}

// TableName задает имя таблицы для модели PartnerDailySnapshot
func (PartnerDailySnapshot) TableName() string {
	return "partner_daily_snapshots"
}

// BeforeCreate устанавливает дефолтные значения перед созданием
func (s *PartnerDailySnapshot) BeforeCreate(tx *gorm.DB) error {
	// Дневная цена = месячная цена / 30, округляем до 4 знаков
	if s.MonthlyPrice.GreaterThan(decimal.Zero) {
		dailyPriceExact := s.MonthlyPrice.Div(decimal.NewFromInt(30))
		// Округляем до 4 знаков после запятой (как на калькуляторе)
		s.DailyPrice = dailyPriceExact.Round(4)
	}
	
	// Стоимость до скидки = (дневная цена) * количество объектов
	costExact := s.DailyPrice.Mul(decimal.NewFromInt(int64(s.ActiveObjectsCount)))
	s.CostBeforeDiscount = costExact.Round(2)
	
	// Рассчитываем скидку
	if s.DiscountPercent.GreaterThan(decimal.Zero) {
		// Сумма скидки = стоимость * (процент / 100)
		discountMultiplier := s.DiscountPercent.Div(decimal.NewFromInt(100))
		s.DiscountAmount = s.CostBeforeDiscount.Mul(discountMultiplier).Round(2)
	} else {
		s.DiscountAmount = decimal.Zero
	}
	
	// Итоговая стоимость = стоимость до скидки - сумма скидки
	s.DailyCost = s.CostBeforeDiscount.Sub(s.DiscountAmount).Round(2)
	
	return nil
}

