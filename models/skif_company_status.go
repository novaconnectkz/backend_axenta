package models

import "time"

// SkifCompanyStatus — локальный кэш биллинг-статуса SKIF-компании.
//
// SKIF.skif_units.is_active = "юнит онлайн прямо сейчас", не имеет отношения к
// блокировке компании. Реальный статус (ACTIVE / BLOCKED / DELETING) живёт в
// admin.skif.pro/api_v1/auth_admin_query → billing.company_status и обновляется
// периодически SkifSyncScheduler.
type SkifCompanyStatus struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID  uint   `json:"connection_id" gorm:"not null;uniqueIndex:uq_skif_status_conn_comp;index"`
	SkifCompanyID string `json:"skif_company_id" gorm:"not null;type:varchar(64);uniqueIndex:uq_skif_status_conn_comp"`
	CompanyName   string `json:"company_name" gorm:"type:varchar(255)"`

	// CompanyStatus — billing.company_status из SKIF (ACTIVE / BLOCKED / DELETING / etc).
	CompanyStatus string `json:"company_status" gorm:"type:varchar(32);index"`

	// TerminalBlockType — billing.terminal_block_type ("terminals_not_block" / "terminals_block").
	TerminalBlockType string `json:"terminal_block_type" gorm:"type:varchar(32)"`

	// SkifDealerID — UUID дилера-владельца компании (auth_admin_query → dealer_id).
	// Ф1 partner billing: связь компания→дилер для агрегации объектов на дилера.
	SkifDealerID string `json:"skif_dealer_id" gorm:"type:varchar(64);index"`

	UnitsCount int `json:"units_count"`
	UsersCount int `json:"users_count"`

	LastSyncedAt time.Time `json:"last_synced_at" gorm:"index"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	Connection *SkifConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (SkifCompanyStatus) TableName() string { return "skif_company_statuses" }
