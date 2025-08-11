package models

import (
	"time"

	"gorm.io/gorm"
)

// BillingPlan представляет модель тарифного плана в системе
type BillingPlan struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля тарифного плана
	Name        string  `json:"name" gorm:"uniqueIndex;not null;type:varchar(100)"`
	Description string  `json:"description" gorm:"type:text"`
	Price       float64 `json:"price" gorm:"not null"`
	Currency    string  `json:"currency" gorm:"default:'RUB';type:varchar(3)"`

	// Период тарификации
	BillingPeriod string `json:"billing_period" gorm:"default:'monthly';type:varchar(20)"` // monthly, yearly, one-time

	// Лимиты и возможности
	MaxDevices      int  `json:"max_devices" gorm:"default:0"` // 0 = безлимитно
	MaxUsers        int  `json:"max_users" gorm:"default:0"`   // 0 = безлимитно
	MaxStorage      int  `json:"max_storage" gorm:"default:0"` // в ГБ, 0 = безлимитно
	HasAnalytics    bool `json:"has_analytics" gorm:"default:false"`
	HasAPI          bool `json:"has_api" gorm:"default:false"`
	HasSupport      bool `json:"has_support" gorm:"default:false"`
	HasCustomDomain bool `json:"has_custom_domain" gorm:"default:false"`

	// Статус и доступность
	IsActive  bool `json:"is_active" gorm:"default:true"`
	IsPopular bool `json:"is_popular" gorm:"default:false"`

	// Для управления доступом
	CompanyID uint `json:"company_id" gorm:"index"` // Если тариф специфичен для компании
}

// TableName задает имя таблицы для модели BillingPlan
func (BillingPlan) TableName() string {
	return "billing_plans"
}

// Subscription представляет подписку компании на тарифный план
type Subscription struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связи
	CompanyID     uint        `json:"company_id" gorm:"not null;index"`
	BillingPlanID uint        `json:"billing_plan_id" gorm:"not null"`
	BillingPlan   BillingPlan `json:"billing_plan" gorm:"foreignKey:BillingPlanID"`

	// Период подписки
	StartDate time.Time  `json:"start_date" gorm:"not null"`
	EndDate   *time.Time `json:"end_date"`

	// Статус подписки
	Status      string `json:"status" gorm:"default:'active';type:varchar(20)"` // active, expired, cancelled, suspended
	IsAutoRenew bool   `json:"is_auto_renew" gorm:"default:true"`

	// Платежная информация
	LastPaymentDate *time.Time `json:"last_payment_date"`
	NextPaymentDate *time.Time `json:"next_payment_date"`
	PaymentMethod   string     `json:"payment_method" gorm:"type:varchar(50)"`
}

// TableName задает имя таблицы для модели Subscription
func (Subscription) TableName() string {
	return "subscriptions"
}
