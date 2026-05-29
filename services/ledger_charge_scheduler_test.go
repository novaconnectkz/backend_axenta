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
	"gorm.io/gorm/logger"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestDaysHelpers(t *testing.T) {
	assert.Equal(t, 31, daysInMonth(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, 28, daysInMonth(time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, 29, daysInMonth(time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC))) // високосный
	assert.Equal(t, 30, daysInMonth(time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, 365, daysInYear(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, 366, daysInYear(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)))

	// dayFloor обрезает время до 00:00 UTC.
	f := dayFloor(time.Date(2026, 5, 28, 13, 45, 12, 0, time.UTC))
	assert.Equal(t, time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC), f)
}

// Codex-чек-лист: сумма дневных charge за месяц = месячная (drift в копейки приемлем).
func TestDailyChargeNoDrift_FullMonth(t *testing.T) {
	price := d("1200") // 1 объект, 1200/мес
	// Январь 2026 — 31 день.
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sum := decimal.Zero
	for day := first; day.Month() == 1; day = day.AddDate(0, 0, 1) {
		sum = sum.Add(dailyChargeAmount(price, 1, day, false))
	}
	// 1200/31 = 38.71 (round) × 31 = 1200.01 — drift ≤ 1 коп.
	diff := sum.Sub(price).Abs()
	assert.True(t, diff.LessThanOrEqual(d("0.31")), "drift за месяц должен быть в пределах копеек, got sum=%s", sum.String())
}

// Цена×объекты делится корректно; разный состав даёт разную дневную сумму.
func TestDailyChargeAmount_Variations(t *testing.T) {
	day := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC) // апрель, 30 дней
	// 600₽ × 2 объекта / 30 = 40.00
	assert.True(t, dailyChargeAmount(d("600"), 2, day, false).Equal(d("40")))
	// 0 объектов → 0
	assert.True(t, dailyChargeAmount(d("600"), 0, day, false).IsZero())
	// нулевая цена → 0
	assert.True(t, dailyChargeAmount(d("0"), 5, day, false).IsZero())
	// yearly: 3650₽ × 1 / 365 = 10.00
	yday := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	assert.True(t, dailyChargeAmount(d("3650"), 1, yday, true).Equal(d("10")))
}

// holdLifecycleAction — pure-решение жизненного цикла зонта (П3/П4) в sweep.
// Покрываем все ветки: fulfill / expire / keep + приоритет fulfill над expire.
func TestHoldLifecycleAction(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)  // зонт ещё держит
	past := now.Add(-1 * time.Hour)    // срок истёк
	threshold := d("-1000")            // допустимый минус = −creditLimit (creditLimit=1000)

	cases := []struct {
		name      string
		balance   decimal.Decimal
		holdUntil time.Time
		want      holdAction
	}{
		// Долг в пределах лимита (balance >= threshold) → fulfill, независимо от срока.
		{"в долгу но в пределах лимита, срок есть", d("-500"), future, holdFulfill},
		{"переплата, срок есть", d("300"), future, holdFulfill},
		{"ровно на пороге (balance==threshold)", d("-1000"), future, holdFulfill},
		{"вышел из долга в день истечения → fulfill приоритетнее expire", d("0"), past, holdFulfill},
		// В долгу сверх лимита + срок истёк → expire.
		{"глубоко в долгу, срок истёк", d("-5000"), past, holdExpire},
		{"чуть за порогом, срок истёк", d("-1000.01"), past, holdExpire},
		{"в долгу, срок ровно now (now>=holdUntil)", d("-5000"), now, holdExpire},
		// В долгу сверх лимита + срок не истёк → keep (зонт держит).
		{"глубоко в долгу, срок есть", d("-5000"), future, holdKeep},
		{"чуть за порогом, срок есть", d("-1000.01"), future, holdKeep},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := holdLifecycleAction(tc.balance, threshold, tc.holdUntil, now)
			assert.Equal(t, tc.want, got, "balance=%s holdUntil=%s", tc.balance, tc.holdUntil.Format(time.RFC3339))
		})
	}
}

func setupChargeTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ContractObject{}))
	return db
}

// Codex-чек-лист: исторический состав объектов на день (created_at/deleted_at + привязка).
func TestObjectCountOnDay_Historical(t *testing.T) {
	db := setupChargeTestDB(t)
	s := &LedgerChargeScheduler{}
	subID := uint(100)

	mk := func(created time.Time, deleted *time.Time, start time.Time, end *time.Time, status string) {
		co := models.ContractObject{
			ContractID:      1,
			SubscriptionID:  &subID,
			ObjectID:        uint(time.Now().UnixNano() % 1000000),
			ObjectCompanyID: 186,
			ObjectSchema:    "tenant_186",
			Status:          status,
			StartDate:       start,
			EndDate:         end,
		}
		require.NoError(t, db.Create(&co).Error)
		// created_at/deleted_at выставляем явно (BeforeCreate ставит attached_at, не трогает created_at у sqlite — перезапишем).
		require.NoError(t, db.Model(&models.ContractObject{}).Where("id = ?", co.ID).
			Updates(map[string]interface{}{"created_at": created, "deleted_at": deleted}).Error)
	}

	jan := func(d int) time.Time { return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC) }

	// Объект A: существует весь месяц.
	mk(jan(1), nil, jan(1), nil, "active")
	// Объект B: добавлен 16-го (created_at 16-го).
	mk(jan(16), nil, jan(16), nil, "active")
	// Объект C: удалён 20-го (deleted_at 20-го).
	del20 := jan(20)
	mk(jan(1), &del20, jan(1), &del20, "active")
	// Объект D: status inactive — не биллится.
	mk(jan(1), nil, jan(1), nil, "inactive")

	cnt := func(day time.Time) int {
		c, err := s.objectCountOnDay(db, subID, day)
		assert.NoError(t, err)
		return c
	}
	// 10-е: A + C активны (B ещё не добавлен, D inactive) = 2.
	assert.Equal(t, 2, cnt(jan(10)))
	// 18-е: A + B + C = 3.
	assert.Equal(t, 3, cnt(jan(18)))
	// 25-е: A + B (C удалён 20-го) = 2.
	assert.Equal(t, 2, cnt(jan(25)))
}
