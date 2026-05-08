package models

import "time"

// SkifUser — реестр пользователей SKIF.PRO (одна запись на (connection, skif_user_id, skif_company_id)).
//
// SKIF-пользователь может состоять в нескольких компаниях с разной ролью, поэтому ключ
// тройной: один email → N строк (по одной на company-отношение). Аналогично паттерну
// SkifUnit, но с дополнительной размерностью company.
//
// Источник: POST /api_v1/users/query (см. wiki/sources/skif-api/polzovatel.md).
// Заполняется SkifSyncScheduler через /me → switchCompany → users/query per company.
type SkifUser struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID  uint   `json:"connection_id"  gorm:"not null;index;uniqueIndex:uq_skif_users_conn_user_company"`
	SkifUserID    string `json:"skif_user_id"   gorm:"not null;type:varchar(64);uniqueIndex:uq_skif_users_conn_user_company"` // UUID из SKIF
	SkifCompanyID string `json:"skif_company_id" gorm:"not null;type:varchar(64);uniqueIndex:uq_skif_users_conn_user_company;index"` // UUID компании в SKIF

	Name           string `json:"name"             gorm:"type:varchar(255);index"`
	Email          string `json:"email"            gorm:"type:varchar(255);index"`
	Phone          string `json:"phone"            gorm:"type:varchar(50)"`
	RoleKey        string `json:"role_key"         gorm:"type:varchar(32);index"` // EDITOR / ADMIN / READER / SUPERVISOR / NoAccess
	RoleName       string `json:"role_name"        gorm:"type:varchar(64)"`        // display name из API ("Редактор" и т.п.)
	IsActive       bool   `json:"is_active"        gorm:"index;default:true"`
	IsApproved     bool   `json:"is_approved"      gorm:"default:false"` // подтверждён email
	IsDriver       bool   `json:"is_driver"        gorm:"default:false"`
	Code           string `json:"code"             gorm:"type:varchar(64)"`  // RFID id
	Language       string `json:"language"         gorm:"type:varchar(8)"`   // ключ языка ("en", "ru")
	Details        string `json:"details"          gorm:"type:text"`
	TelegramChatID string `json:"telegram_chat_id" gorm:"type:varchar(64)"`

	CompanyID   uint   `json:"company_id"   gorm:"index"` // ACRM tenant company_id (для фильтра)
	SkifCompany string `json:"skif_company" gorm:"type:varchar(255)"` // Название компании в SKIF

	LastCollectedAt time.Time  `json:"last_collected_at" gorm:"not null;index"`
	SkifCreatedAt   *time.Time `json:"skif_created_at"   gorm:"index"` // дата создания юзера в SKIF (поле created из API)
	SkifDeletedAt   *time.Time `json:"skif_deleted_at"   gorm:"index"` // когда юзер исчез из выгрузки SKIF
	LastLoginAt     *time.Time `json:"last_login_at"     gorm:"index"` // если SKIF отдаёт last_login

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Connection *SkifConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (SkifUser) TableName() string { return "skif_users" }
