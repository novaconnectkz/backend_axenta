package models

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Invoice представляет счет в системе биллинга
type Invoice struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля счета
	Number      string    `json:"number" gorm:"uniqueIndex;not null;type:varchar(50)"`
	Title       string    `json:"title" gorm:"not null;type:varchar(200)"`
	Description string    `json:"description" gorm:"type:text"`
	InvoiceDate time.Time `json:"invoice_date" gorm:"not null"`
	DueDate     time.Time `json:"due_date" gorm:"not null"`

	// Связи
	AdminAccountID uint        `json:"admin_account_id" gorm:"not null;index"`
	CompanyID      uint        `json:"company_id" gorm:"not null;index"`
	ContractID     *uint       `json:"contract_id" gorm:"index"` // Может быть null для общих счетов
	Contract       *Contract   `json:"contract,omitempty" gorm:"foreignKey:ContractID"`
	TariffPlanID   uint        `json:"tariff_plan_id" gorm:"not null"`
	TariffPlan     *TariffPlan `json:"tariff_plan,omitempty" gorm:"foreignKey:TariffPlanID"`

	// Период биллинга
	BillingPeriodStart time.Time `json:"billing_period_start" gorm:"not null"`
	BillingPeriodEnd   time.Time `json:"billing_period_end" gorm:"not null"`

	// Финансовая информация
	SubtotalAmount decimal.Decimal `json:"subtotal_amount" gorm:"type:decimal(15,2);not null"`
	TaxRate        decimal.Decimal `json:"tax_rate" gorm:"type:decimal(5,2);default:0"`     // НДС в процентах
	TaxAmount      decimal.Decimal `json:"tax_amount" gorm:"type:decimal(15,2);default:0"`  // Сумма НДС
	TotalAmount    decimal.Decimal `json:"total_amount" gorm:"type:decimal(15,2);not null"` // Итоговая сумма
	Currency       string          `json:"currency" gorm:"default:'RUB';type:varchar(3)"`

	// Статус счета
	Status     string          `json:"status" gorm:"default:'draft';type:varchar(20)"`  // draft, sent, paid, overdue, cancelled
	PaidAt     *time.Time      `json:"paid_at"`                                         // Дата оплаты
	PaidAmount decimal.Decimal `json:"paid_amount" gorm:"type:decimal(15,2);default:0"` // Оплаченная сумма

	// Дополнительная информация
	Notes      string `json:"notes" gorm:"type:text"`
	ExternalID string `json:"external_id" gorm:"type:varchar(100)"` // ID во внешних системах

	// Связанные позиции счета
	Items []InvoiceItem `json:"items,omitempty" gorm:"foreignKey:InvoiceID"`
}

// TableName задает имя таблицы для модели Invoice
func (Invoice) TableName() string {
	return "invoices"
}

// IsOverdue проверяет, просрочен ли счет
func (i *Invoice) IsOverdue() bool {
	return i.Status != "paid" && i.Status != "cancelled" && time.Now().After(i.DueDate)
}

// GetRemainingAmount возвращает оставшуюся к доплате сумму
func (i *Invoice) GetRemainingAmount() decimal.Decimal {
	return i.TotalAmount.Sub(i.PaidAmount)
}

// IsFullyPaid проверяет, полностью ли оплачен счет
func (i *Invoice) IsFullyPaid() bool {
	return i.PaidAmount.GreaterThanOrEqual(i.TotalAmount)
}

// InvoiceItem представляет позицию в счете
type InvoiceItem struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связь с счетом
	InvoiceID uint    `json:"invoice_id" gorm:"not null;index"`
	Invoice   Invoice `json:"invoice,omitempty" gorm:"foreignKey:InvoiceID"`

	// Основные поля позиции
	Name        string `json:"name" gorm:"not null;type:varchar(200)"`
	Description string `json:"description" gorm:"type:text"`
	ItemType    string `json:"item_type" gorm:"not null;type:varchar(50)"` // subscription, object, setup, discount

	// Связи с объектами (для позиций по объектам)
	ObjectID *uint   `json:"object_id" gorm:"index"`
	Object   *Object `json:"object,omitempty" gorm:"foreignKey:ObjectID"`

	// Количество и цены
	Quantity  decimal.Decimal `json:"quantity" gorm:"type:decimal(10,3);not null"`
	UnitPrice decimal.Decimal `json:"unit_price" gorm:"type:decimal(15,2);not null"`
	Amount    decimal.Decimal `json:"amount" gorm:"type:decimal(15,2);not null"`

	// Период для позиции (если применимо)
	PeriodStart *time.Time `json:"period_start"`
	PeriodEnd   *time.Time `json:"period_end"`

	// Дополнительная информация
	Notes string `json:"notes" gorm:"type:text"`
}

// TableName задает имя таблицы для модели InvoiceItem
func (InvoiceItem) TableName() string {
	return "invoice_items"
}

// BillingHistory представляет историю биллинговых операций
type BillingHistory struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связи
	AdminAccountID uint      `json:"admin_account_id" gorm:"not null;index"`
	CompanyID      uint      `json:"company_id" gorm:"not null;index"`
	InvoiceID      *uint     `json:"invoice_id" gorm:"index"`
	Invoice        *Invoice  `json:"invoice,omitempty" gorm:"foreignKey:InvoiceID"`
	ContractID     *uint     `json:"contract_id" gorm:"index"`
	Contract       *Contract `json:"contract,omitempty" gorm:"foreignKey:ContractID"`

	// Информация об операции
	Operation   string          `json:"operation" gorm:"not null;type:varchar(50)"` // invoice_created, payment_received, invoice_cancelled
	Amount      decimal.Decimal `json:"amount" gorm:"type:decimal(15,2)"`
	Currency    string          `json:"currency" gorm:"default:'RUB';type:varchar(3)"`
	Description string          `json:"description" gorm:"type:text"`

	// Период операции
	PeriodStart *time.Time `json:"period_start"`
	PeriodEnd   *time.Time `json:"period_end"`

	// Дополнительные данные (JSON)
	Metadata string `json:"metadata" gorm:"type:jsonb"`

	// Статус операции
	Status string `json:"status" gorm:"default:'completed';type:varchar(20)"` // pending, completed, failed, cancelled
}

// TableName задает имя таблицы для модели BillingHistory
func (BillingHistory) TableName() string {
	return "billing_history"
}

// BillingSettings представляет настройки биллинга для компании
type BillingSettings struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связь с компанией
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index"`
	CompanyID      uint `json:"company_id" gorm:"not null;index"`

	// Настройки генерации счетов
	AutoGenerateInvoices   bool `json:"auto_generate_invoices" gorm:"default:true"`
	InvoiceGenerationDay   int  `json:"invoice_generation_day" gorm:"default:1"`     // День месяца для генерации (1-28)
	InvoicePaymentTermDays int  `json:"invoice_payment_term_days" gorm:"default:14"` // Срок оплаты в днях

	// Настройки налогов
	DefaultTaxRate decimal.Decimal `json:"default_tax_rate" gorm:"type:decimal(5,2);default:20"` // НДС по умолчанию
	TaxIncluded    bool            `json:"tax_included" gorm:"default:false"`                    // НДС включен в цену
	VATRatePreset  string          `json:"vat_rate_preset" gorm:"type:varchar(20);default:'russia'"` // Пресет ставки НДС: russia, kazakhstan, none, custom
	VATRateCustom  decimal.Decimal `json:"vat_rate_custom" gorm:"type:decimal(5,2);default:20"` // Своя ставка НДС (используется при VATRatePreset = custom)

	// Настройки уведомлений
	NotifyBeforeInvoice int `json:"notify_before_invoice" gorm:"default:3"` // За сколько дней уведомлять о выставлении счета
	NotifyBeforeDue     int `json:"notify_before_due" gorm:"default:3"`     // За сколько дней уведомлять о сроке оплаты
	NotifyOverdue       int `json:"notify_overdue" gorm:"default:1"`        // Через сколько дней уведомлять о просрочке

	// Настройки форматирования
	InvoiceNumberPrefix string `json:"invoice_number_prefix" gorm:"default:'INV';type:varchar(10)"`     // Префикс номера счета
	InvoiceNumberFormat string `json:"invoice_number_format" gorm:"default:'%s-%04d';type:varchar(20)"` // Формат номера счета

	// Настройки нумерации договоров
	ContractNumberingMethod    string `json:"contract_numbering_method" gorm:"default:'manual';type:varchar(20)"` // manual, numerator, bitrix24
	ContractDefaultNumeratorID *uint  `json:"contract_default_numerator_id" gorm:"index"`                         // ID нумератора по умолчанию
	Bitrix24DealNumberField    string `json:"bitrix24_deal_number_field" gorm:"type:varchar(50)"`                 // Код поля номера договора в Bitrix24 (например, UF_CRM_CONTRACT_NUMBER)

	// Дополнительные настройки
	Currency              string `json:"currency" gorm:"default:'RUB';type:varchar(3)"`
	DefaultPaymentMethod  string `json:"default_payment_method" gorm:"type:varchar(50)"`
	AllowPartialPayments  bool   `json:"allow_partial_payments" gorm:"default:true"`
	RequirePaymentConfirm bool   `json:"require_payment_confirm" gorm:"default:false"`

	// Настройки для льготных тарифов
	EnableInactiveDiscounts bool            `json:"enable_inactive_discounts" gorm:"default:true"`
	InactiveDiscountRatio   decimal.Decimal `json:"inactive_discount_ratio" gorm:"type:decimal(3,2);default:0.5"`
}

// TableName задает имя таблицы для модели BillingSettings
func (BillingSettings) TableName() string {
	return "billing_settings"
}

// GetInvoiceNumber генерирует номер счета
func (bs *BillingSettings) GetInvoiceNumber(sequenceNumber int) string {
	if bs.InvoiceNumberFormat == "" {
		bs.InvoiceNumberFormat = "%s-%04d"
	}
	if bs.InvoiceNumberPrefix == "" {
		bs.InvoiceNumberPrefix = "INV"
	}

	// Можно добавить год/месяц в формат
	year := time.Now().Year()
	month := int(time.Now().Month())

	// Формат: PREFIX-YYYY-MM-NNNN
	return fmt.Sprintf("%s-%d-%02d-%04d", bs.InvoiceNumberPrefix, year, month, sequenceNumber)
}

// InvoiceNumerator представляет нумератор счетов в системе
type InvoiceNumerator struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связь с администратором и компанией
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index"`
	CompanyID      uint `json:"company_id" gorm:"not null;index"`

	// Основные поля нумератора
	Name        string `json:"name" gorm:"not null;type:varchar(100)"`     // Название нумератора
	Prefix      string `json:"prefix" gorm:"not null;type:varchar(10)"`    // Префикс (например "INV")
	Template    string `json:"template" gorm:"not null;type:varchar(200)"` // Шаблон номера (например "{PREFIX}-{YEAR}-{MONTH}-{SEQ}")
	Description string `json:"description" gorm:"type:text"`               // Описание нумератора

	// Счетчик для последовательных номеров
	CounterValue int `json:"counter_value" gorm:"default:0"` // Текущее значение счетчика

	// Настройки
	IsDefault   bool   `json:"is_default" gorm:"default:false"`      // Нумератор по умолчанию
	IsActive    bool   `json:"is_active" gorm:"default:true"`        // Активен ли нумератор
	AutoReset   bool   `json:"auto_reset" gorm:"default:false"`      // Автоматически сбрасывать счетчик
	ResetPeriod string `json:"reset_period" gorm:"type:varchar(20)"` // Период сброса: yearly, monthly, never

	// Дополнительные поля
	Notes string `json:"notes" gorm:"type:text"`
}

// TableName задает имя таблицы для модели InvoiceNumerator
func (InvoiceNumerator) TableName() string {
	return "invoice_numerators"
}
