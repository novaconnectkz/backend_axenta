package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupPartnerSnapshotServiceTestDB создает тестовую базу данных для partner snapshot service
func setupPartnerSnapshotServiceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Contract{},
		&models.PartnerDailySnapshot{},
		&models.BillingPlan{},
		&models.Company{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// TestNewPartnerSnapshotService тестирует создание нового сервиса
func TestNewPartnerSnapshotService(t *testing.T) {
	setupPartnerSnapshotServiceTestDB(t)

	service := NewPartnerSnapshotService()
	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
}

// TestPartnerSnapshotService_CreateDailySnapshots_NoContracts тестирует CreateDailySnapshots без договоров
func TestPartnerSnapshotService_CreateDailySnapshots_NoContracts(t *testing.T) {
	setupPartnerSnapshotServiceTestDB(t)
	service := NewPartnerSnapshotService()

	err := service.CreateDailySnapshots()
	// Должно вернуть nil, так как нет договоров
	assert.NoError(t, err)
}

// TestPartnerSnapshotService_CreateSnapshotForContract_NoToken тестирует CreateSnapshotForContract без токена
func TestPartnerSnapshotService_CreateSnapshotForContract_NoToken(t *testing.T) {
	setupPartnerSnapshotServiceTestDB(t)
	service := NewPartnerSnapshotService()

	contract := &models.Contract{
		ID:             1,
		AdminAccountID: 123,
		Number:         "T-001",
		ContractType:   "partner",
	}

	err := service.CreateSnapshotForContract(contract, time.Now())
	// Должно вернуть nil, так как токен не установлен
	assert.NoError(t, err)
}

// TestPartnerSnapshotService_GetSnapshotsForContract_NoSnapshots тестирует GetSnapshotsForContract без снимков
func TestPartnerSnapshotService_GetSnapshotsForContract_NoSnapshots(t *testing.T) {
	setupPartnerSnapshotServiceTestDB(t)
	service := NewPartnerSnapshotService()

	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)

	snapshots, err := service.GetSnapshotsForContract(1, startDate, endDate)
	require.NoError(t, err)
	assert.Empty(t, snapshots)
}

// TestPartnerSnapshotService_GetSnapshotsForContract_WithSnapshots тестирует GetSnapshotsForContract со снимками
func TestPartnerSnapshotService_GetSnapshotsForContract_WithSnapshots(t *testing.T) {
	db := setupPartnerSnapshotServiceTestDB(t)
	service := NewPartnerSnapshotService()

	// Создаем снимки
	snapshot1 := models.PartnerDailySnapshot{
		ContractID:         1,
		AdminAccountID:     123,
		SnapshotDate:       time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		TotalObjectsCount:  100,
		ActiveObjectsCount: 90,
		DailyCost:          decimal.NewFromInt(1000),
	}
	snapshot2 := models.PartnerDailySnapshot{
		ContractID:         1,
		AdminAccountID:     123,
		SnapshotDate:       time.Date(2025, 1, 16, 0, 0, 0, 0, time.UTC),
		TotalObjectsCount:  110,
		ActiveObjectsCount: 100,
		DailyCost:          decimal.NewFromInt(1100),
	}
	db.Create(&snapshot1)
	db.Create(&snapshot2)

	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)

	snapshots, err := service.GetSnapshotsForContract(1, startDate, endDate)
	require.NoError(t, err)
	assert.Len(t, snapshots, 2)
}

// TestPartnerSnapshotService_CalculateCostForPeriod_NoSnapshots тестирует CalculateCostForPeriod без снимков
func TestPartnerSnapshotService_CalculateCostForPeriod_NoSnapshots(t *testing.T) {
	setupPartnerSnapshotServiceTestDB(t)
	service := NewPartnerSnapshotService()

	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)

	cost, err := service.CalculateCostForPeriod(1, startDate, endDate)
	require.NoError(t, err)
	assert.True(t, cost.IsZero())
}

// TestPartnerSnapshotService_CalculateCostForPeriod_WithSnapshots тестирует CalculateCostForPeriod со снимками
func TestPartnerSnapshotService_CalculateCostForPeriod_WithSnapshots(t *testing.T) {
	db := setupPartnerSnapshotServiceTestDB(t)
	service := NewPartnerSnapshotService()

	// Создаем снимки
	snapshot1 := models.PartnerDailySnapshot{
		ContractID:         1,
		AdminAccountID:     123,
		SnapshotDate:       time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		TotalObjectsCount:  100,
		ActiveObjectsCount: 90,
		DailyCost:          decimal.NewFromInt(1000),
	}
	snapshot2 := models.PartnerDailySnapshot{
		ContractID:         1,
		AdminAccountID:     123,
		SnapshotDate:       time.Date(2025, 1, 16, 0, 0, 0, 0, time.UTC),
		TotalObjectsCount:  110,
		ActiveObjectsCount: 100,
		DailyCost:          decimal.NewFromInt(1100),
	}
	db.Create(&snapshot1)
	db.Create(&snapshot2)

	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)

	cost, err := service.CalculateCostForPeriod(1, startDate, endDate)
	require.NoError(t, err)
	assert.Equal(t, decimal.NewFromInt(2100), cost) // 1000 + 1100
}

// TestPartnerSnapshotService_GetSnapshotsForContract_DateFilter тестирует GetSnapshotsForContract с фильтром по датам
func TestPartnerSnapshotService_GetSnapshotsForContract_DateFilter(t *testing.T) {
	db := setupPartnerSnapshotServiceTestDB(t)
	service := NewPartnerSnapshotService()

	// Создаем снимки за разные периоды
	snapshot1 := models.PartnerDailySnapshot{
		ContractID:         1,
		AdminAccountID:     123,
		SnapshotDate:       time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		TotalObjectsCount:  100,
		ActiveObjectsCount: 90,
		DailyCost:          decimal.NewFromInt(1000),
	}
	snapshot2 := models.PartnerDailySnapshot{
		ContractID:         1,
		AdminAccountID:     123,
		SnapshotDate:       time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC), // Вне периода
		TotalObjectsCount:  110,
		ActiveObjectsCount: 100,
		DailyCost:          decimal.NewFromInt(1100),
	}
	db.Create(&snapshot1)
	db.Create(&snapshot2)

	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)

	snapshots, err := service.GetSnapshotsForContract(1, startDate, endDate)
	require.NoError(t, err)
	assert.Len(t, snapshots, 1) // Только снимок за январь
	assert.Equal(t, time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC), snapshots[0].SnapshotDate)
}

// zone#1: save-loop happy-path + idempotency + timeout-helper. До этого save-цикл
// (savePartnerObjectsToDB) имел ноль покрытия; тест фиксирует что context-обёрнутый
// OnConflict Create персистит строки без ложного timeout, и повтор не плодит дубли.
func TestSavePartnerObjectsToDB_HappyPathAndIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AxentaObjectSnapshot{}))
	s := &PartnerSnapshotService{}
	snapDate := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)

	objs := []axentaObject{
		{ID: 101, Name: "Объект А", UniqueID: "u-101", AccountID: 10, AccountName: "ACC", IsActive: true, Status: "active"},
		{ID: 102, Name: "Объект Б", UniqueID: "u-102", AccountID: 10, AccountName: "ACC", IsActive: false, Status: "blocked"},
	}

	// Первый прогон: 2 строки сохранены, без ошибки/timeout.
	require.NoError(t, s.savePartnerObjectsToDB(186, objs, snapDate, db))
	var cnt int64
	db.Model(&models.AxentaObjectSnapshot{}).Count(&cnt)
	assert.EqualValues(t, 2, cnt, "две строки персистнуты")

	// Идемпотентность: повтор с обновлённым именем → OnConflict update, без дублей.
	objs[0].Name = "Объект А (упд)"
	require.NoError(t, s.savePartnerObjectsToDB(186, objs, snapDate, db))
	db.Model(&models.AxentaObjectSnapshot{}).Count(&cnt)
	assert.EqualValues(t, 2, cnt, "повтор не плодит дубли (OnConflict по external_object_id)")
	var upd models.AxentaObjectSnapshot
	db.Where("external_object_id = ?", 101).First(&upd)
	assert.Equal(t, "Объект А (упд)", upd.ObjectName, "имя обновлено через OnConflict")
}

func TestSnapshotDBStmtTimeout_Default(t *testing.T) {
	t.Setenv("SNAPSHOT_DB_STMT_TIMEOUT_SEC", "")
	assert.Equal(t, 120*time.Second, snapshotDBStmtTimeout(), "дефолт 120s")
	t.Setenv("SNAPSHOT_DB_STMT_TIMEOUT_SEC", "30")
	assert.Equal(t, 30*time.Second, snapshotDBStmtTimeout(), "env override 30s")
	t.Setenv("SNAPSHOT_DB_STMT_TIMEOUT_SEC", "0")
	assert.Equal(t, 120*time.Second, snapshotDBStmtTimeout(), "невалидный 0 → дефолт")
}
