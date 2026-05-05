package services

import (
	"backend_axenta/models"
	"time"

	"gorm.io/gorm"
)

// AnalyticsService — единый источник для агрегатов из partner_daily_snapshots.
//
// Зачем: ранее SUM(active_objects_count) считался в 3+ местах
// (api/dashboard_kpi.go, services/partner_snapshot_scheduler.go) с разной
// фильтрацией — лёгкий drift при изменении логики снимков. Сервис
// инкапсулирует SQL и даёт единые методы.
//
// Источник данных: partner_daily_snapshots — ежедневные агрегаты по контрактам,
// наполняются Partner Snapshot scheduler (00:30 UTC).
type AnalyticsService struct {
	db *gorm.DB
}

// NewAnalyticsService создаёт сервис на указанной БД (обычно tenant DB).
func NewAnalyticsService(db *gorm.DB) *AnalyticsService {
	return &AnalyticsService{db: db}
}

// GetLatestSnapshotDate возвращает максимальную snapshot_date из таблицы.
// Если таблица пустая — нулевое значение time.Time и ok=false.
func (s *AnalyticsService) GetLatestSnapshotDate() (time.Time, bool) {
	var row struct {
		SnapshotDate time.Time
	}
	res := s.db.Table("partner_daily_snapshots").
		Select("snapshot_date").
		Order("snapshot_date DESC").
		Limit(1).
		Scan(&row)
	if res.Error != nil || res.RowsAffected == 0 {
		return time.Time{}, false
	}
	return row.SnapshotDate, true
}

// SnapshotAggregate — пара (total, active) объектов на конкретную дату.
type SnapshotAggregate struct {
	Total  int64
	Active int64
}

// GetTotalAndActiveForDate возвращает SUM(total_objects_count) и
// SUM(active_objects_count) на конкретную дату snapshot_date.
// Если на дату снимков нет — оба 0 (без ошибки).
func (s *AnalyticsService) GetTotalAndActiveForDate(date time.Time) (SnapshotAggregate, error) {
	var agg SnapshotAggregate
	err := s.db.Model(&models.PartnerDailySnapshot{}).
		Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", date.Format("2006-01-02")).
		Select("COALESCE(SUM(total_objects_count), 0) as total, COALESCE(SUM(active_objects_count), 0) as active").
		Row().Scan(&agg.Total, &agg.Active)
	return agg, err
}

// GetActiveCountForDate — упрощённая версия GetTotalAndActiveForDate если
// нужен только active. Использует тот же SQL что и dashboard_kpi.
func (s *AnalyticsService) GetActiveCountForDate(date time.Time) int64 {
	var row struct {
		Active int64
	}
	s.db.Table("partner_daily_snapshots").
		Select("COALESCE(SUM(active_objects_count), 0) as active").
		Where("DATE(snapshot_date) = DATE(?)", date).
		Scan(&row)
	return row.Active
}
