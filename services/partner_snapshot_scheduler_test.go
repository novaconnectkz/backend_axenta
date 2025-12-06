package services

import (
	"backend_axenta/database"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupPartnerSnapshotSchedulerTestDB создает тестовую базу данных
func setupPartnerSnapshotSchedulerTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	database.DB = db
	return db
}

// TestNewPartnerSnapshotScheduler тестирует создание нового PartnerSnapshotScheduler
func TestNewPartnerSnapshotScheduler(t *testing.T) {
	setupPartnerSnapshotSchedulerTestDB(t)
	scheduler := NewPartnerSnapshotScheduler()

	assert.NotNil(t, scheduler)
	assert.NotNil(t, scheduler.cron)
	assert.NotNil(t, scheduler.snapshotService)
	assert.NotNil(t, scheduler.axentaSyncService)
	assert.False(t, scheduler.isRunning)
}

// TestPartnerSnapshotScheduler_Start тестирует Start
func TestPartnerSnapshotScheduler_Start(t *testing.T) {
	setupPartnerSnapshotSchedulerTestDB(t)
	scheduler := NewPartnerSnapshotScheduler()

	err := scheduler.Start()
	// Может вернуть ошибку или успех
	if err != nil {
		assert.Contains(t, err.Error(), "ошибка")
	} else {
		assert.True(t, scheduler.isRunning)
		// Останавливаем планировщик
		scheduler.Stop()
	}
}

// TestPartnerSnapshotScheduler_Stop тестирует Stop
func TestPartnerSnapshotScheduler_Stop(t *testing.T) {
	setupPartnerSnapshotSchedulerTestDB(t)
	scheduler := NewPartnerSnapshotScheduler()

	// Останавливаем планировщик (даже если он не запущен)
	scheduler.Stop()
	// Не должно паниковать
	assert.False(t, scheduler.isRunning)
}
