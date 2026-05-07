package models

import "time"

// SkifDailySnapshot — per-day агрегат событий create/delete по юнитам SKIF.
//
// Источник: POST /api_v1/company/updates/query с objects=units (см. wiki/sources/skif-api.md).
// Заполняется через ручной backfill: POST /api/auth/skif/connections/:id/history/backfill.
//
// Используется в lifecycle endpoint как точная история создания/удаления юнитов
// (вместо proxy через skif_units.created_at).
type SkifDailySnapshot struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID  uint      `json:"connection_id"   gorm:"not null;uniqueIndex:uq_skif_ds_conn_comp_date;index"`
	SkifCompanyID string    `json:"skif_company_id" gorm:"not null;uniqueIndex:uq_skif_ds_conn_comp_date;type:varchar(64)"`
	SnapshotDate  time.Time `json:"snapshot_date"   gorm:"not null;uniqueIndex:uq_skif_ds_conn_comp_date;type:date;index"`

	UnitsCreated int `json:"units_created" gorm:"default:0"`
	UnitsDeleted int `json:"units_deleted" gorm:"default:0"`
	UnitsUpdated int `json:"units_updated" gorm:"default:0"` // PUT events за день — для аудита

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Connection *SkifConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (SkifDailySnapshot) TableName() string { return "skif_daily_snapshots" }
