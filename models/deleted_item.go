package models

import (
	"time"

	"gorm.io/gorm"
)

// DeletedItem представляет запись об удаленном элементе (аудит удалений)
type DeletedItem struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Идентификация удаленного элемента
	EntityType string `json:"entity_type" gorm:"not null;type:varchar(50);index"` // user, contract, object, warehouse, template, etc.
	EntityID   uint   `json:"entity_id" gorm:"not null;index"`                    // ID удаленного элемента
	EntityData string `json:"entity_data" gorm:"type:jsonb"`                      // Сохраненные данные удаленного элемента в JSON

	// Информация об удалении
	DeletedBy       uint      `json:"deleted_by" gorm:"not null;index"`                                 // ID пользователя, который удалил
	DeletedByName   string    `json:"deleted_by_name" gorm:"type:varchar(200)"`                         // Имя пользователя, который удалил
	DeletedAtCustom time.Time `json:"deleted_at_custom" gorm:"not null;index;column:deleted_at_custom"` // Время удаления оригинального элемента
	DeleteReason    string    `json:"delete_reason" gorm:"type:text"`                                   // Причина удаления (опционально)

	// Для мультитенантности
	CompanyID uint `json:"company_id" gorm:"not null;index"`

	// Метаданные для удобного отображения
	EntityName        string `json:"entity_name" gorm:"type:varchar(200)"` // Название удаленного элемента
	EntityDescription string `json:"entity_description" gorm:"type:text"`  // Описание удаленного элемента
	EntityPreview     string `json:"entity_preview" gorm:"type:text"`      // Краткое превью данных

	// Статус восстановления
	IsRestored     bool       `json:"is_restored" gorm:"default:false"`
	RestoredAt     *time.Time `json:"restored_at"`
	RestoredBy     *uint      `json:"restored_by" gorm:"index"`
	RestoredByName string     `json:"restored_by_name" gorm:"type:varchar(200)"`

	// Окончательное удаление
	IsPermanentlyDeleted bool       `json:"is_permanently_deleted" gorm:"default:false"`
	PermanentlyDeletedAt *time.Time `json:"permanently_deleted_at"`
	PermanentlyDeletedBy *uint      `json:"permanently_deleted_by" gorm:"index"`
}

// TableName задает имя таблицы для модели DeletedItem
func (DeletedItem) TableName() string {
	return "deleted_items"
}

// CanBeRestored проверяет, может ли элемент быть восстановлен
func (d *DeletedItem) CanBeRestored() bool {
	return !d.IsRestored && !d.IsPermanentlyDeleted
}

// CanBePermanentlyDeleted проверяет, может ли элемент быть окончательно удален
func (d *DeletedItem) CanBePermanentlyDeleted() bool {
	return !d.IsPermanentlyDeleted
}
