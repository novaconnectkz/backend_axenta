package models

import (
	"time"

	"gorm.io/gorm"
)

// Object представляет объект мониторинга в системе
type Object struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля объекта
	Name        string `json:"name" gorm:"not null;type:varchar(100)"`
	Type        string `json:"type" gorm:"not null;type:varchar(50)"` // vehicle, equipment, asset, etc.
	Description string `json:"description" gorm:"type:text"`

	// Географические координаты
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Address   string   `json:"address" gorm:"type:text"`

	// Идентификаторы устройства
	IMEI         string `json:"imei" gorm:"uniqueIndex;type:varchar(20)"`
	PhoneNumber  string `json:"phone_number" gorm:"type:varchar(20)"`
	SerialNumber string `json:"serial_number" gorm:"type:varchar(50)"`

	// Статус объекта
	Status            string     `json:"status" gorm:"default:'active';type:varchar(20)"` // active, inactive, maintenance, deleted
	IsActive          bool       `json:"is_active" gorm:"default:true"`
	ScheduledDeleteAt *time.Time `json:"scheduled_delete_at"` // Плановая дата удаления
	LastActivityAt    *time.Time `json:"last_activity_at"`    // Последняя активность

	// Связи с другими сущностями
	ContractID uint      `json:"contract_id" gorm:"not null;index"`
	Contract   *Contract `json:"contract,omitempty" gorm:"foreignKey:ContractID"`

	TemplateID *uint           `json:"template_id"`
	Template   *ObjectTemplate `json:"template,omitempty" gorm:"foreignKey:TemplateID"`

	LocationID uint      `json:"location_id" gorm:"index"`
	Location   *Location `json:"location,omitempty" gorm:"foreignKey:LocationID"`

	// Оборудование, установленное на объекте
	Equipment []Equipment `json:"equipment,omitempty" gorm:"foreignKey:ObjectID"`

	// Монтажи и обслуживание
	Installations []Installation `json:"installations,omitempty" gorm:"foreignKey:ObjectID"`

	// Дополнительные настройки (JSON)
	Settings string `json:"settings" gorm:"type:jsonb"` // Настройки мониторинга, уведомлений и т.д.

	// Метаданные
	Tags       []string `json:"tags" gorm:"type:text[]"`              // Теги для группировки
	Notes      string   `json:"notes" gorm:"type:text"`               // Заметки
	ExternalID string   `json:"external_id" gorm:"type:varchar(100)"` // ID во внешних системах
}
