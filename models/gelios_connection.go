package models

import (
	"time"

	"gorm.io/gorm"
)

// GeliosConnection — подключение к GELIOS GPS (api.geliospro.com).
//
// Auth: POST /api/v1/auth/login, Content-Type x-www-form-urlencoded,
// body grant_type=password&username=<urlenc>&password=<...>
// → {access_token, refresh_token, expires_in, token_type}.
// Далее каждый запрос с заголовком Authorization: Bearer <access_token>.
// Токен короткоживущий → кешируем + refresh по expires.
//
// База знаний: ACRM-Brain/wiki/sources/gelios-api/billing.md
type GeliosConnection struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Тенантность + имя для UI
	CompanyID uint   `json:"company_id" gorm:"not null;index"`
	Name      string `json:"name" gorm:"not null;type:varchar(255)"` // "Основной аккаунт GELIOS"

	// Подключение
	BaseURL  string `json:"base_url" gorm:"not null;type:varchar(255);default:'https://api.geliospro.com'"`
	Username string `json:"username" gorm:"not null;type:varchar(255)"`
	Password string `json:"-" gorm:"type:text"` // Скрыт из JSON, шифруется utils.EncryptString (enc:v1:)

	// OAuth2-токен — заполняется после успешного login, переиспользуется до истечения.
	AccessToken    string     `json:"-" gorm:"type:text"`
	RefreshToken   string     `json:"-" gorm:"type:text"`
	TokenExpiresAt *time.Time `json:"-"`
	LastLoginAt    *time.Time `json:"last_login_at"`

	// Статус
	IsActive   bool       `json:"is_active" gorm:"default:true"`
	LastSyncAt *time.Time `json:"last_sync_at"`
	UnitsCount int        `json:"units_count" gorm:"default:0"`

	// EmptySyncStreak — сколько ПОДРЯД синков вернули totalCount=0 при
	// наличии живых юнитов. Защита от API-flap: mass-soft-delete только
	// после geliosEmptyConfirmTicks подтверждений (transient 0 не сносит
	// флот; легит-пустой аккаунт чистится после подтверждения).
	EmptySyncStreak int `json:"-" gorm:"default:0"`

	// LoginCount — счётчик full password-login (не refresh). Наблюдаемость
	// login-rate: высокий рост = refresh ломается (фикс #1 не работает).
	LoginCount int64 `json:"login_count" gorm:"default:0"`

	// Sync settings
	SyncInterval    int  `json:"sync_interval" gorm:"default:15"` // минуты
	AutoSyncEnabled bool `json:"auto_sync_enabled" gorm:"default:false"`
	SyncUnits       bool `json:"sync_units" gorm:"default:true"`

	// Ошибки
	LastErrorAt  *time.Time `json:"last_error_at"`
	ErrorMessage string     `json:"error_message" gorm:"type:text"`
	ErrorCount   int        `json:"error_count" gorm:"default:0"`

	// Связи
	Company *Company `json:"company,omitempty" gorm:"foreignKey:CompanyID;constraint:-"`
}

func (GeliosConnection) TableName() string { return "gelios_connections" }
