package services

import (
	"testing"
	"time"

	"backend_axenta/models"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Регресс Codex-critical: manual_approved снимок не должен сниматься ночным cron.
// Проверяем guard partnerSnapshotIsApproved, на котором держится skip во всех 4 источниках.
func TestPartnerSnapshotIsApproved_Freeze(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.PartnerDailySnapshot{}))

	day := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	snap := models.PartnerDailySnapshot{
		AdminAccountID:       1,
		CompanyID:            186,
		ContractID:           444,
		SnapshotDate:         day,
		PartnerSource:        "skif",
		ConnectionID:         8,
		PartnerExternalID:    "dealer-1",
		TariffPlanID:         1,
		MonthlyPrice:         decimal.NewFromInt(70),
		ActiveObjectsCount:   700,
		VerifySecondaryCount: -1,
	}
	require.NoError(t, db.Create(&snap).Error) // BeforeCreate → verified

	// Сначала НЕ подтверждён.
	assert.False(t, partnerSnapshotIsApproved(db, "skif", 8, "dealer-1", 444, day))

	// Эмулируем approve endpoint (Updates по map, без хуков).
	require.NoError(t, db.Model(&models.PartnerDailySnapshot{}).
		Where("id = ?", snap.ID).
		Updates(map[string]interface{}{"verify_status": models.VerifyStatusApproved}).Error)

	// Теперь guard видит подтверждение → cron сделает continue (не перезатрёт).
	assert.True(t, partnerSnapshotIsApproved(db, "skif", 8, "dealer-1", 444, day))

	// Другой день того же договора — не подтверждён (изоляция по дате).
	assert.False(t, partnerSnapshotIsApproved(db, "skif", 8, "dealer-1", 444, day.AddDate(0, 0, 1)))
	// Другой источник — не подтверждён (изоляция по source).
	assert.False(t, partnerSnapshotIsApproved(db, "wialon", 8, "dealer-1", 444, day))
}
