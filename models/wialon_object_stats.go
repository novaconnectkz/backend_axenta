package models

import "time"

// WialonObjectStat — кэш статистики объектов Wialon в БД.
// Заполняется фоновым планировщиком (WialonStatsScheduler), читается endpoint
// /api/wialon/connections/:id/objects-stats. Не глобальная — у каждого тенанта
// свои wialon_connections, эта таблица в public-схеме т.к. ссылается на
// wialon_connections (которая глобальная).
type WialonObjectStat struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID uint  `json:"connection_id" gorm:"not null;index;uniqueIndex:uq_wialon_object_stats_conn_res"`
	ResourceID   int64 `json:"resource_id"   gorm:"not null;uniqueIndex:uq_wialon_object_stats_conn_res"` // avl_resource.id в Wialon
	UserID       int64 `json:"user_id"       gorm:"index"`                                                // user.id, по которому фронт сопоставляет аккаунт

	ObjectsTotal       int `json:"objects_total"       gorm:"not null;default:0"`
	ObjectsActive      int `json:"objects_active"      gorm:"not null;default:0"`
	ObjectsDeactivated int `json:"objects_deactivated" gorm:"not null;default:0"`

	LastCollectedAt time.Time `json:"last_collected_at" gorm:"not null;index"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	Connection *WialonConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (WialonObjectStat) TableName() string {
	return "wialon_object_stats"
}
