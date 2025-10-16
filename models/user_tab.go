package models

import (
	"time"

	"gorm.io/gorm"
)

// UserTab представляет модель вкладки пользователя
type UserTab struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	UserID uint   `json:"user_id" gorm:"not null;index"`
	User   *User  `json:"user,omitempty" gorm:"foreignKey:UserID"`
	Name   string `json:"name" gorm:"not null;size:100"` // Название вкладки (monitoring, reports, objects, etc.)
}

// TableName задает имя таблицы для модели UserTab
func (UserTab) TableName() string {
	return "user_tabs"
}
