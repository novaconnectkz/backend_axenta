package models

import (
	"time"

	"gorm.io/gorm"
)

// AxentaUserSnapshot хранит актуальные данные пользователей Axenta для быстрого read-path /unified/users.
// Заполняется AxentaSyncService параллельно с AxentaAccountSnapshot.
// Read-path: tryServeUnifiedUsersFromSnapshot в api/unified_users.go (TTL 60м, fallback на live при устаревании).
type AxentaUserSnapshot struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	AdminAccountID uint  `json:"admin_account_id" gorm:"not null;index;uniqueIndex:idx_axenta_user_admin_external"`
	ExternalUserID int64 `json:"external_user_id" gorm:"not null;uniqueIndex:idx_axenta_user_admin_external"`

	Username         string     `json:"username" gorm:"type:varchar(200);index:idx_axenta_user_username"`
	Name             string     `json:"name" gorm:"type:varchar(300)"`
	Email            string     `json:"email" gorm:"type:varchar(200);index"`
	AccountType      string     `json:"account_type" gorm:"type:varchar(50);index:idx_axenta_user_account_type"`
	IsActive         bool       `json:"is_active" gorm:"index"`
	CreatorName      string     `json:"creator_name" gorm:"type:varchar(200);index"`
	CreationDatetime string     `json:"creation_datetime" gorm:"type:varchar(50)"`
	LastLogin        *time.Time `json:"last_login" gorm:"index"`

	LastSyncedAt time.Time `json:"last_synced_at" gorm:"not null;index"`
	RawPayload   string    `json:"raw_payload" gorm:"type:jsonb"`
}

// TableName задаёт имя таблицы для снимков пользователей Axenta
func (AxentaUserSnapshot) TableName() string {
	return "axenta_user_snapshots"
}
