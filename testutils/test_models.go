package testutils

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TestCompany - SQLite-совместимая версия модели Company для тестов
type TestCompany struct {
	ID        string         `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля компании
	Name           string `json:"name" gorm:"not null"`
	DatabaseSchema string `json:"database_schema" gorm:"uniqueIndex;not null"`
	Domain         string `json:"domain" gorm:"uniqueIndex"`

	// Интеграция с Axenta.cloud
	AxetnaLogin    string `json:"-" gorm:"not null"`
	AxetnaPassword string `json:"-" gorm:"not null"`

	// Интеграция с Битрикс24
	Bitrix24WebhookURL   string `json:"-"`
	Bitrix24ClientID     string `json:"-"`
	Bitrix24ClientSecret string `json:"-"`

	// Контактная информация
	ContactEmail  string `json:"contact_email"`
	ContactPhone  string `json:"contact_phone"`
	ContactPerson string `json:"contact_person"`

	// Адрес
	Address string `json:"address"`
	City    string `json:"city"`
	Country string `json:"country" gorm:"default:'Russia'"`

	// Настройки и статус
	IsActive     bool `json:"is_active" gorm:"default:true"`
	MaxUsers     int  `json:"max_users" gorm:"default:10"`
	MaxObjects   int  `json:"max_objects" gorm:"default:100"`
	StorageQuota int  `json:"storage_quota" gorm:"default:1024"`

	// Настройки локализации
	Language string `json:"language" gorm:"default:'ru'"`
	Timezone string `json:"timezone" gorm:"default:'Europe/Moscow'"`
	Currency string `json:"currency" gorm:"default:'RUB'"`

	// Подписка
	SubscriptionID *string `json:"subscription_id"`
}

// BeforeCreate устанавливает UUID перед созданием
func (c *TestCompany) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return nil
}

// TestUser - SQLite-совместимая версия модели User для тестов
type TestUser struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	Username string `json:"username" gorm:"uniqueIndex;not null"`
	Email    string `json:"email" gorm:"uniqueIndex;not null"`
	Password string `json:"-" gorm:"not null"`

	// Профиль
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone"`

	// Статус и роли
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	LastLoginAt time.Time `json:"last_login_at"`

	// Связи
	CompanyID string `json:"company_id" gorm:"not null;index"`
	RoleID    uint   `json:"role_id" gorm:"index"`
}

// TestRole - SQLite-совместимая версия модели Role для тестов
type TestRole struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	Name        string `json:"name" gorm:"uniqueIndex;not null"`
	Description string `json:"description"`
	IsSystem    bool   `json:"is_system" gorm:"default:false"`

	// Связи
	Users []TestUser `json:"users" gorm:"foreignKey:RoleID"`
}

// TestPermission - SQLite-совместимая версия модели Permission для тестов
type TestPermission struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	Name        string `json:"name" gorm:"uniqueIndex;not null"`
	Description string `json:"description"`
	Resource    string `json:"resource" gorm:"not null"`
	Action      string `json:"action" gorm:"not null"`
}

// GetTestModels возвращает список всех тестовых моделей для миграции
func GetTestModels() []interface{} {
	return []interface{}{
		&TestCompany{},
		&TestUser{},
		&TestRole{},
		&TestPermission{},
	}
}
