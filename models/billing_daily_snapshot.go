package models

import "time"

// BillingDailySnapshot хранит агрегированные показатели по объектам для биллинга
type BillingDailySnapshot struct {
	ID                     uint      `json:"id" gorm:"primarykey"`
	SnapshotDate           time.Time `json:"snapshot_date" gorm:"type:date;uniqueIndex:idx_billing_snapshot_date;not null"`
	CreatedToday           int       `json:"created_today" gorm:"not null;default:0"`
	TotalObjectsCumulative int       `json:"total_objects_cumulative" gorm:"not null;default:0"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// TableName задает имя таблицы для снимков биллинга
func (BillingDailySnapshot) TableName() string {
	return "billing_daily_snapshots"
}
