package models

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newVerifyTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&PartnerDailySnapshot{}))
	return db
}

// базовый снимок-шаблон (skif-партнёр, тариф 70₽/мес).
func baseSnap(date time.Time, active int) PartnerDailySnapshot {
	return PartnerDailySnapshot{
		AdminAccountID:       1,
		CompanyID:            186,
		ContractID:           444,
		SnapshotDate:         date,
		PartnerSource:        "skif",
		ConnectionID:         8,
		PartnerExternalID:    "dealer-1",
		TariffPlanID:         1,
		MonthlyPrice:         decimal.NewFromInt(70),
		TotalObjectsCount:    active,
		ActiveObjectsCount:   active,
		VerifySecondaryCount: -1,
	}
}

func TestComputeCosts_DaysInMonth(t *testing.T) {
	// Июнь = 30 дней, тариф 70 → дневная 2.3333, active 717 → 1672.97/.98.
	s := baseSnap(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), 717)
	s.ComputeCosts()
	assert.Equal(t, "2.3333", s.DailyPrice.StringFixed(4))
	// 2.3333 * 717 = 1672.9761 → округление .Round(2) = 1672.98
	assert.Equal(t, "1672.98", s.DailyCost.StringFixed(2))

	// Февраль 2026 = 28 дней → делитель меняется (НЕ хардкод /30).
	f := baseSnap(time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), 100)
	f.ComputeCosts()
	assert.Equal(t, "2.5000", f.DailyPrice.StringFixed(4)) // 70/28
}

func TestVerifyGuard_FirstDayNoBaseline(t *testing.T) {
	db := newVerifyTestDB(t)
	s := baseSnap(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), 700)
	require.NoError(t, db.Create(&s).Error)
	assert.Equal(t, VerifyStatusVerified, s.VerifyStatus, "первый день без baseline → verified")
	assert.Equal(t, -1, s.PrevActiveCount)
	assert.NotNil(t, s.VerifiedAt)
}

func TestVerifyGuard_WithinTolerance(t *testing.T) {
	db := newVerifyTestDB(t)
	prev := baseSnap(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), 700)
	require.NoError(t, db.Create(&prev).Error)
	require.Equal(t, VerifyStatusVerified, prev.VerifyStatus)

	cur := baseSnap(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), 705) // +5, в пределах
	require.NoError(t, db.Create(&cur).Error)
	assert.Equal(t, VerifyStatusVerified, cur.VerifyStatus)
	assert.Equal(t, 700, cur.PrevActiveCount)
}

func TestVerifyGuard_Zeroing(t *testing.T) {
	db := newVerifyTestDB(t)
	prev := baseSnap(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), 700)
	require.NoError(t, db.Create(&prev).Error)

	cur := baseSnap(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), 0) // обнуление
	require.NoError(t, db.Create(&cur).Error)
	assert.Equal(t, VerifyStatusNeedsRev, cur.VerifyStatus)
	assert.True(t, cur.AmountAtRisk.GreaterThan(decimal.Zero), "amount_at_risk = потерянный day-cost")
}

func TestVerifyGuard_Spike(t *testing.T) {
	db := newVerifyTestDB(t)
	prev := baseSnap(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), 100)
	require.NoError(t, db.Create(&prev).Error)

	cur := baseSnap(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), 200) // +100 (>15) и +100% (>25%)
	require.NoError(t, db.Create(&cur).Error)
	assert.Equal(t, VerifyStatusNeedsRev, cur.VerifyStatus)
}

func TestVerifyGuard_SmallCountNoFalsePositive(t *testing.T) {
	db := newVerifyTestDB(t)
	prev := baseSnap(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), 2)
	require.NoError(t, db.Create(&prev).Error)

	cur := baseSnap(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), 4) // +100% но абс=2 < 15 → НЕ флаг
	require.NoError(t, db.Create(&cur).Error)
	assert.Equal(t, VerifyStatusVerified, cur.VerifyStatus, "малые числа не должны давать ложный needs_review")
}

func TestVerifyGuard_CrossSourceMismatch(t *testing.T) {
	db := newVerifyTestDB(t)
	prev := baseSnap(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), 700)
	require.NoError(t, db.Create(&prev).Error)

	cur := baseSnap(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), 700)
	cur.VerifySecondaryCount = 500 // второй счёт расходится: абс 200 ≥15 и 28.6% ≥25%
	require.NoError(t, db.Create(&cur).Error)
	assert.Equal(t, VerifyStatusNeedsRev, cur.VerifyStatus)
}

func TestVerifyGuard_SourceWarn(t *testing.T) {
	db := newVerifyTestDB(t)
	s := baseSnap(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), 0)
	s.SourceWarn = "force-sync упал"
	require.NoError(t, db.Create(&s).Error)
	assert.Equal(t, VerifyStatusNeedsRev, s.VerifyStatus)
	assert.Contains(t, s.VerifyNotes, "force-sync")
}

func TestVerifyGuard_Estimated(t *testing.T) {
	db := newVerifyTestDB(t)
	prev := baseSnap(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), 700)
	require.NoError(t, db.Create(&prev).Error)

	cur := baseSnap(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), 700)
	cur.IsEstimated = true // backfill без истории
	require.NoError(t, db.Create(&cur).Error)
	assert.Equal(t, VerifyStatusEstimated, cur.VerifyStatus, "estimated не должен стать verified (блокируется в billing)")
}

func TestCountsDiverge(t *testing.T) {
	assert.False(t, countsDiverge(700, 700))
	assert.False(t, countsDiverge(700, 690)) // абс 10 < 15
	assert.False(t, countsDiverge(2, 4))     // мелочь
	assert.False(t, countsDiverge(700, 600)) // абс 100 ≥15, но 100/700=14.3% < 25% → НЕ расхождение
	assert.True(t, countsDiverge(100, 200))  // абс 100 ≥15 и 100% ≥25% → расхождение
	assert.True(t, countsDiverge(100, 50))   // абс 50 ≥15 и 50% ≥25% → расхождение
}

func TestPctDelta(t *testing.T) {
	assert.Equal(t, "100.00", pctDelta(0, 5).StringFixed(2))
	assert.Equal(t, "0.00", pctDelta(0, 0).StringFixed(2))
	assert.Equal(t, "100.00", pctDelta(100, 200).StringFixed(2))
	assert.Equal(t, "-50.00", pctDelta(100, 50).StringFixed(2))
}
