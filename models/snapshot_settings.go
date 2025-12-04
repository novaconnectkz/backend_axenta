package models

import (
	"time"

	"gorm.io/gorm"
)

// SnapshotSettings представляет настройки для автоматического создания снимков
type SnapshotSettings struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связь с компанией (суперадмин, ID=1)
	CompanyID uint `json:"company_id" gorm:"not null;uniqueIndex;default:1"`

	// Токен для доступа к Axenta API
	AxentaToken string `json:"axenta_token" gorm:"type:text;not null"`

	// Дополнительные настройки
	IsActive bool `json:"is_active" gorm:"default:true"`
}

// TableName задает имя таблицы для модели SnapshotSettings
func (SnapshotSettings) TableName() string {
	return "snapshot_settings"
}

