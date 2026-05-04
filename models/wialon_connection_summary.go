package models

import "time"

// WialonConnectionSummary — точные счётчики объектов от top-level resource подключения.
//
// Заполняется из `account/get_account_data` на главный resource (user.bact). Числа
// отражают то что Wialon CMS Manager показывает на корневом аккаунте — без
// per-resource SUM-overcount и без proportion-приближений.
//
// Используется в /api/auth/dashboard/sources-stats для WH/WL active/inactive
// breakdown. Distinct unit count берётся из wialon_units.
type WialonConnectionSummary struct {
	ConnectionID uint `json:"connection_id" gorm:"primarykey"`

	ObjectsTotal    int `json:"objects_total"    gorm:"default:0"`
	ObjectsActive   int `json:"objects_active"   gorm:"default:0"` // activated_units.usage
	ObjectsInactive int `json:"objects_inactive" gorm:"default:0"` // seasonal_units.usage

	LastCollectedAt time.Time `json:"last_collected_at" gorm:"index"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	Connection *WialonConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (WialonConnectionSummary) TableName() string {
	return "wialon_connection_summary"
}
