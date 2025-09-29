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
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Name       string `json:"name" gorm:"type:varchar(200)"` // Полное имя для контрагентов
	Phone      string `json:"phone" gorm:"type:varchar(50)"`
	TelegramID string `json:"telegram_id" gorm:"type:varchar(50)"`
	IsActive   bool   `json:"is_active" gorm:"default:true"`

	// Поля для интеграций
	UserType       string `json:"user_type" gorm:"default:'user';type:varchar(50)"` // user, client, installer, etc.
	ExternalID     string `json:"external_id" gorm:"type:varchar(100)"`             // ID во внешних системах
	ExternalSource string `json:"external_source" gorm:"type:varchar(50)"`          // bitrix24, 1c, etc.

	// Для мультитенантности (временно, пока не перейдем полностью на схемы)
	CompanyID uint `json:"company_id" gorm:"index"`

	// Роль и права доступа
	RoleID uint  `json:"role_id" gorm:"index"`
	Role   *Role `json:"role,omitempty" gorm:"foreignKey:RoleID"`

	// Шаблон пользователя
	TemplateID *uint         `json:"template_id"`
	Template   *UserTemplate `json:"template,omitempty" gorm:"foreignKey:TemplateID"`

	// Дополнительные поля для CRM
	LastLogin  *time.Time `json:"last_login"`
	LoginCount int        `json:"login_count" gorm:"default:0"`

	// Примечание: CompanyID не нужен в мультитенантной архитектуре,
	// так как изоляция обеспечивается на уровне схем БД
}

// TableName задает имя таблицы для модели User
func (User) TableName() string {
	return "users"
}
