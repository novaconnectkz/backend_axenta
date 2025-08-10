package models

import (
	"time"

	"gorm.io/gorm"
)

// User представляет модель пользователя в системе
type User struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	Username string `json:"username" gorm:"uniqueIndex;not null"`
	Email    string `json:"email" gorm:"uniqueIndex;not null"`
	Password string `json:"-" gorm:"not null"` // Пароль не возвращается в JSON

	// Дополнительные поля
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role" gorm:"default:'user'"`
	IsActive  bool   `json:"is_active" gorm:"default:true"`

	// Для системы управления складом [[memory:5739350]]
	CompanyID uint `json:"company_id" gorm:"not null;index"`
}

// TableName задает имя таблицы для модели User
func (User) TableName() string {
	return "users"
}
