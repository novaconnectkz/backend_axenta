package models

import (
	"time"

	"gorm.io/gorm"
)

// UserToken хранит токен пользователя для Axenta Cloud
type UserToken struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	UserID    uint       `json:"user_id" gorm:"not null;index"`
	User      *User      `json:"user,omitempty" gorm:"foreignKey:UserID"`
	Username  string     `json:"username" gorm:"not null;size:100;index"`
	Token     string     `json:"token" gorm:"not null;type:text"`
	ExpiresAt *time.Time `json:"expires_at" gorm:"index"`
	IsActive  bool       `json:"is_active" gorm:"default:true"`
	AccountID uint       `json:"account_id" gorm:"not null;index"`

	// Дополнительная информация
	LastUsedAt *time.Time `json:"last_used_at"`
	UserAgent  string     `json:"user_agent" gorm:"type:text"`
	IPAddress  string     `json:"ip_address" gorm:"size:45"`
}
