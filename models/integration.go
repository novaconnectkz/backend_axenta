package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Integration представляет модель интеграции с внешними системами
type Integration struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	CompanyID       uuid.UUID `json:"company_id" gorm:"type:uuid;not null;index"`
	IntegrationType string    `json:"integration_type" gorm:"not null;type:varchar(50)"` // axetna_cloud, bitrix24, 1c
	Name            string    `json:"name" gorm:"not null;type:varchar(100)"`
	Description     string    `json:"description" gorm:"type:text"`

	// Настройки интеграции (JSON)
	Settings string `json:"settings" gorm:"type:text"` // Зашифрованные настройки подключения

	// Статус интеграции
	IsActive     bool       `json:"is_active" gorm:"default:true"`
	LastSyncAt   *time.Time `json:"last_sync_at"`
	LastErrorAt  *time.Time `json:"last_error_at"`
	ErrorMessage string     `json:"error_message" gorm:"type:text"`

	// Статистика
	SyncCount    int `json:"sync_count" gorm:"default:0"`
	ErrorCount   int `json:"error_count" gorm:"default:0"`
	SuccessCount int `json:"success_count" gorm:"default:0"`

	// Связи
	Company *Company `json:"company,omitempty" gorm:"foreignKey:CompanyID"`
}

// TableName задает имя таблицы для модели Integration
func (Integration) TableName() string {
	return "integrations"
}

// IsHealthy проверяет, работает ли интеграция нормально
func (i *Integration) IsHealthy() bool {
	if !i.IsActive {
		return false
	}

	// Если последняя ошибка была недавно (в течение часа), считаем интеграцию нездоровой
	if i.LastErrorAt != nil && time.Since(*i.LastErrorAt) < time.Hour {
		return false
	}

	// Если синхронизация не выполнялась долго (более 24 часов), это может быть проблемой
	if i.LastSyncAt != nil && time.Since(*i.LastSyncAt) > 24*time.Hour {
		return false
	}

	return true
}

// GetSuccessRate возвращает процент успешных синхронизаций
func (i *Integration) GetSuccessRate() float64 {
	if i.SyncCount == 0 {
		return 0.0
	}

	return float64(i.SuccessCount) / float64(i.SyncCount) * 100.0
}

// UpdateStats обновляет статистику интеграции
func (i *Integration) UpdateStats(success bool, errorMessage string) {
	i.SyncCount++
	now := time.Now()
	i.LastSyncAt = &now

	if success {
		i.SuccessCount++
		i.ErrorMessage = ""
		i.LastErrorAt = nil
	} else {
		i.ErrorCount++
		i.ErrorMessage = errorMessage
		i.LastErrorAt = &now
	}

	i.UpdatedAt = now
}
