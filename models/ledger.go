package models

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// LedgerEntry — неизменяемая проводка лицевого счёта (единый источник правды баланса).
// Баланс договора = SUM(amount) по всем проводкам (deleted_at IS NULL).
// Знак amount: платёж/пополнение/перенос остатка > 0, начисление (charge) < 0.
// balance > 0 — переплата (клиент в плюсе), balance < 0 — долг.
//
// Глобальная (public) таблица — как invoices/billing_history. Ключи admin/company/contract,
// без FK на tenant-таблицы (contracts в tenant-схеме), чтобы платежи/1С/банки/отчёты
// не пересекали схемы.
//
// Проводки ИММУТАБЕЛЬНЫ: правка/отмена — через reversal-проводку (entry_type=reversal),
// не UPDATE/DELETE. deleted_at — только для технического отката импорта.
type LedgerEntry struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	AdminAccountID uint  `json:"admin_account_id" gorm:"not null;index"`
	CompanyID      uint  `json:"company_id" gorm:"not null;index"`
	ContractID     uint  `json:"contract_id" gorm:"not null;index"`
	SubscriptionID *uint `json:"subscription_id" gorm:"index"`
	ObjectID       *uint `json:"object_id" gorm:"index"`

	// charge | payment | adjustment | reversal | migration_balance
	EntryType string          `json:"entry_type" gorm:"not null;type:varchar(30);index"`
	Amount    decimal.Decimal `json:"amount" gorm:"not null;type:decimal(15,2)"` // знаковый: payment(+), charge(−)
	Currency  string          `json:"currency" gorm:"not null;default:'RUB';type:varchar(3)"`

	// manual | excel | 1c | payment_system | bank | migration | auto_charge
	Source     string `json:"source" gorm:"not null;type:varchar(30);index"`
	ExternalID string `json:"external_id" gorm:"type:varchar(128)"` // идемпотентность импорта

	Description string    `json:"description" gorm:"type:text"`
	EntryDate   time.Time `json:"entry_date" gorm:"not null;index"` // дата операции (может отличаться от created_at)
	CreatedBy   string    `json:"created_by" gorm:"type:varchar(100)"`

	// Для reversal: ссылка на сторнируемую проводку.
	ReversalOfID *uint   `json:"reversal_of_id" gorm:"index"`
	Metadata     *string `json:"metadata" gorm:"type:jsonb"` // nil → NULL (пустая строка невалидна для jsonb)
}

func (LedgerEntry) TableName() string { return "ledger_entries" }

// LedgerTransfer — заголовок перевода средств между лицевыми счетами договоров.
// Связывает пару проводок ledger_entries (transfer_out у from, transfer_in у to),
// созданных в одной транзакции. Reversal перевода — только парой (по transfer_id).
type LedgerTransfer struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index"`
	CompanyID      uint `json:"company_id" gorm:"not null;index"`

	TransferID     string          `json:"transfer_id" gorm:"not null;uniqueIndex;type:varchar(64)"`                                          // UUID, связь пары проводок
	IdempotencyKey string          `json:"idempotency_key" gorm:"uniqueIndex:idx_transfer_idem,where:idempotency_key <> '';type:varchar(64)"` // клиентский ключ против дублей retry
	FromContractID uint            `json:"from_contract_id" gorm:"not null;index"`
	ToContractID   uint            `json:"to_contract_id" gorm:"not null;index"`
	Amount         decimal.Decimal `json:"amount" gorm:"not null;type:decimal(15,2)"` // сумма списания у источника (в Currency)
	Currency       string          `json:"currency" gorm:"not null;default:'RUB';type:varchar(3)"`

	// Кросс-валютный перевод (П5 фаза 3): если валюта источника ≠ валюте получателя,
	// сумма конвертится по курсу. ToAmount/ToCurrency — что зачислено получателю.
	// Для одновалютного перевода ToAmount=Amount, ToCurrency=Currency, Rate=1.
	ToAmount   decimal.Decimal `json:"to_amount" gorm:"type:decimal(15,2)"`           // сумма зачисления получателю (в ToCurrency)
	ToCurrency string          `json:"to_currency" gorm:"type:varchar(3)"`           // валюта получателя
	Rate       decimal.Decimal `json:"rate" gorm:"type:decimal(18,8)"`               // применённый курс from→to (1 при одной валюте)
	RateSource string          `json:"rate_source" gorm:"type:varchar(20)"`          // источник курса (cbr_rf|nbk_kz), пусто при одной валюте

	Status      string `json:"status" gorm:"not null;default:'completed';type:varchar(20)"` // completed | reversed
	Description string `json:"description" gorm:"type:text"`
	CreatedBy   string `json:"created_by" gorm:"type:varchar(100)"`
}

func (LedgerTransfer) TableName() string { return "ledger_transfers" }

// BillingSuspension — приостановка договора (П2). Отдельная сущность, НЕ перетираем
// Contract.Status напрямую: храним причину + предыдущий статус, чтобы авто-разблок
// при погашении долга вернул именно прежний статус и не задел ручной cancel/expired.
// Авто-разблок касается только reason='billing_debt'.
type BillingSuspension struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Партиальный uniqueIndex: не более одной АКТИВНОЙ debt-приостановки на договор
	// (анти-дубль при гонке двух sweep — Codex H3).
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index;uniqueIndex:idx_susp_active_debt,where:active AND deleted_at IS NULL AND reason = 'billing_debt'"`
	CompanyID      uint `json:"company_id" gorm:"not null;index;uniqueIndex:idx_susp_active_debt"`
	ContractID     uint `json:"contract_id" gorm:"not null;index;uniqueIndex:idx_susp_active_debt"`

	Reason         string          `json:"reason" gorm:"not null;type:varchar(30);index"` // billing_debt | manual
	PreviousStatus string          `json:"previous_status" gorm:"type:varchar(20)"`       // статус договора до приостановки
	DebtAmount     decimal.Decimal `json:"debt_amount" gorm:"type:decimal(15,2)"`         // долг на момент блокировки
	Active         bool            `json:"active" gorm:"not null;default:true;index"`
	SuspendedBy    string          `json:"suspended_by" gorm:"type:varchar(100)"` // scheduler | username
	ResolvedAt     *time.Time      `json:"resolved_at"`
}

func (BillingSuspension) TableName() string { return "billing_suspensions" }

// BillingHold — «зонт» над договором, который временно блокирует авто-приостановку
// за долг (П3 отсрочка + П4 обещанный платёж). НЕ трогает баланс: ledger остаётся
// единственным источником правды. Hold лишь говорит sweep'у «не блокируй до HoldUntil».
//
// Два типа (HoldType):
//   - deferral — отсрочка платежа: держим до даты, сумма не важна (Amount=0);
//   - promise  — обещанный платёж: клиент обещал внести Amount до HoldUntil.
//
// Lifecycle (Status): active → fulfilled (вышел из долга) | expired (срок истёк, долг
// остался) | cancelled (оператор отменил). Sweep гасит expired и снимает зонт при fulfilled.
//
// Глобальная (public) таблица — как ledger_entries / billing_suspensions (ключи
// admin/company/contract, без FK на tenant-схему).
type BillingHold struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Партиальный uniqueIndex: не более одного АКТИВНОГО hold на договор
	// (анти-дубль при гонке двух операторов / двух sweep — зеркало BillingSuspension).
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index;uniqueIndex:idx_hold_active,where:active AND deleted_at IS NULL"`
	CompanyID      uint `json:"company_id" gorm:"not null;index;uniqueIndex:idx_hold_active"`
	ContractID     uint `json:"contract_id" gorm:"not null;index;uniqueIndex:idx_hold_active"`

	HoldType string          `json:"hold_type" gorm:"not null;type:varchar(20);index"` // deferral | promise
	Amount   decimal.Decimal `json:"amount" gorm:"type:decimal(15,2);default:0"`       // promise: обещанная сумма; deferral: 0
	Currency string          `json:"currency" gorm:"not null;default:'RUB';type:varchar(3)"`

	HoldUntil time.Time `json:"hold_until" gorm:"not null;index"`                         // до какой даты зонт держит
	Status    string    `json:"status" gorm:"not null;default:'active';type:varchar(20);index"` // active | fulfilled | expired | cancelled
	Active    bool      `json:"active" gorm:"not null;default:true;index"`               // = (status==active), для partial-index

	DebtAtCreate decimal.Decimal `json:"debt_at_create" gorm:"type:decimal(15,2);default:0"` // долг на момент создания (отчёт)
	Reason       string          `json:"reason" gorm:"type:text"`
	CreatedBy    string          `json:"created_by" gorm:"type:varchar(100)"`
	ResolvedAt   *time.Time      `json:"resolved_at"` // когда стал fulfilled/expired/cancelled
}

func (BillingHold) TableName() string { return "billing_holds" }
