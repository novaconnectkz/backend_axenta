package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Contract представляет договор в системе
type Contract struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля договора
	Number      string `json:"number" gorm:"uniqueIndex;not null;type:varchar(50)"`
	Title       string `json:"title" gorm:"not null;type:varchar(200)"`
	Description string `json:"description" gorm:"type:text"`

	// Связь с компанией (мультитенантность)
	CompanyID uuid.UUID `json:"company_id" gorm:"type:uuid;not null;index"`

	// Клиент
	ClientName    string `json:"client_name" gorm:"not null;type:varchar(200)"`
	ClientINN     string `json:"client_inn" gorm:"type:varchar(20)"`
	ClientKPP     string `json:"client_kpp" gorm:"type:varchar(20)"`
	ClientEmail   string `json:"client_email" gorm:"type:varchar(100)"`
	ClientPhone   string `json:"client_phone" gorm:"type:varchar(20)"`
	ClientAddress string `json:"client_address" gorm:"type:text"`

	// Даты договора
	StartDate time.Time  `json:"start_date" gorm:"not null"`
	EndDate   time.Time  `json:"end_date" gorm:"not null"`
	SignedAt  *time.Time `json:"signed_at"`

	// Тарификация
	TariffPlanID uint        `json:"tariff_plan_id" gorm:"not null"`
	TariffPlan   BillingPlan `json:"tariff_plan" gorm:"foreignKey:TariffPlanID"`

	// Стоимость
	TotalAmount decimal.Decimal `json:"total_amount" gorm:"type:decimal(15,2)"`
	Currency    string          `json:"currency" gorm:"default:'RUB';type:varchar(3)"`

	// Статус договора
	Status   string `json:"status" gorm:"default:'draft';type:varchar(20)"` // draft, active, expired, cancelled, suspended
	IsActive bool   `json:"is_active" gorm:"default:true"`

	// Настройки уведомлений
	NotifyBefore int `json:"notify_before" gorm:"default:30"` // За сколько дней уведомлять об истечении

	// Дополнительная информация
	Notes      string `json:"notes" gorm:"type:text"`
	ExternalID string `json:"external_id" gorm:"type:varchar(100)"` // ID во внешних системах (1С, Битрикс24)

	// Связи
	Appendices []ContractAppendix `json:"appendices,omitempty" gorm:"foreignKey:ContractID"`
	Objects    []Object           `json:"objects,omitempty" gorm:"foreignKey:ContractID"`
}

// TableName задает имя таблицы для модели Contract
func (Contract) TableName() string {
	return "contracts"
}

// IsExpired проверяет, истек ли договор
func (c *Contract) IsExpired() bool {
	return time.Now().After(c.EndDate)
}

// IsExpiringSoon проверяет, истекает ли договор скоро
func (c *Contract) IsExpiringSoon() bool {
	notifyDate := c.EndDate.AddDate(0, 0, -c.NotifyBefore)
	return time.Now().After(notifyDate) && !c.IsExpired()
}

// GetDaysUntilExpiry возвращает количество дней до истечения договора
func (c *Contract) GetDaysUntilExpiry() int {
	if c.IsExpired() {
		return 0
	}
	duration := c.EndDate.Sub(time.Now())
	return int(duration.Hours() / 24)
}

// ContractAppendix представляет приложение к договору
type ContractAppendix struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связь с договором
	ContractID uint     `json:"contract_id" gorm:"not null;index"`
	Contract   Contract `json:"contract,omitempty" gorm:"foreignKey:ContractID"`

	// Основные поля приложения
	Number      string `json:"number" gorm:"not null;type:varchar(50)"`
	Title       string `json:"title" gorm:"not null;type:varchar(200)"`
	Description string `json:"description" gorm:"type:text"`

	// Даты приложения
	StartDate time.Time  `json:"start_date" gorm:"not null"`
	EndDate   time.Time  `json:"end_date" gorm:"not null"`
	SignedAt  *time.Time `json:"signed_at"`

	// Стоимость приложения
	Amount   decimal.Decimal `json:"amount" gorm:"type:decimal(15,2)"`
	Currency string          `json:"currency" gorm:"default:'RUB';type:varchar(3)"`

	// Статус приложения
	Status   string `json:"status" gorm:"default:'draft';type:varchar(20)"` // draft, active, expired, cancelled
	IsActive bool   `json:"is_active" gorm:"default:true"`

	// Дополнительная информация
	Notes      string `json:"notes" gorm:"type:text"`
	ExternalID string `json:"external_id" gorm:"type:varchar(100)"`
}

// TableName задает имя таблицы для модели ContractAppendix
func (ContractAppendix) TableName() string {
	return "contract_appendices"
}

// IsExpired проверяет, истекло ли приложение
func (ca *ContractAppendix) IsExpired() bool {
	return time.Now().After(ca.EndDate)
}

// TariffPlan представляет тарифный план (расширяет BillingPlan)
type TariffPlan struct {
	BillingPlan

	// Дополнительные поля для тарификации
	SetupFee         decimal.Decimal `json:"setup_fee" gorm:"type:decimal(10,2);default:0"`       // Плата за подключение
	MinimumPeriod    int             `json:"minimum_period" gorm:"default:1"`                     // Минимальный период в месяцах
	DiscountPercent  decimal.Decimal `json:"discount_percent" gorm:"type:decimal(5,2);default:0"` // Скидка в процентах
	IsPromotional    bool            `json:"is_promotional" gorm:"default:false"`                 // Акционный тариф
	PromotionalUntil *time.Time      `json:"promotional_until"`                                   // До какой даты действует акция

	// Тарификация по объектам
	PricePerObject   decimal.Decimal `json:"price_per_object" gorm:"type:decimal(10,2)"` // Цена за объект
	FreeObjectsCount int             `json:"free_objects_count" gorm:"default:0"`        // Количество бесплатных объектов

	// Специальные тарифы
	InactivePriceRatio decimal.Decimal `json:"inactive_price_ratio" gorm:"type:decimal(3,2);default:0.5"` // Коэффициент для неактивных объектов
}

// TableName задает имя таблицы для модели TariffPlan
func (TariffPlan) TableName() string {
	return "tariff_plans"
}

// CalculateObjectPrice рассчитывает стоимость для определенного количества объектов
func (tp *TariffPlan) CalculateObjectPrice(objectCount int, inactiveCount int) decimal.Decimal {
	if objectCount <= 0 {
		return decimal.Zero
	}

	// Учитываем бесплатные объекты
	billableObjects := objectCount - tp.FreeObjectsCount
	if billableObjects <= 0 {
		return decimal.Zero
	}

	// Основная стоимость
	activeObjects := billableObjects - inactiveCount
	totalPrice := tp.PricePerObject.Mul(decimal.NewFromInt(int64(activeObjects)))

	// Добавляем стоимость неактивных объектов со скидкой
	if inactiveCount > 0 {
		inactivePrice := tp.PricePerObject.Mul(tp.InactivePriceRatio).Mul(decimal.NewFromInt(int64(inactiveCount)))
		totalPrice = totalPrice.Add(inactivePrice)
	}

	// Применяем скидку
	if tp.DiscountPercent.GreaterThan(decimal.Zero) {
		discount := totalPrice.Mul(tp.DiscountPercent).Div(decimal.NewFromInt(100))
		totalPrice = totalPrice.Sub(discount)
	}

	return totalPrice
}
