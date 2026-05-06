package models

import "time"

// WialonDailySnapshot — точная история объектов Wialon на конкретную дату.
//
// Заполняется через on-demand backfill (см. WialonHistoryService): при нажатии
// "Пересчитать историю" в Настройках → backend дёргает Wialon API
// `core/get_statistics` для каждого аккаунта, делает обратный walk от текущего
// usage'а к прошлому через created/deleted per day.
//
// Используется в /api/auth/dashboard/chart как точный источник вместо proxy
// через wialon_units.created_at (которое отражает только "когда scheduler
// впервые увидел unit", а не реальную историю Wialon).
//
// Глобальная таблица в public-схеме (как wialon_units).
type WialonDailySnapshot struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID    uint      `json:"connection_id"     gorm:"not null;index;uniqueIndex:uq_wds_conn_acc_date"`
	AccountWialonID int64     `json:"account_wialon_id" gorm:"not null;uniqueIndex:uq_wds_conn_acc_date"`
	SnapshotDate    time.Time `json:"snapshot_date"     gorm:"type:date;not null;uniqueIndex:uq_wds_conn_acc_date;index"`

	TotalUnits       int `json:"total_units"       gorm:"not null;default:0"` // активных на конец дня
	UnitsCreated     int `json:"units_created"     gorm:"default:0"`          // создано за день
	UnitsDeleted     int `json:"units_deleted"     gorm:"default:0"`          // удалено за день
	UnitsDeactivated int `json:"units_deactivated" gorm:"default:0"`          // деактивированных на конец дня

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Connection *WialonConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (WialonDailySnapshot) TableName() string {
	return "wialon_daily_snapshots"
}

// WialonHistorySettings — настройки on-demand backfill истории Wialon (per-company).
type WialonHistorySettings struct {
	ID        uint `json:"id" gorm:"primarykey"`
	CompanyID uint `json:"company_id" gorm:"not null;uniqueIndex"`

	Enabled bool `json:"enabled" gorm:"default:false"` // Включён ли расчёт истории для company

	LastBackfillFrom *time.Time `json:"last_backfill_from"` // Период последнего успешного backfill
	LastBackfillTo   *time.Time `json:"last_backfill_to"`
	LastBackfillAt   *time.Time `json:"last_backfill_at"`
	LastBackfillRows int        `json:"last_backfill_rows"` // Сколько записей создано/обновлено

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (WialonHistorySettings) TableName() string {
	return "wialon_history_settings"
}
