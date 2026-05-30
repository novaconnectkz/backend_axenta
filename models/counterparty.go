package models

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Counterparty — контрагент: единый владелец N договоров и ОДНОГО лицевого счёта.
// Бизнес-модель: один контрагент = N договоров = ОДИН баланс (SUM по counterparty_id).
//
// Идентичность — отдельная сущность (НЕ группировка по client_inn: опечатки=дубли,
// договоры без ИНН смешиваются). Идентификатор гибкий и страно/тип-зависимый:
// РФ=ИНН (id_type=inn), KZ=БИН орг/ИИН ИП-физ (bin|iin), физлица=паспорт (passport),
// прочее=other. tax_id опционален — при пустом контрагент идентифицируется по имени
// (manual_review=true для последующей ручной проверки).
//
// Глобальная (public) таблица — как ledger_entries/invoices. Ключи admin/company, без FK
// на tenant-таблицы (contracts в tenant-схеме), чтобы баланс агрегировался без cross-schema
// JOIN. Contract.counterparty_id и LedgerEntry.counterparty_id ссылаются сюда по id (без FK).
//
// Ф1 (аддитивно): таблица + backfill из договоров. Денорм-поля Client* на Contract остаются
// живым источником до Ф4 (FE-форма); credit_limit/billing_mode здесь — копия, авторитетным
// источником станут с Ф2.
type Counterparty struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Мультитенант-ключи (как у ledger_entries). Партиальный uniqueIndex по идентичности:
	// не более одного контрагента на (admin, company, id_type, tax_id) при непустом tax_id.
	// Контрагенты без tax_id (по имени) под уникальность не попадают — допускаются дубли-омонимы,
	// разрешаются ручной проверкой (manual_review).
	// idx_cp_identity — уникальность по налоговому id (tax_id<>''). idx_cp_name_legacy —
	// уникальность по ИМЕНИ для контрагентов без tax_id (иначе авто-назначение Ф4 при гонке
	// двух договоров одного безИНН-клиента создаёт дубли — Codex Ф4a HIGH-1).
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index;uniqueIndex:idx_cp_identity,where:tax_id <> '' AND deleted_at IS NULL;uniqueIndex:idx_cp_name_legacy,where:(tax_id = '' OR tax_id IS NULL) AND deleted_at IS NULL"`
	CompanyID      uint `json:"company_id" gorm:"not null;index;uniqueIndex:idx_cp_identity;uniqueIndex:idx_cp_name_legacy"`

	// Идентификатор контрагента.
	Country string `json:"country" gorm:"type:varchar(2);default:'ru';index"`                            // ru|kz|...
	IDType  string `json:"id_type" gorm:"not null;default:'inn';type:varchar(20);uniqueIndex:idx_cp_identity"` // inn|bin|iin|passport|other
	TaxID   string `json:"tax_id" gorm:"type:varchar(32);uniqueIndex:idx_cp_identity"`                    // ИНН/БИН/ИИН/паспорт; пусто = по имени

	Name       string `json:"name" gorm:"not null;type:varchar(200);index;uniqueIndex:idx_cp_name_legacy"`
	ClientType string `json:"client_type" gorm:"type:varchar(50)"` // organization|individual_entrepreneur|physical_person

	// Реквизиты (переносятся с Contract.Client* — будущий дом идентичности клиента, форма Ф4).
	ShortName      string `json:"short_name" gorm:"type:varchar(200)"`
	KPP            string `json:"kpp" gorm:"type:varchar(20)"`
	Email          string `json:"email" gorm:"type:varchar(100)"`
	Phone          string `json:"phone" gorm:"type:varchar(20)"`
	Address        string `json:"address" gorm:"type:text"`
	LegalAddress   string `json:"legal_address" gorm:"type:text"`
	PostalAddress  string `json:"postal_address" gorm:"type:text"`
	OGRN           string `json:"ogrn" gorm:"type:varchar(20)"`
	OKPO           string `json:"okpo" gorm:"type:varchar(20)"`
	Director       string `json:"director" gorm:"type:varchar(200)"`
	BasedOn        string `json:"based_on" gorm:"type:varchar(200)"`
	Website        string `json:"website" gorm:"type:varchar(200)"`

	// Банковские реквизиты.
	BankName                 string `json:"bank_name" gorm:"type:varchar(200)"`
	BankBIK                  string `json:"bank_bik" gorm:"type:varchar(20)"`
	BankCorrespondentAccount string `json:"bank_correspondent_account" gorm:"type:varchar(20)"`
	BankAccount              string `json:"bank_account" gorm:"type:varchar(20)"`
	BankRecipient            string `json:"bank_recipient" gorm:"type:varchar(200)"`

	// Поля физлиц.
	PassportSeries         string `json:"passport_series" gorm:"type:varchar(10)"`
	PassportNumber         string `json:"passport_number" gorm:"type:varchar(20)"`
	PassportIssuedBy       string `json:"passport_issued_by" gorm:"type:text"`
	PassportIssueDate      string `json:"passport_issue_date" gorm:"type:varchar(20)"`
	PassportDepartmentCode string `json:"passport_department_code" gorm:"type:varchar(10)"`
	RegistrationAddress    string `json:"registration_address" gorm:"type:text"`
	ActualAddress          string `json:"actual_address" gorm:"type:text"`
	SNILS                  string `json:"snils" gorm:"type:varchar(20)"`
	OGRNIP                 string `json:"ogrnip" gorm:"column:ogrn_ip;type:varchar(20)"`

	// Биллинг per-контрагент (единый лимит/режим на все договоры). Копия с Contract в Ф1;
	// авторитетным источником баланса/лимита станет с Ф2.
	BillingMode string          `json:"billing_mode" gorm:"default:'prepaid';type:varchar(20)"`
	CreditLimit decimal.Decimal `json:"credit_limit" gorm:"type:decimal(15,2);default:0"`

	// Контрагент без tax_id (заведён по имени при миграции) — пометка ручной проверки.
	ManualReview bool   `json:"manual_review" gorm:"default:false;index"`
	CreatedBy    string `json:"created_by" gorm:"type:varchar(100)"`
}

func (Counterparty) TableName() string { return "counterparties" }
