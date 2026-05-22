package models

import (
	"fmt"
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
	ContractID     uint `json:"contract_id" gorm:"not null;index:idx_partner_snapshots_contract;uniqueIndex:idx_partner_snapshot_unique,priority:4"`

	// Дата снимка (начало дня в UTC)
	SnapshotDate time.Time `json:"snapshot_date" gorm:"not null;index:idx_partner_snapshots_date;uniqueIndex:idx_partner_snapshot_unique,priority:5"`

	// ID учетной записи партнера
	PartnerCompanyID uint `json:"partner_company_id" gorm:"not null;index:idx_partner_snapshots_partner"`

	// Мульти-системный партнёр (Ф0). Составной uniqueIndex idx_partner_snapshot_unique =
	// (partner_source, connection_id, partner_external_id, contract_id, snapshot_date).
	// Для Axenta: source='axenta', connection_id=0, partner_external_id=partner_company_id.
	// NON-partial (GORM не умеет WHERE deleted_at) — данные чистые, soft-delete+recreate
	// в штатном потоке не происходит (scheduler пропускает существующие через Unscoped).
	PartnerSource     string `json:"partner_source" gorm:"type:varchar(20);not null;default:'axenta';uniqueIndex:idx_partner_snapshot_unique,priority:1"`
	ConnectionID      uint   `json:"connection_id" gorm:"not null;default:0;uniqueIndex:idx_partner_snapshot_unique,priority:2"`
	PartnerExternalID string `json:"partner_external_id" gorm:"type:varchar(128);not null;default:'';uniqueIndex:idx_partner_snapshot_unique,priority:3"`

	// Тарифный план на момент снимка
	TariffPlanID uint        `json:"tariff_plan_id" gorm:"not null"`
	TariffPlan   BillingPlan `json:"tariff_plan,omitempty" gorm:"foreignKey:TariffPlanID;references:ID;constraint:-"`

	// Месячная цена тарифа (сохраняем для истории)
	MonthlyPrice decimal.Decimal `json:"monthly_price" gorm:"type:decimal(10,2);not null"`

	// Дневная цена = monthly_price / 30 (храним с высокой точностью для расчетов)
	DailyPrice decimal.Decimal `json:"daily_price" gorm:"type:decimal(12,6);not null"`

	// Количество объектов
	TotalObjectsCount  int `json:"total_objects_count" gorm:"not null;default:0"`  // Всего объектов в учетной записи
	ActiveObjectsCount int `json:"active_objects_count" gorm:"not null;default:0"` // Активных объектов (для тарификации)

	// Скидки (используется только один тип: либо процент, либо фиксированная сумма)
	DiscountType    string          `json:"discount_type" gorm:"type:varchar(20);default:'none'"` // none, manual, auto
	DiscountPercent decimal.Decimal `json:"discount_percent" gorm:"type:decimal(5,2);default:0"`  // Процент скидки (0-100)
	DiscountFixed   decimal.Decimal `json:"discount_fixed" gorm:"type:decimal(12,2);default:0"`   // Фиксированная скидка в рублях

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
	// Ф0: гарантируем заполнение мульти-системных полей на ВСЕХ путях создания.
	// Без этого Axenta-снимки писались бы с partner_external_id='' → orphan-строки
	// разных партнёров (contract_id=0) схлопнулись бы по составному unique-индексу.
	if s.PartnerSource == "" {
		s.PartnerSource = "axenta"
	}
	if s.PartnerExternalID == "" && s.PartnerCompanyID > 0 {
		s.PartnerExternalID = fmt.Sprintf("%d", s.PartnerCompanyID)
	}

	// Для фиксированной скидки: применяем скидку к месячному тарифу, затем рассчитываем дневную цену
	// Для процентной скидки: рассчитываем дневную цену из базового тарифа, затем применяем скидку к стоимости

	var effectiveDailyPrice decimal.Decimal

	if s.DiscountFixed.GreaterThan(decimal.Zero) {
		// Фиксированная скидка применяется к МЕСЯЧНОМУ тарифу
		effectiveMonthlyPrice := s.MonthlyPrice.Sub(s.DiscountFixed)
		// Если скидка больше месячной цены, устанавливаем 0
		if effectiveMonthlyPrice.IsNegative() {
			effectiveMonthlyPrice = decimal.Zero
		}
		// Рассчитываем эффективную дневную цену
		effectiveDailyPrice = effectiveMonthlyPrice.Div(decimal.NewFromInt(30)).Round(4)
		s.DailyPrice = effectiveDailyPrice

		// Стоимость = эффективная дневная цена * количество объектов
		s.CostBeforeDiscount = s.MonthlyPrice.Div(decimal.NewFromInt(30)).Mul(decimal.NewFromInt(int64(s.ActiveObjectsCount))).Round(2)
		s.DiscountAmount = s.DiscountFixed.Div(decimal.NewFromInt(30)).Mul(decimal.NewFromInt(int64(s.ActiveObjectsCount))).Round(2)
		s.DailyCost = effectiveDailyPrice.Mul(decimal.NewFromInt(int64(s.ActiveObjectsCount))).Round(2)
	} else {
		// Базовая дневная цена (без скидки)
		baseDailyPrice := s.MonthlyPrice.Div(decimal.NewFromInt(30)).Round(4)
		s.DailyPrice = baseDailyPrice

		// Стоимость до скидки
		s.CostBeforeDiscount = baseDailyPrice.Mul(decimal.NewFromInt(int64(s.ActiveObjectsCount))).Round(2)

		// Процентная скидка применяется к стоимости
		if s.DiscountPercent.GreaterThan(decimal.Zero) {
			discountMultiplier := s.DiscountPercent.Div(decimal.NewFromInt(100))
			s.DiscountAmount = s.CostBeforeDiscount.Mul(discountMultiplier).Round(2)
		} else {
			s.DiscountAmount = decimal.Zero
		}

		// Итоговая стоимость
		s.DailyCost = s.CostBeforeDiscount.Sub(s.DiscountAmount).Round(2)
	}

	return nil
}
