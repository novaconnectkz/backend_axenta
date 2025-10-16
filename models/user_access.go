package models

import (
	"time"

	"gorm.io/gorm"
)

// UserAccess представляет модель доступа пользователя
type UserAccess struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	UserID uint   `json:"user_id" gorm:"not null;index"`
	User   *User  `json:"user,omitempty" gorm:"foreignKey:UserID"`
	Scope  string `json:"scope" gorm:"not null;size:100"` // Область доступа
	Perms  string `json:"perms" gorm:"type:text"`         // JSON строка с разрешениями
}

// TableName задает имя таблицы для модели UserAccess
func (UserAccess) TableName() string {
	return "user_accesses"
}
