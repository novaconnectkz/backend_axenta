package models

import "time"

// GeliosUser — реестр пользователей GELIOS (одна запись на (connection, gelios_user_id)).
// UNIQUE(connection_id, gelios_user_id) гарантирует дедупликацию.
//
// В GELIOS НЕТ сущности «аккаунт» — всё дерево пользователей, биллинг привязан
// к юзеру (balance/currency/units_limit). Иерархия через creator (родитель):
// GET /api/v1/users отдаёт плоский рекурсивный список потомков token-owner,
// дерево реконструируется по creator_id. token-owner сам в /users НЕ входит.
// Юзер с is_admin || units_count>0 одновременно играет роль «учётной записи».
//
// Источник: GET /api/v1/users?inclbillinf=true (paginate, paginationMetadata.totalCount).
// Заполняется GeliosSyncScheduler по расписанию connection.SyncInterval
// (forward-сбор, прошлое невосстановимо — как GeliosUnit).
//
// База знаний: ACRM-Brain/wiki/sources/gelios-api/users.md
type GeliosUser struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID uint   `json:"connection_id" gorm:"not null;index;uniqueIndex:uq_gelios_users_conn_user"`
	GeliosUserID string `json:"gelios_user_id" gorm:"not null;type:varchar(64);uniqueIndex:uq_gelios_users_conn_user"` // user.id из GELIOS

	Login     string `json:"login"      gorm:"type:varchar(255);index"`
	Email     string `json:"email"      gorm:"type:varchar(255);index"`
	Phone     string `json:"phone"      gorm:"type:varchar(50)"`
	LegalName string `json:"legal_name" gorm:"type:varchar(255);index"`

	IsAdmin bool `json:"is_admin" gorm:"index;default:false"` // узел-дилер/админ (может иметь детей)
	IsBlock bool `json:"is_block" gorm:"index;default:false"` // заблокирован в GELIOS

	// Родитель в иерархии (creator). У token-owner в /users не отдаётся;
	// для верхних узлов creator = сам token-owner.
	CreatorID    int64  `json:"creator_id"    gorm:"index"`
	CreatorLogin string `json:"creator_login" gorm:"type:varchar(255);index"`

	// Биллинг юзера (BillingInfoModelOutput). null в GET /users без inclbillinf=true.
	Balance           float64 `json:"balance"              gorm:"default:0"`
	MinBalance        float64 `json:"min_balance"          gorm:"default:0"`
	CurrencyCode      string  `json:"currency_code"        gorm:"type:varchar(8)"`  // alphabeticCode "RUB"
	CurrencyAbbr      string  `json:"currency_abbr"        gorm:"type:varchar(16)"` // abbreviation "₽"
	UnitsLimit        *int    `json:"units_limit"`
	FreeUnitsLimit    *int    `json:"free_units_limit"`
	PaymentTypeID     int     `json:"payment_type_id"`
	PaymentTypeName   string  `json:"payment_type_name"    gorm:"type:varchar(64)"` // "Active units"
	PaymentPeriodID   int     `json:"payment_period_id"`
	PaymentPeriodName string  `json:"payment_period_name"  gorm:"type:varchar(64)"` // "Monthly"
	PaymentPeriodLen  int     `json:"payment_period_len"`
	UnitsCount        int     `json:"units_count"          gorm:"default:0"`

	CompanyID uint `json:"company_id" gorm:"index"` // ACRM tenant company_id (для фильтра)

	LastCollectedAt time.Time  `json:"last_collected_at" gorm:"not null;index"`
	GeliosDeletedAt *time.Time `json:"gelios_deleted_at" gorm:"index"` // Когда юзер исчез из выгрузки (soft-delete метка; GELIOS DELETE = hard)
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	Connection *GeliosConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (GeliosUser) TableName() string { return "gelios_users" }
