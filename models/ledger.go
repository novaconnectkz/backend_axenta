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

	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index"`
	CompanyID      uint `json:"company_id" gorm:"not null;index"`
	ContractID     uint `json:"contract_id" gorm:"not null;index"`
	// Денормализованный контрагент (Ф1): баланс агрегируется per-контрагент без cross-schema
	// JOIN (contracts в tenant, ledger в public). Backfill datafix'ом; 0 до проставления.
	// Асимметрия: charge остаётся per-договор (ContractID), баланс/платёж — per-контрагент (Ф2).
	// not null;default:0 — «не размечен» = 0 (не NULL), иначе на проде AutoMigrate создаёт
	// nullable-колонку и backfill (`counterparty_id = 0`) пропускает NULL-строки.
	CounterpartyID uint  `json:"counterparty_id" gorm:"index;not null;default:0"`
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

	// Ф5: батч Excel-импорта (для отката пачкой). 0 — проводка не из импорт-батча.
	ImportBatchID uint `json:"import_batch_id" gorm:"index;not null;default:0"`

	// Б0 (per-договор аллокатор, read-only): логический период проводки для FIFO-упорядочивания
	// долгов. НЕ entry_date — postpaid entry_date «убегает» на период вперёд. Заполняется
	// write-side позже; пока аллокатор резолвит из external_id (autocharge-форматы). Аддитивные
	// nullable-поля: ничего не читает их в проде до Б4. concepts/wallet-percontract-gating.
	PeriodStart       *time.Time `json:"period_start" gorm:"index"`
	PeriodGranularity string     `json:"period_granularity" gorm:"type:varchar(10)"` // day|month|year
	PeriodKeySource   string     `json:"period_key_source" gorm:"type:varchar(20)"`  // persisted|external_id|entry_date
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
	ToAmount   decimal.Decimal `json:"to_amount" gorm:"type:decimal(15,2)"` // сумма зачисления получателю (в ToCurrency)
	ToCurrency string          `json:"to_currency" gorm:"type:varchar(3)"`  // валюта получателя
	Rate       decimal.Decimal `json:"rate" gorm:"type:decimal(18,8)"`      // применённый курс from→to (1 при одной валюте)
	RateSource string          `json:"rate_source" gorm:"type:varchar(20)"` // источник курса (cbr_rf|nbk_kz), пусто при одной валюте

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

	// Ф3/Б3: debt-приостановка. Дуальный партиальный uniqueIndex (анти-дубль при гонке sweep):
	//   - idx_susp_debt_cp_ct: cp<>0 → ключ (admin,company,counterparty_id,contract_id);
	//     Б3 добавил contract_id в ключ → допускает строку НА КАЖДЫЙ договор cp (per-договор
	//     блокировка Б4). До Б4 sweep пишет ContractID=0 → ключ (cp,0) = по-прежнему 1 на cp.
	//   - idx_susp_debt_legacy: cp=0 → legacy per-договор (admin,company,contract_id).
	// Manual-приостановки (reason='manual') под uniq НЕ попадают (WHERE reason='billing_debt').
	// priority задаёт порядок колонок struct-tag индекса = порядку explicit-DDL
	// (ensureBillingSchemaIntegrity) — иначе свежая и мигрированная БД разошлись бы по порядку.
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index;uniqueIndex:idx_susp_debt_cp_ct,priority:1,where:active AND deleted_at IS NULL AND reason = 'billing_debt' AND counterparty_id <> 0;uniqueIndex:idx_susp_debt_legacy,where:active AND deleted_at IS NULL AND reason = 'billing_debt' AND counterparty_id = 0"`
	CompanyID      uint `json:"company_id" gorm:"not null;index;uniqueIndex:idx_susp_debt_cp_ct,priority:2;uniqueIndex:idx_susp_debt_legacy"`
	ContractID     uint `json:"contract_id" gorm:"not null;index;uniqueIndex:idx_susp_debt_legacy;uniqueIndex:idx_susp_debt_cp_ct,priority:4"`
	CounterpartyID uint `json:"counterparty_id" gorm:"not null;default:0;index;uniqueIndex:idx_susp_debt_cp_ct,priority:3"`

	Reason         string `json:"reason" gorm:"not null;type:varchar(30);index"` // billing_debt | manual
	PreviousStatus string `json:"previous_status" gorm:"type:varchar(20)"`       // статус договора до приостановки
	// Ф3: CSV id договоров, которые ИМЕННО ЭТА debt-строка перевела active→suspended.
	// Нужен для точного restore: при погашении долга вернуть в active только эти договоры,
	// не трогая приостановленные по др. причине (нет подписок/ручной bulk). cp-level строка
	// гасит N договоров — без списка resolve вслепую поднял бы чужие приостановки.
	AffectedContractIDs string          `json:"affected_contract_ids" gorm:"type:text"`
	DebtAmount          decimal.Decimal `json:"debt_amount" gorm:"type:decimal(15,2)"` // долг на момент блокировки
	Active              bool            `json:"active" gorm:"not null;default:true;index"`
	SuspendedBy         string          `json:"suspended_by" gorm:"type:varchar(100)"` // scheduler | username
	ResolvedAt          *time.Time      `json:"resolved_at"`
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

	// Ф3: зонт per-КОНТРАГЕНТ (один активный зонт на контрагента блокирует приостановку всех
	// его договоров). Дуальный партиальный uniqueIndex (зеркало BillingSuspension):
	//   - idx_hold_cp: cp<>0 → (admin,company,counterparty_id), ContractID=0;
	//   - idx_hold_legacy: cp=0 → legacy per-договор (admin,company,contract_id).
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index;uniqueIndex:idx_hold_cp,where:active AND deleted_at IS NULL AND counterparty_id <> 0;uniqueIndex:idx_hold_legacy,where:active AND deleted_at IS NULL AND counterparty_id = 0"`
	CompanyID      uint `json:"company_id" gorm:"not null;index;uniqueIndex:idx_hold_cp;uniqueIndex:idx_hold_legacy"`
	ContractID     uint `json:"contract_id" gorm:"not null;index;uniqueIndex:idx_hold_legacy"`
	CounterpartyID uint `json:"counterparty_id" gorm:"not null;default:0;index;uniqueIndex:idx_hold_cp"`

	HoldType string          `json:"hold_type" gorm:"not null;type:varchar(20);index"` // deferral | promise
	Amount   decimal.Decimal `json:"amount" gorm:"type:decimal(15,2);default:0"`       // promise: обещанная сумма; deferral: 0
	Currency string          `json:"currency" gorm:"not null;default:'RUB';type:varchar(3)"`

	HoldUntil time.Time `json:"hold_until" gorm:"not null;index"`                               // до какой даты зонт держит
	Status    string    `json:"status" gorm:"not null;default:'active';type:varchar(20);index"` // active | fulfilled | expired | cancelled
	Active    bool      `json:"active" gorm:"not null;default:true;index"`                      // = (status==active), для partial-index

	DebtAtCreate decimal.Decimal `json:"debt_at_create" gorm:"type:decimal(15,2);default:0"` // долг на момент создания (отчёт)
	Reason       string          `json:"reason" gorm:"type:text"`
	CreatedBy    string          `json:"created_by" gorm:"type:varchar(100)"`
	ResolvedAt   *time.Time      `json:"resolved_at"` // когда стал fulfilled/expired/cancelled
}

func (BillingHold) TableName() string { return "billing_holds" }

// LedgerImportBatch — заголовок батча Excel-импорта платежей (Ф5). Группирует payment-проводки
// одного импорта для отката пачкой (reversal). Платежи матчатся на КОНТРАГЕНТА (единый ЛС):
// проводки получают counterparty_id (contract_id=0, уровень контрагента) + import_batch_id.
//
// Глобальная (public) таблица — как ledger_entries. Откат: reversal-проводки на каждую
// payment-проводку батча; status → reversed (полный) либо остаётся imported (выборочный).
type LedgerImportBatch struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index"`
	CompanyID      uint `json:"company_id" gorm:"not null;index"`

	Source       string          `json:"source" gorm:"not null;type:varchar(30);default:'excel'"`          // excel|1c|bank|payment_system
	Status       string          `json:"status" gorm:"not null;type:varchar(20);default:'imported';index"` // imported|reversed
	RowsTotal    int             `json:"rows_total"`
	RowsImported int             `json:"rows_imported"`
	RowsSkipped  int             `json:"rows_skipped"` // дубли по идемпотентности
	TotalAmount  decimal.Decimal `json:"total_amount" gorm:"type:decimal(15,2)"`
	Currency     string          `json:"currency" gorm:"not null;default:'RUB';type:varchar(3)"`
	FileName     string          `json:"file_name" gorm:"type:varchar(255)"`
	CreatedBy    string          `json:"created_by" gorm:"type:varchar(100)"`
	ReversedAt   *time.Time      `json:"reversed_at"`
	ReversedBy   string          `json:"reversed_by" gorm:"type:varchar(100)"`
}

func (LedgerImportBatch) TableName() string { return "ledger_import_batches" }

// BillingEnforcementAction — B1: per-target запись физической блокировки/разблокировки
// учётки в GPS-системе (МЕТОД 1 — учётка целиком). Строка на (suspension, system, account).
// Отдельная таблица (не JSON в suspension): нужны per-target строки для идемпотентного
// re-assert (physical_ok=false) и различения billing-блока от ручной деактивации оператором.
// SHADOW-first: при mode='shadow' строка пишется (physical_ok=false), реальный API НЕ вызывается.
// Подробности и blast-radius — wiki/concepts/billing-enforcement-layer.
type BillingEnforcementAction struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Цель: одна активная строка на (suspension, system, connection, account).
	SuspensionID      uint   `json:"suspension_id" gorm:"not null;index;uniqueIndex:idx_enf_target"`
	CompanyID         uint   `json:"company_id" gorm:"not null;index"`
	Level             string `json:"level" gorm:"type:varchar(10);not null;default:'account'"` // B1: всегда 'account' (МЕТОД 2 не делаем)
	System            string `json:"system" gorm:"type:varchar(20);not null;uniqueIndex:idx_enf_target"`
	ConnectionID      uint   `json:"connection_id" gorm:"not null;default:0;uniqueIndex:idx_enf_target"`
	ExternalAccountID string `json:"external_account_id" gorm:"type:varchar(128);not null;uniqueIndex:idx_enf_target"`

	Action          string     `json:"action" gorm:"type:varchar(10);not null"`         // block | unblock
	Mode            string     `json:"mode" gorm:"type:varchar(10);not null"`           // shadow | live
	PhysicalOK      bool       `json:"physical_ok" gorm:"not null;default:false;index"` // true ТОЛЬКО после реального успеха в live
	ManualOverride  bool       `json:"manual_override" gorm:"not null;default:false"`   // внешнее состояние расходится с решением → не авто-реверсить
	LastError       string     `json:"last_error" gorm:"type:text"`
	DecisionVersion int        `json:"decision_version" gorm:"not null;default:1"`
	ForeignObjects  int        `json:"foreign_objects" gorm:"default:0"` // чужие объекты той же учётки (blast radius)
	CacheMismatch   bool       `json:"cache_mismatch" gorm:"default:false"`
	EnforcedAt      *time.Time `json:"enforced_at"`
}

func (BillingEnforcementAction) TableName() string { return "billing_enforcement_actions" }
