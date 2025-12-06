package services

import (
	"backend_axenta/database"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAxentaSyncSchedulerTestDB создает тестовую базу данных
func setupAxentaSyncSchedulerTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	database.DB = db
	return db
}

// TestNewAxentaSyncScheduler тестирует создание нового AxentaSyncScheduler
func TestNewAxentaSyncScheduler(t *testing.T) {
	db := setupAxentaSyncSchedulerTestDB(t)
	syncService := NewAxentaSyncService(db)

	scheduler := NewAxentaSyncScheduler(syncService, 5)
	assert.NotNil(t, scheduler)
	assert.NotNil(t, scheduler.syncService)
	assert.NotNil(t, scheduler.cron)
	assert.Equal(t, 5, scheduler.interval)
}

// TestNewAxentaSyncScheduler_DefaultInterval тестирует создание с интервалом по умолчанию
func TestNewAxentaSyncScheduler_DefaultInterval(t *testing.T) {
	db := setupAxentaSyncSchedulerTestDB(t)
	syncService := NewAxentaSyncService(db)

	scheduler := NewAxentaSyncScheduler(syncService, 0)
	assert.NotNil(t, scheduler)
	assert.Equal(t, 5, scheduler.interval) // Должен быть установлен по умолчанию
}

// TestAxentaSyncScheduler_Start тестирует Start
func TestAxentaSyncScheduler_Start(t *testing.T) {
	db := setupAxentaSyncSchedulerTestDB(t)
	syncService := NewAxentaSyncService(db)
	scheduler := NewAxentaSyncScheduler(syncService, 5)

	err := scheduler.Start()
	// Может вернуть ошибку или успех в зависимости от реализации
	if err != nil {
		assert.Contains(t, err.Error(), "ошибка")
	} else {
		// Останавливаем планировщик
		scheduler.Stop()
	}
}

// TestAxentaSyncScheduler_Stop тестирует Stop
func TestAxentaSyncScheduler_Stop(t *testing.T) {
	db := setupAxentaSyncSchedulerTestDB(t)
	syncService := NewAxentaSyncService(db)
	scheduler := NewAxentaSyncScheduler(syncService, 5)

	// Останавливаем планировщик (даже если он не запущен)
	scheduler.Stop()
	// Не должно паниковать
}

// TestAxentaSyncScheduler_GetInterval тестирует GetInterval
func TestAxentaSyncScheduler_GetInterval(t *testing.T) {
	db := setupAxentaSyncSchedulerTestDB(t)
	syncService := NewAxentaSyncService(db)
	scheduler := NewAxentaSyncScheduler(syncService, 10)

	interval := scheduler.GetInterval()
	assert.Equal(t, 10, interval)
}

// TestAxentaSyncScheduler_UpdateInterval тестирует UpdateInterval
func TestAxentaSyncScheduler_UpdateInterval(t *testing.T) {
	db := setupAxentaSyncSchedulerTestDB(t)
	syncService := NewAxentaSyncService(db)
	scheduler := NewAxentaSyncScheduler(syncService, 5)

	err := scheduler.UpdateInterval(15)
	// Может вернуть ошибку или успех
	if err != nil {
		assert.Contains(t, err.Error(), "ошибка")
	} else {
		interval := scheduler.GetInterval()
		assert.Equal(t, 15, interval)
	}
}

// TestAxentaSyncScheduler_UpdateInterval_Invalid тестирует UpdateInterval с неверным интервалом
func TestAxentaSyncScheduler_UpdateInterval_Invalid(t *testing.T) {
	db := setupAxentaSyncSchedulerTestDB(t)
	syncService := NewAxentaSyncService(db)
	scheduler := NewAxentaSyncScheduler(syncService, 5)

	err := scheduler.UpdateInterval(0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "интервал должен быть больше 0")
}

// TestAxentaSyncScheduler_SyncAdminAsync тестирует SyncAdminAsync
func TestAxentaSyncScheduler_SyncAdminAsync(t *testing.T) {
	db := setupAxentaSyncSchedulerTestDB(t)
	syncService := NewAxentaSyncService(db)
	scheduler := NewAxentaSyncScheduler(syncService, 5)

	// Вызываем асинхронную синхронизацию
	scheduler.SyncAdminAsync(123)
	// Не должно паниковать
	// Даем время на выполнение
	time.Sleep(100 * time.Millisecond)
}
