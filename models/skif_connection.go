package models

import (
	"time"

	"gorm.io/gorm"
)

// SkifConnection — подключение к SKIF.PRO (cookie session-based auth).
//
// Auth: POST /api_v1/login {userProviderId, provider_key="TEXT", password}
// → Set-Cookie: session_id. Дальше каждый запрос с этой cookie.
//
// База знаний: ACRM-Brain/wiki/sources/skif-api.md
type SkifConnection struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Тенантность + имя для UI
	CompanyID uint   `json:"company_id" gorm:"not null;index"`
	Name      string `json:"name" gorm:"not null;type:varchar(255)"` // "Основной аккаунт SKIF"

	// Подключение
	BaseURL  string `json:"base_url" gorm:"not null;type:varchar(255);default:'https://app.skif.pro'"`
	Login    string `json:"login" gorm:"not null;type:varchar(255)"`
	Password string `json:"-" gorm:"type:text"` // Скрыт из JSON, хранится как есть (TODO: encrypt)

	// Cookie session — заполняется после успешного login.
	// Полный raw-cookie-string для воспроизведения в HTTP-клиенте.
	SessionCookie string     `json:"-" gorm:"type:text"`
	LastLoginAt   *time.Time `json:"last_login_at"`

	// Статус
	IsActive   bool       `json:"is_active" gorm:"default:true"`
	LastSyncAt *time.Time `json:"last_sync_at"`
	UnitsCount int        `json:"units_count" gorm:"default:0"`
	UsersCount int        `json:"users_count" gorm:"default:0"`

	// Sync settings
	SyncInterval    int  `json:"sync_interval" gorm:"default:15"`        // минуты
	AutoSyncEnabled bool `json:"auto_sync_enabled" gorm:"default:false"`
	SyncUnits       bool `json:"sync_units" gorm:"default:true"`
	SyncTerminals   bool `json:"sync_terminals" gorm:"default:false"`

	// Ошибки
	LastErrorAt  *time.Time `json:"last_error_at"`
	ErrorMessage string     `json:"error_message" gorm:"type:text"`
	ErrorCount   int        `json:"error_count" gorm:"default:0"`

	// Связи
	Company *Company `json:"company,omitempty" gorm:"foreignKey:CompanyID;constraint:-"`
}

func (SkifConnection) TableName() string { return "skif_connections" }
