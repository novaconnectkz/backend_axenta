package models

import (
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestCompany - тестовая модель компании для SQLite
type TestCompany struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля компании
	Name           string `json:"name" gorm:"not null"`
	DatabaseSchema string `json:"database_schema" gorm:"uniqueIndex;not null"`
	Domain         string `json:"domain"`

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
	Address       string `json:"address"`
	City          string `json:"city"`
	Country       string `json:"country" gorm:"default:Russia"`

	// Статус и ограничения
	IsActive     bool `json:"is_active" gorm:"default:true"`
	MaxUsers     int  `json:"max_users" gorm:"default:10"`
	MaxObjects   int  `json:"max_objects" gorm:"default:100"`
	StorageQuota int  `json:"storage_quota" gorm:"default:1024"` // В МБ

	// Настройки локализации
	Language string `json:"language" gorm:"default:ru"`
	Timezone string `json:"timezone" gorm:"default:Europe/Moscow"`
	Currency string `json:"currency" gorm:"default:RUB"`

	// Связь с подпиской
	SubscriptionID *string `json:"subscription_id"`
}

// TestUser - тестовая модель пользователя для SQLite
type TestUser struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основная информация
	Username  string `json:"username" gorm:"uniqueIndex;not null"`
	Email     string `json:"email" gorm:"uniqueIndex;not null"`
	Password  string `json:"-" gorm:"not null"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`

	// Telegram интеграция
	TelegramID string `json:"telegram_id"`

	// Статус и тип
	IsActive bool   `json:"is_active" gorm:"default:true"`
	UserType string `json:"user_type" gorm:"default:user"`

	// Внешние связи
	ExternalID     string `json:"external_id"`
	ExternalSource string `json:"external_source"`

	// Связи
	CompanyID  uint  `json:"company_id" gorm:"not null"`
	RoleID     uint  `json:"role_id"`
	TemplateID *uint `json:"template_id"`

	// Статистика
	LastLogin  *time.Time `json:"last_login"`
	LoginCount int        `json:"login_count" gorm:"default:0"`
}

// TestObject - тестовая модель объекта для SQLite
type TestObject struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основная информация
	Name        string `json:"name" gorm:"not null"`
	Description string `json:"description"`
	Type        string `json:"type" gorm:"not null"`
	Status      string `json:"status" gorm:"default:active"`

	// Связи
	CompanyID uint `json:"company_id" gorm:"not null"`

	// Дополнительные поля
	ExternalID string `json:"external_id"`
	IsActive   bool   `json:"is_active" gorm:"default:true"`
}

// TestContract - тестовая модель договора для SQLite
type TestContract struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основная информация
	Number      string `json:"number" gorm:"uniqueIndex;not null"`
	Title       string `json:"title" gorm:"not null"`
	Description string `json:"description"`

	// Связи
	CompanyID uint `json:"company_id" gorm:"not null"`

	// Клиент
	ClientName    string `json:"client_name" gorm:"not null"`
	ClientContact string `json:"client_contact"`

	// Даты
	StartDate time.Time `json:"start_date" gorm:"not null"`
	EndDate   time.Time `json:"end_date" gorm:"not null"`

	// Финансы
	MonthlyFee string `json:"monthly_fee" gorm:"type:decimal(10,2);not null"`
	Currency   string `json:"currency" gorm:"default:RUB"`

	// Статус
	Status   string `json:"status" gorm:"default:active"`
	IsActive bool   `json:"is_active" gorm:"default:true"`
}

// setupTestDB создает тестовую базу данных с совместимыми моделями
func setupTestDBWithCompatibleModels(t interface{}) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database: " + err.Error())
	}

	// Мигрируем тестовые модели
	err = db.AutoMigrate(
		&TestCompany{},
		&TestUser{},
		&TestObject{},
		&TestContract{},
	)
	if err != nil {
		panic("failed to migrate database: " + err.Error())
	}

	return db
}
