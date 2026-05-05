package models

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Country представляет справочник стран для определения налогов
type Country struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	Code        string `json:"code" gorm:"uniqueIndex;not null;type:varchar(2)"` // ISO код (RU, KZ и т.д.)
	Name        string `json:"name" gorm:"not null;type:varchar(100)"`           // Название страны
	DisplayName string `json:"display_name" gorm:"type:varchar(100)"`            // Отображаемое название
	IsActive    bool   `json:"is_active" gorm:"default:true"`                    // Активность

	// Связи
	TaxRates []TaxRate `json:"tax_rates,omitempty" gorm:"foreignKey:CountryCode;constraint:-;references:Code"`
	TaxRules []TaxRule `json:"tax_rules,omitempty" gorm:"foreignKey:SellerCountryCode;constraint:-;references:Code"`
}

// TableName задает имя таблицы для модели Country
func (Country) TableName() string {
	return "countries"
}

// TaxRate представляет ставку НДС для страны
type TaxRate struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	CountryCode string          `json:"country_code" gorm:"not null;type:varchar(2);index"`
	Country     *Country        `json:"country,omitempty" gorm:"foreignKey:CountryCode;constraint:-;references:Code"`
	Rate        decimal.Decimal `json:"rate" gorm:"not null;type:decimal(5,2)"` // НДС в процентах
	Name        string          `json:"name" gorm:"type:varchar(100)"`          // Название ставки (например, "НДС 20%")

	// Временные рамки действия
	EffectiveFrom time.Time  `json:"effective_from" gorm:"not null"`
	EffectiveTo   *time.Time `json:"effective_to"` // NULL = действует бессрочно

	// Приоритет для выбора ставки
	Priority int  `json:"priority" gorm:"default:0"` // Больше = выше приоритет
	IsActive bool `json:"is_active" gorm:"default:true"`
}

// TableName задает имя таблицы для модели TaxRate
func (TaxRate) TableName() string {
	return "tax_rates"
}

// TaxRule представляет правило применения НДС между странами
type TaxRule struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Страны операции
	SellerCountryCode string   `json:"seller_country_code" gorm:"not null;type:varchar(2);index"`
	SellerCountry     *Country `json:"seller_country,omitempty" gorm:"foreignKey:SellerCountryCode;constraint:-;references:Code"`
	BuyerCountryCode  string   `json:"buyer_country_code" gorm:"not null;type:varchar(2);index"`
	BuyerCountry      *Country `json:"buyer_country,omitempty" gorm:"foreignKey:BuyerCountryCode;constraint:-;references:Code"`

	// Условия применения
	ServiceType string     `json:"service_type" gorm:"type:varchar(50)"` // Тип услуги (например, "software", "monitoring")
	PeriodStart *time.Time `json:"period_start"`                         // Начало действия правила
	PeriodEnd   *time.Time `json:"period_end"`                           // Конец действия правила

	// Результат правила
	ApplyTax        bool             `json:"apply_tax" gorm:"default:true"`              // Применять ли НДС
	TaxRateOverride *decimal.Decimal `json:"tax_rate_override" gorm:"type:decimal(5,2)"` // Переопределение ставки НДС
	RateSource      string           `json:"rate_source" gorm:"type:varchar(50)"`        // Источник ставки: "default", "override", "rule"

	// Приоритет
	Priority int  `json:"priority" gorm:"default:0"`
	IsActive bool `json:"is_active" gorm:"default:true"`
}

// TableName задает имя таблицы для модели TaxRule
func (TaxRule) TableName() string {
	return "tax_rules"
}

// TariffComponent представляет компонент тарифа (абонплата, за объект, за использование)
type TariffComponent struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связь с тарифом
	AdminAccountID uint        `json:"admin_account_id" gorm:"not null;index"`
	TariffPlanID   uint        `json:"tariff_plan_id" gorm:"not null;index"`
	TariffPlan     *TariffPlan `json:"tariff_plan,omitempty" gorm:"foreignKey:TariffPlanID;constraint:-"`

	// Тип компонента
	Type string `json:"type" gorm:"not null;type:varchar(50)"` // recurring, per_usage, one_time

	// Параметры компонента
	Name      string          `json:"name" gorm:"not null;type:varchar(200)"`            // Название компонента
	Unit      string          `json:"unit" gorm:"type:varchar(50)"`                      // Единица измерения
	Price     decimal.Decimal `json:"price" gorm:"not null;type:decimal(15,2)"`          // Цена за единицу
	MinCommit decimal.Decimal `json:"min_commit" gorm:"type:decimal(15,2);default:0.00"` // Минимальная оплата

	// Для recurring
	BillingPeriod string `json:"billing_period" gorm:"type:varchar(20)"` // monthly, yearly

	// Статус
	IsActive bool `json:"is_active" gorm:"default:true"`
}

// TableName задает имя таблицы для модели TariffComponent
func (TariffComponent) TableName() string {
	return "tariff_components"
}

// Assignment представляет привязку объекта к подписке
type Assignment struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связи
	SubscriptionID uint          `json:"subscription_id" gorm:"not null;index"`
	Subscription   *Subscription `json:"subscription,omitempty" gorm:"foreignKey:SubscriptionID;constraint:-"`
	ObjectID       uint          `json:"object_id" gorm:"not null;index"`
	Object         *Object       `json:"object,omitempty" gorm:"foreignKey:ObjectID;constraint:-"`
	TariffPlanID   uint          `json:"tariff_plan_id" gorm:"not null;index"`
	TariffPlan     *TariffPlan   `json:"tariff_plan,omitempty" gorm:"foreignKey:TariffPlanID;constraint:-"`

	// Период действия привязки
	StartDate time.Time  `json:"start_date" gorm:"not null"`
	EndDate   *time.Time `json:"end_date"` // NULL = бессрочно

	// Статус
	Status   string `json:"status" gorm:"default:'active';type:varchar(20)"` // active, frozen, cancelled
	IsActive bool   `json:"is_active" gorm:"default:true"`
}

// TableName задает имя таблицы для модели Assignment
func (Assignment) TableName() string {
	return "assignments"
}

// Freeze представляет заморозку объекта (период не тарификации)
type Freeze struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связи
	ObjectID     uint        `json:"object_id" gorm:"not null;index"`
	Object       *Object     `json:"object,omitempty" gorm:"foreignKey:ObjectID;constraint:-"`
	AssignmentID *uint       `json:"assignment_id" gorm:"index"`
	Assignment   *Assignment `json:"assignment,omitempty" gorm:"foreignKey:AssignmentID;constraint:-"`

	// Период заморозки
	StartDate time.Time `json:"start_date" gorm:"not null"`
	EndDate   time.Time `json:"end_date" gorm:"not null"`

	// Тип заморозки
	Billable bool   `json:"billable" gorm:"default:false"` // Если true - использовать альтернативный тариф
	Reason   string `json:"reason" gorm:"type:text"`       // Причина заморозки

	// Статус
	IsActive bool `json:"is_active" gorm:"default:true"`
}

// TableName задает имя таблицы для модели Freeze
func (Freeze) TableName() string {
	return "freezes"
}

// Usage представляет использование объекта за конкретный день
type Usage struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связи
	ObjectID       uint          `json:"object_id" gorm:"not null;index"`
	Object         *Object       `json:"object,omitempty" gorm:"foreignKey:ObjectID;constraint:-"`
	SubscriptionID *uint         `json:"subscription_id" gorm:"index"`
	Subscription   *Subscription `json:"subscription,omitempty" gorm:"foreignKey:SubscriptionID;constraint:-"`
	AssignmentID   *uint         `json:"assignment_id" gorm:"index"`
	Assignment     *Assignment   `json:"assignment,omitempty" gorm:"foreignKey:AssignmentID;constraint:-"`

	// День использования
	UsageDate time.Time `json:"usage_date" gorm:"not null;index"` // Дата в формате YYYY-MM-DD

	// Количество использования
	Quantity decimal.Decimal `json:"quantity" gorm:"type:decimal(10,3);default:1.000"` // Количество единиц

	// Дополнительная информация
	Metadata string `json:"metadata" gorm:"type:jsonb"` // Дополнительные данные в JSON
}

// TableName задает имя таблицы для модели Usage
func (Usage) TableName() string {
	return "usages"
}

// Discount представляет скидку на различных уровнях иерархии
type Discount struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Уровень применения скидки
	Level    string `json:"level" gorm:"not null;type:varchar(50)"` // object, tariff, subscription, appendix, contract
	EntityID uint   `json:"entity_id" gorm:"not null;index"`        // ID сущности на соответствующем уровне

	// Тип и размер скидки
	Type  string          `json:"type" gorm:"not null;type:varchar(20)"`    // fixed (фиксированная сумма) или percent (%)
	Value decimal.Decimal `json:"value" gorm:"not null;type:decimal(15,2)"` // Значение скидки

	// Период действия
	StartDate time.Time  `json:"start_date" gorm:"not null"`
	EndDate   *time.Time `json:"end_date"` // NULL = бессрочно

	// Статус
	IsActive bool   `json:"is_active" gorm:"default:true"`
	Reason   string `json:"reason" gorm:"type:text"` // Причина скидки
}

// TableName задает имя таблицы для модели Discount
func (Discount) TableName() string {
	return "discounts"
}

// InvoiceHeader представляет заголовок счета (старая структура из roadmap)
// Используется для совместимости со старым биллингом
type InvoiceHeader struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	Number  string    `json:"number" gorm:"uniqueIndex;not null;type:varchar(50)"`
	Date    time.Time `json:"date" gorm:"not null"`
	DueDate time.Time `json:"due_date" gorm:"not null"`

	// Связи
	CompanyID      uint          `json:"company_id" gorm:"not null;index"`
	Company        *Company      `json:"company,omitempty" gorm:"foreignKey:CompanyID;constraint:-"`
	ContractID     *uint         `json:"contract_id" gorm:"index"`
	Contract       *Contract     `json:"contract,omitempty" gorm:"foreignKey:ContractID;constraint:-"`
	SubscriptionID *uint         `json:"subscription_id" gorm:"index"`
	Subscription   *Subscription `json:"subscription,omitempty" gorm:"foreignKey:SubscriptionID;constraint:-"`

	// Период биллинга
	PeriodStart time.Time `json:"period_start" gorm:"not null"`
	PeriodEnd   time.Time `json:"period_end" gorm:"not null"`

	// Финансы
	SubtotalAmount decimal.Decimal `json:"subtotal_amount" gorm:"type:decimal(15,2);not null"`
	TaxAmount      decimal.Decimal `json:"tax_amount" gorm:"type:decimal(15,2);default:0.00"`
	TotalAmount    decimal.Decimal `json:"total_amount" gorm:"type:decimal(15,2);not null"`
	Currency       string          `json:"currency" gorm:"default:'RUB';type:varchar(3)"`

	// Страны и налоги
	SellerCountryCode string          `json:"seller_country_code" gorm:"type:varchar(2)"`
	BuyerCountryCode  string          `json:"buyer_country_code" gorm:"type:varchar(2)"`
	TaxRate           decimal.Decimal `json:"tax_rate" gorm:"type:decimal(5,2);default:0.00"`

	// Статус
	Status     string          `json:"status" gorm:"default:'draft';type:varchar(20)"` // draft, sent, paid, overdue, cancelled
	PaidAt     *time.Time      `json:"paid_at"`
	PaidAmount decimal.Decimal `json:"paid_amount" gorm:"type:decimal(15,2);default:0.00"`
}

// TableName задает имя таблицы для модели InvoiceHeader
func (InvoiceHeader) TableName() string {
	return "invoice_headers"
}

// InvoiceLine представляет строку счета (старая структура из roadmap)
type InvoiceLine struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связь со счетом
	InvoiceHeaderID uint           `json:"invoice_header_id" gorm:"not null;index"`
	InvoiceHeader   *InvoiceHeader `json:"invoice_header,omitempty" gorm:"foreignKey:InvoiceHeaderID;constraint:-"`

	// Позиция
	LineNumber int             `json:"line_number" gorm:"not null"`
	Name       string          `json:"name" gorm:"not null;type:varchar(200)"`
	Quantity   decimal.Decimal `json:"quantity" gorm:"type:decimal(10,3);not null"`
	UnitPrice  decimal.Decimal `json:"unit_price" gorm:"type:decimal(15,2);not null"`
	Amount     decimal.Decimal `json:"amount" gorm:"type:decimal(15,2);not null"`

	// Связи
	TariffComponentID *uint            `json:"tariff_component_id" gorm:"index"`
	TariffComponent   *TariffComponent `json:"tariff_component,omitempty" gorm:"foreignKey:TariffComponentID;constraint:-"`
	ObjectID          *uint            `json:"object_id" gorm:"index"`
	Object            *Object          `json:"object,omitempty" gorm:"foreignKey:ObjectID;constraint:-"`

	// Период для позиции
	PeriodStart *time.Time `json:"period_start"`
	PeriodEnd   *time.Time `json:"period_end"`
}

// TableName задает имя таблицы для модели InvoiceLine
func (InvoiceLine) TableName() string {
	return "invoice_lines"
}

// InvoiceSequence представляет последовательность нумерации счетов по странам
type InvoiceSequence struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Параметры нумерации
	CountryCode string `json:"country_code" gorm:"not null;type:varchar(2);uniqueIndex:idx_country_prefix;index"` // Код страны (RU, KZ и т.д.)
	Prefix      string `json:"prefix" gorm:"not null;type:varchar(20);uniqueIndex:idx_country_prefix"`            // Префикс номера (например, "INV", "СЧТ")
	Pattern     string `json:"pattern" gorm:"not null;type:varchar(50);default:'%s-%04d'"`                        // Шаблон номера (например, "%s-%04d" для "INV-0001")

	// Текущее состояние
	LastNumber int `json:"last_number" gorm:"default:0;not null"` // Последний использованный номер

	// Дополнительная информация
	Description string `json:"description" gorm:"type:text"`  // Описание последовательности
	IsActive    bool   `json:"is_active" gorm:"default:true"` // Активна ли последовательность
}

// TableName задает имя таблицы для модели InvoiceSequence
func (InvoiceSequence) TableName() string {
	return "invoice_sequences"
}
