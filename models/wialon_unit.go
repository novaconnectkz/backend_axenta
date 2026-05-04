package models

import "time"

// WialonUnit — одна запись на (connection, unit). UNIQUE(connection_id, unit_id) гарантирует
// дедупликацию: если у одного user_id несколько билинг-resource'ов и они "видят" один и тот
// же объект, в таблице будет одна строка — не дубль на каждый resource.
//
// Используется для точного COUNT(DISTINCT unit_id) на уровне connection в
// /api/auth/dashboard/sources-stats. Старая таблица wialon_object_stats остаётся —
// /accounts read-path использует per-resource агрегаты с user_id-ключом.
//
// Глобальная таблица в public-схеме (как wialon_object_stats), т.к. ссылается на
// wialon_connections (которая глобальная).
type WialonUnit struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID uint  `json:"connection_id" gorm:"not null;index;uniqueIndex:uq_wialon_units_conn_unit"`
	UnitID       int64 `json:"unit_id"       gorm:"not null;uniqueIndex:uq_wialon_units_conn_unit"` // avl_unit.id в Wialon

	Name      string `json:"name"       gorm:"type:varchar(255)"`
	IsActive  bool   `json:"is_active"  gorm:"index;default:true"`
	CreatorID int64  `json:"creator_id" gorm:"index"` // avl_unit.crt — кто создал юнит
	BillingID int64  `json:"billing_id" gorm:"index"` // avl_unit.bact — billing-resource

	LastCollectedAt time.Time `json:"last_collected_at" gorm:"not null;index"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	Connection *WialonConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (WialonUnit) TableName() string {
	return "wialon_units"
}
