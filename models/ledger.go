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
	Amount         decimal.Decimal `json:"amount" gorm:"not null;type:decimal(15,2)"` // положительная сумма перевода
	Currency       string          `json:"currency" gorm:"not null;default:'RUB';type:varchar(3)"`
	Status         string          `json:"status" gorm:"not null;default:'completed';type:varchar(20)"` // completed | reversed
	Description    string          `json:"description" gorm:"type:text"`
	CreatedBy      string          `json:"created_by" gorm:"type:varchar(100)"`
}

func (LedgerTransfer) TableName() string { return "ledger_transfers" }
