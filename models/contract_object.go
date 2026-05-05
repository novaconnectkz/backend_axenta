package models

import (
	"time"

	"gorm.io/gorm"
)

// ContractObject представляет связь между договором и объектом
// Используется для связи объектов из разных tenant схем с договорами
type ContractObject struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связь с договором (находится в схеме компании-создателя)
	ContractID uint     `json:"contract_id" gorm:"not null;index"`
	Contract   Contract `json:"contract,omitempty" gorm:"foreignKey:ContractID;constraint:-"`

	// Связь с подпиской (опционально, для отслеживания какой подпиской был добавлен объект)
	SubscriptionID *uint `json:"subscription_id" gorm:"index"` // ID подписки, которая добавила этот объект

	// Информация об объекте (может находиться в другой tenant схеме)
	ObjectID        uint   `json:"object_id" gorm:"not null;index"`
	ObjectCompanyID uint   `json:"object_company_id" gorm:"not null;index"`         // ID компании, которой принадлежит объект
	ObjectSchema    string `json:"object_schema" gorm:"not null;type:varchar(100)"` // Схема, где находится объект (tenant_1803)

	// Метаданные связи
	AttachedAt *time.Time `json:"attached_at"`                                     // Дата привязки объекта
	Status     string     `json:"status" gorm:"default:'active';type:varchar(20)"` // active, inactive, suspended
	Notes      string     `json:"notes" gorm:"type:text"`                          // Дополнительные заметки

	// Сроки привязки объекта к договору
	// Объект может быть привязан к одному договору на определенный период
	// Повторная привязка возможна только на другой срок (без пересечений)
	StartDate time.Time  `json:"start_date" gorm:"not null"`   // Дата начала привязки (обычно совпадает с StartDate договора)
	EndDate   *time.Time `json:"end_date" gorm:"default:NULL"` // Дата окончания привязки (опционально, обычно совпадает с EndDate договора)
}

// TableName задает имя таблицы для модели ContractObject
func (ContractObject) TableName() string {
	return "contract_objects"
}

// BeforeCreate устанавливает дату привязки перед созданием
func (co *ContractObject) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	co.AttachedAt = &now

	// Если сроки не установлены, устанавливаем их на текущую дату
	if co.StartDate.IsZero() {
		co.StartDate = now
	}
	// EndDate опционален - будет установлен через подписку, не устанавливаем автоматически

	return nil
}

// HasDateOverlap проверяет, пересекаются ли сроки привязки с другим ContractObject
func (co *ContractObject) HasDateOverlap(other *ContractObject) bool {
	// Проверяем пересечение периодов: [start1, end1] и [start2, end2]
	// Периоды пересекаются, если start1 <= end2 && start2 <= end1
	// Если у одного из объектов нет end_date, проверяем только пересечение с началом
	if co.EndDate == nil || other.EndDate == nil {
		// Если у одного из объектов нет end_date, считаем что пересечения нет (период будет установлен через подписку)
		return false
	}
	return !co.StartDate.After(*other.EndDate) && !other.StartDate.After(*co.EndDate)
}
