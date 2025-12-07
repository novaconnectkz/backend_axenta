package services

import (
	"testing"
	"time"

	"backend_axenta/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBillingSnapshotService_RebuildBillingSnapshots(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.BillingDailySnapshot{}, &models.SnapshotJob{}))

	counts := map[string]int{
		"2024-03-14": 1,
		"2024-03-15": 2,
		"2024-03-16": 0,
	}

	countFn := func(date time.Time) (int, error) {
		return counts[date.Format("2006-01-02")], nil
	}

	now := time.Date(2025, 12, 7, 0, 0, 0, 0, time.UTC)
	service := NewBillingSnapshotServiceWithDeps(db, countFn, func() time.Time { return now })

	start := time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 3, 16, 0, 0, 0, 0, time.UTC)

	result, err := service.RebuildBillingSnapshots(start, end)
	require.NoError(t, err)
	assert.Equal(t, 3, result.DaysProcessed)
	assert.Equal(t, 3, result.FinalTotal)

	var snapshots []models.BillingDailySnapshot
	require.NoError(t, db.Order("snapshot_date asc").Find(&snapshots).Error)
	require.Len(t, snapshots, 3)

	assert.Equal(t, 1, snapshots[0].CreatedToday)
	assert.Equal(t, 1, snapshots[0].TotalObjectsCumulative)
	assert.Equal(t, 2, snapshots[1].CreatedToday)
	assert.Equal(t, 3, snapshots[1].TotalObjectsCumulative)
	assert.Equal(t, 0, snapshots[2].CreatedToday)
	assert.Equal(t, 3, snapshots[2].TotalObjectsCumulative)

	// Проверяем идемпотентность: повторный запуск не создает дубликаты
	_, err = service.RebuildBillingSnapshots(start, end)
	require.NoError(t, err)

	var totalRows int64
	require.NoError(t, db.Model(&models.BillingDailySnapshot{}).Count(&totalRows).Error)
	assert.Equal(t, int64(3), totalRows)
}

func TestBillingSnapshotService_RunDailySnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.BillingDailySnapshot{}, &models.SnapshotJob{}))

	start := time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)
	// Базовый снимок, чтобы ежедневная джоба знала точку старта
	require.NoError(t, db.Create(&models.BillingDailySnapshot{
		SnapshotDate:           start,
		CreatedToday:           1,
		TotalObjectsCumulative: 1,
	}).Error)

	counts := map[string]int{
		"2024-03-15": 2,
		"2024-03-16": 1,
	}

	countFn := func(date time.Time) (int, error) {
		return counts[date.Format("2006-01-02")], nil
	}

	fixedNow := time.Date(2024, 3, 16, 12, 0, 0, 0, time.UTC)
	service := NewBillingSnapshotServiceWithDeps(db, countFn, func() time.Time { return fixedNow })

	result, err := service.RunDailySnapshot()
	require.NoError(t, err)

	assert.Len(t, result.ProcessedDates, 2)
	assert.Equal(t, 4, result.FinalTotal) // 1 (старт) + 2 + 1

	var snapshots []models.BillingDailySnapshot
	require.NoError(t, db.Order("snapshot_date asc").Find(&snapshots).Error)
	require.Len(t, snapshots, 3)
	assert.Equal(t, 1, snapshots[0].CreatedToday)
	assert.Equal(t, 1, snapshots[0].TotalObjectsCumulative)
	assert.Equal(t, 2, snapshots[1].CreatedToday)
	assert.Equal(t, 3, snapshots[1].TotalObjectsCumulative)
	assert.Equal(t, 1, snapshots[2].CreatedToday)
	assert.Equal(t, 4, snapshots[2].TotalObjectsCumulative)

	// Проверяем, что задачи истории создаются и обновляются
	var jobCount int64
	require.NoError(t, db.Model(&models.SnapshotJob{}).Where("job_type = ?", models.SnapshotJobTypeDailyAuto).Count(&jobCount).Error)
	assert.Equal(t, int64(2), jobCount)

	// Повторный запуск в тот же день не должен создавать новые снимки
	result, err = service.RunDailySnapshot()
	require.NoError(t, err)
	assert.Len(t, result.ProcessedDates, 0)
}
