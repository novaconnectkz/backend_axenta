package models

import (
	"time"

	"gorm.io/gorm"
)

// AxentaAccountSnapshot хранит актуальные данные учетных записей Axenta для партнера
type AxentaAccountSnapshot struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	AdminAccountID    uint   `json:"admin_account_id" gorm:"not null;index;uniqueIndex:idx_axenta_account_admin_external"`
	ExternalAccountID int64  `json:"external_account_id" gorm:"not null;uniqueIndex:idx_axenta_account_admin_external"`
	AccountName       string `json:"account_name" gorm:"type:varchar(200)"`
	AccountType       string `json:"account_type" gorm:"type:varchar(50)"`
	AdminFullname     string `json:"admin_fullname" gorm:"type:varchar(200)"`
	AdminExternalID   *int64 `json:"admin_external_id"`
	ParentAccountName string `json:"parent_account_name" gorm:"type:varchar(200)"`
	Hierarchy         string `json:"hierarchy" gorm:"type:text"`
	Comment           string `json:"comment" gorm:"type:text"`
	IsActive          bool   `json:"is_active" gorm:"index"`

	BlockingDatetime   *time.Time `json:"blocking_datetime"`
	DaysBeforeBlocking *int       `json:"days_before_blocking"`

	ObjectsActive int `json:"objects_active"`
	ObjectsTotal  int `json:"objects_total"`

	LastSyncedAt time.Time `json:"last_synced_at" gorm:"not null;index"`
	RawPayload   string    `json:"raw_payload" gorm:"type:jsonb"`
}

// TableName задает имя таблицы для снимков учетных записей Axenta
func (AxentaAccountSnapshot) TableName() string {
	return "axenta_account_snapshots"
}

// AxentaObjectSnapshot хранит актуальные данные объектов Axenta для партнера
type AxentaObjectSnapshot struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	AdminAccountID      uint       `json:"admin_account_id" gorm:"not null;index;uniqueIndex:idx_axenta_object_admin_external"`
	AccountExternalID   int64      `json:"account_external_id" gorm:"not null;index"`
	ExternalObjectID    int64      `json:"external_object_id" gorm:"not null;uniqueIndex:idx_axenta_object_admin_external"`
	ObjectName          string     `json:"object_name" gorm:"type:varchar(200)"`
	UniqueID            string     `json:"unique_id" gorm:"type:varchar(100)"`
	DeviceTypeName      string     `json:"device_type_name" gorm:"type:varchar(200)"`
	AccountName         string     `json:"account_name" gorm:"type:varchar(200)"`
	Status              string     `json:"status" gorm:"type:varchar(50)"`
	IsActive            bool       `json:"is_active" gorm:"index"`
	ScheduledDeleteAt   *time.Time `json:"scheduled_delete_at"`
	LastCommunicationAt *time.Time `json:"last_communication_at"`

	// Новые поля из Axenta Cloud API (nullable для обратной совместимости)
	CreatorName     *string    `json:"creator_name" gorm:"type:varchar(200)"`
	CreatorID       *int       `json:"creator_id"`
	CreatorIsActive *bool      `json:"creator_is_active"`
	AccountIsActive *bool      `json:"account_is_active"`
	PhoneNumbers    *string    `json:"phone_numbers" gorm:"type:jsonb"` // JSON массив телефонов
	AxentaCreatedAt *time.Time `json:"axenta_created_at"`               // Дата создания в Axenta Cloud
	AxentaDeletedAt *time.Time `json:"axenta_deleted_at"`               // Дата удаления в Axenta Cloud

	LastSyncedAt time.Time `json:"last_synced_at" gorm:"not null;index"`
	RawPayload   string    `json:"raw_payload" gorm:"type:jsonb"`
}

// TableName задает имя таблицы для снимков объектов Axenta
func (AxentaObjectSnapshot) TableName() string {
	return "axenta_object_snapshots"
}
