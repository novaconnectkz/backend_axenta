package models

import (
	"time"

	"gorm.io/gorm"
)

// SystemSettings хранит общие настройки системы для компании
type SystemSettings struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связь с компанией
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index"`
	CompanyID      uint `json:"company_id" gorm:"not null;uniqueIndex:idx_system_settings_company"`

	// Настройки компании
	CompanyName string  `json:"company_name" gorm:"type:varchar(255)"`
	CompanyLogo *string `json:"company_logo" gorm:"type:text"`

	// Региональные настройки
	Timezone   string `json:"timezone" gorm:"type:varchar(50);default:'Europe/Moscow'"`
	DateFormat string `json:"date_format" gorm:"type:varchar(20);default:'DD.MM.YYYY'"`
	Currency   string `json:"currency" gorm:"type:varchar(3);default:'RUB'"`
	Language   string `json:"language" gorm:"type:varchar(10);default:'ru'"`
	Theme      string `json:"theme" gorm:"type:varchar(10);default:'light'"` // light, dark, auto

	// Настройки безопасности
	SessionTimeout         int  `json:"session_timeout" gorm:"default:480"`          // в минутах
	PasswordMinLength      int  `json:"password_min_length" gorm:"default:8"`        // минимальная длина пароля
	PasswordRequireSpecial bool `json:"password_require_special" gorm:"default:true"` // требовать спецсимволы
	MaxLoginAttempts       int  `json:"max_login_attempts" gorm:"default:5"`         // макс попыток входа

	// Настройки уведомлений
	EmailNotificationsEnabled    bool `json:"email_notifications_enabled" gorm:"default:true"`
	SmsNotificationsEnabled      bool `json:"sms_notifications_enabled" gorm:"default:false"`
	TelegramNotificationsEnabled bool `json:"telegram_notifications_enabled" gorm:"default:true"`

	// Налоговые настройки
	VATRatePreset  string  `json:"vat_rate_preset" gorm:"type:varchar(20);default:'russia'"` // russia, kazakhstan, none, custom
	VATRateCustom  float64 `json:"vat_rate_custom" gorm:"type:decimal(5,2);default:20.00"`      // своя ставка НДС
	DefaultTaxRate float64 `json:"default_tax_rate" gorm:"type:decimal(5,2);default:20.00"`     // ставка НДС по умолчанию
	TaxIncluded    bool    `json:"tax_included" gorm:"default:false"`                        // НДС включен в цену

	// Настройки резервного копирования
	BackupEnabled        bool   `json:"backup_enabled" gorm:"default:true"`
	BackupSchedule       string `json:"backup_schedule" gorm:"type:varchar(50);default:'0 2 * * *'"` // cron expression
	BackupRetentionDays  int    `json:"backup_retention_days" gorm:"default:30"`
}

// TableName задает имя таблицы для модели SystemSettings
func (SystemSettings) TableName() string {
	return "system_settings"
}

