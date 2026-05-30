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
	future := now.Add(24 * time.Hour) // зонт ещё держит
	past := now.Add(-1 * time.Hour)   // срок истёк
	threshold := d("-1000")           // допустимый минус = −creditLimit (creditLimit=1000)

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

// convertAmount: конверсия суммы плана в валюту договора по курсу (П5 фаза 2).
func TestConvertAmount(t *testing.T) {
	// 15.50 EUR × 83.6892 = 1297.18 (round 2).
	assert.True(t, convertAmount(d("15.50"), d("83.6892")).Equal(d("1297.18")),
		"got %s", convertAmount(d("15.50"), d("83.6892")))
	// rate=1 (одна валюта) → без изменений.
	assert.True(t, convertAmount(d("1297.18"), d("1")).Equal(d("1297.18")))
	// Округление: 10 × 0.185 (KZT→RUB) = 1.85.
	assert.True(t, convertAmount(d("10"), d("0.185")).Equal(d("1.85")))
	// Округление вниз: 1 × 0.333 = 0.33.
	assert.True(t, convertAmount(d("1"), d("0.333")).Equal(d("0.33")))
	// Округление вверх (банковское? shopspring Round = half-up): 1 × 0.335 = 0.34.
	assert.True(t, convertAmount(d("1"), d("0.335")).Equal(d("0.34")),
		"half-up округление, got %s", convertAmount(d("1"), d("0.335")))
	// Ноль суммы → ноль.
	assert.True(t, convertAmount(d("0"), d("83.68")).IsZero())
}

// Fix двойного округления (Codex #2): raw daily × rate ≠ round(daily) × rate для
// мелких тарифов. 1 EUR/31 = 0.032258..., курс 100.
func TestNoDoubleRounding(t *testing.T) {
	day := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC) // январь, 31 день
	rate := d("100")
	// Старый (багованный) путь: round дневной суммы в валюте плана, потом ×rate.
	roundedFirst := convertAmount(dailyChargeAmount(d("1"), 1, day, false), rate) // 0.03 × 100 = 3.00
	// Новый путь: raw × rate, один round.
	rawPath := convertAmount(dailyChargeAmountRaw(d("1"), 1, day, false), rate) // 0.032258×100=3.2258→3.23
	assert.True(t, roundedFirst.Equal(d("3.00")), "double-round даёт 3.00, got %s", roundedFirst)
	assert.True(t, rawPath.Equal(d("3.23")), "raw-path даёт 3.23, got %s", rawPath)
	assert.False(t, roundedFirst.Equal(rawPath), "пути должны различаться — в этом и баг")
}

// getRateCached: конверсия через pivot (прямой/inverse/cross) + кэш-хит (П5 backlog).
func TestGetRateCached(t *testing.T) {
	s := &LedgerChargeScheduler{rateSvc: setupRateTestSvc(t)}
	mk := func(base, rate string) {
		require.NoError(t, s.rateSvc.db.Create(&models.CurrencyRate{
			RateDate: day(2026, 5, 29), BaseCcy: base, QuoteCcy: "RUB", Source: "cbr_rf", Rate: d(rate),
		}).Error)
	}
	mk("EUR", "100.00")
	mk("USD", "80.00")
	dt := day(2026, 5, 29)
	cache := map[string]cachedRate{}

	// Прямой план→RUB (договор в RUB).
	r, _, _, err := s.getRateCached(cache, dt, "EUR", "RUB", "cbr_rf")
	require.NoError(t, err)
	assert.True(t, r.Equal(d("100")), "EUR→RUB got %s", r)
	assert.Len(t, cache, 1)

	// inverse RUB→X (план в RUB, договор в EUR) — раньше стопил подписку, теперь ок.
	r, _, _, err = s.getRateCached(cache, dt, "RUB", "EUR", "cbr_rf")
	require.NoError(t, err)
	assert.True(t, r.Equal(d("0.01")), "RUB→EUR got %s", r)
	assert.Len(t, cache, 2)

	// cross X→Y (план EUR, договор USD) — раньше стопил, теперь 100/80=1.25.
	r, _, _, err = s.getRateCached(cache, dt, "EUR", "USD", "cbr_rf")
	require.NoError(t, err)
	assert.True(t, r.Equal(d("1.25")), "EUR→USD got %s", r)
	assert.Len(t, cache, 3)

	// Повторный cross — из кэша, новый ключ не добавляется.
	r2, _, _, err := s.getRateCached(cache, dt, "EUR", "USD", "cbr_rf")
	require.NoError(t, err)
	assert.True(t, r2.Equal(d("1.25")))
	assert.Len(t, cache, 3)

	// Нет курса в одном из звеньев cross → ошибка (стоп подписки).
	_, _, _, err = s.getRateCached(cache, dt, "GBP", "USD", "cbr_rf")
	assert.Error(t, err, "GBP→USD без курса GBP→RUB должен дать ошибку")
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

// П6: monthFloor + monthlyPlanAmount (yearly → /12).
func TestMonthlyHelpers(t *testing.T) {
	assert.Equal(t, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), monthFloor(time.Date(2026, 5, 15, 13, 0, 0, 0, time.UTC)))
	assert.True(t, monthlyPlanAmount(d("600"), "monthly").Equal(d("600")))
	assert.True(t, monthlyPlanAmount(d("1200"), "yearly").Equal(d("100")), "yearly 1200 → 100/мес")
	assert.True(t, monthlyPlanAmount(d("500"), "").Equal(d("500")))
}

// П6: бинарный подсчёт объектов с присутствием ≥ minDays в месяце.
func TestObjectsPresentInMonthGE(t *testing.T) {
	db := setupChargeTestDB(t)
	s := &LedgerChargeScheduler{}
	subID := uint(200)

	mk := func(start time.Time, end *time.Time, status string, deleted *time.Time) {
		co := models.ContractObject{
			ContractID: 1, SubscriptionID: &subID,
			ObjectID:        uint(time.Now().UnixNano() % 1000000),
			ObjectCompanyID: 186, ObjectSchema: "tenant_186",
			Status: status, StartDate: start, EndDate: end,
		}
		require.NoError(t, db.Create(&co).Error)
		if deleted != nil {
			require.NoError(t, db.Model(&models.ContractObject{}).Where("id = ?", co.ID).
				Update("deleted_at", deleted).Error)
		}
	}
	jan := func(dd int) time.Time { return time.Date(2026, 1, dd, 0, 0, 0, 0, time.UTC) }
	janStart := jan(1)

	endJ3 := jan(3)
	endJ6 := jan(6)
	mk(jan(1), nil, "active", nil)    // A: весь месяц (31 дн) ≥5 ✓
	mk(jan(1), &endJ3, "active", nil) // B: 1-3 (3 дн) <5 ✗
	mk(jan(1), &endJ6, "active", nil) // C: 1-6 (6 дн) ≥5 ✓
	mk(jan(28), nil, "active", nil)   // D: 28-31 (4 дн) <5 ✗
	mk(jan(1), nil, "inactive", nil)  // E: inactive ✗

	// window = январь, ограниченный днём наблюдения. obs далеко → полный месяц.
	janEnd := janStart.AddDate(0, 1, 0)
	obs := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	win := func(observed time.Time) time.Time { return capWindowEnd(janEnd, observed) }

	cnt, err := s.objectsPresentInWindowGE(db, subID, janStart, win(obs), 5)
	require.NoError(t, err)
	assert.Equal(t, 2, cnt, "при minDays=5 присутствуют A и C")

	// minDays=1 → A,B,C,D (E inactive) = 4.
	cnt1, err := s.objectsPresentInWindowGE(db, subID, janStart, win(obs), 1)
	require.NoError(t, err)
	assert.Equal(t, 4, cnt1)

	// minDays=31 → только A (весь месяц).
	cnt31, err := s.objectsPresentInWindowGE(db, subID, janStart, win(obs), 31)
	require.NoError(t, err)
	assert.Equal(t, 1, cnt31)

	// Codex (c): observedEnd ограничивает текущий месяц. Наблюдаем только до 4-го января
	// → A присутствует 3 дня (1,2,3) < 5 → 0. (защита от будущего присутствия prepaid).
	cntEarly, err := s.objectsPresentInWindowGE(db, subID, janStart, win(jan(4)), 5)
	require.NoError(t, err)
	assert.Equal(t, 0, cntEarly, "до 4-го января никто не накопил ≥5 дней")
}

// П6 Ф2: интеграционный тест monthly-каденции — prepaid/postpaid тайминг,
// идемпотентность, анти-двойное-списание при наличии дневных проводок.
func TestChargeSubscriptionMonthly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ContractObject{}, &models.LedgerEntry{}))
	s := &LedgerChargeScheduler{db: db}

	apr := func(dd int) time.Time { return time.Date(2026, 4, dd, 0, 0, 0, 0, time.UTC) }
	may := func(dd int) time.Time { return time.Date(2026, 5, dd, 0, 0, 0, 0, time.UTC) }

	mkSub := func(id uint, start time.Time) *models.Subscription {
		sub := &models.Subscription{AdminAccountID: 1, StartDate: start,
			BillingPlan: models.BillingPlan{Price: d("600"), BillingPeriod: "monthly", Currency: "RUB"}}
		sub.ID = id
		return sub
	}
	mkObj := func(subID uint, start time.Time, end *time.Time) {
		co := models.ContractObject{ContractID: 1, SubscriptionID: &subID, ObjectID: uint(time.Now().UnixNano() % 1000000),
			ObjectCompanyID: 186, ObjectSchema: "tenant_186", Status: "active", StartDate: start, EndDate: end}
		require.NoError(t, db.Create(&co).Error)
	}

	cutoff, target := apr(1), may(30)
	rc := map[string]cachedRate{}

	// --- Prepaid: апрель (полный) + май (прошло ≥5 дней) = 2 проводки, дата = 1-е тек. месяца ---
	subP := mkSub(300, apr(1))
	mkObj(300, apr(1), nil)
	e, _ := s.chargeSubscriptionMonthly(db, 186, subP, 88, 0, "RUB", "prepaid", "cbr_rf", 5, rc, cutoff, target)
	assert.Equal(t, 2, e, "prepaid: апрель + май")
	var pe []models.LedgerEntry
	db.Where("subscription_id = ?", 300).Order("entry_date").Find(&pe)
	require.Len(t, pe, 2)
	assert.Equal(t, "autocharge:sub:300:2026-04", pe[0].ExternalID)
	assert.Equal(t, apr(1), pe[0].EntryDate.UTC(), "prepaid: дата = 1-е апреля")
	assert.True(t, pe[0].Amount.Equal(d("-600")), "amount апрель")
	assert.Equal(t, "autocharge:sub:300:2026-05", pe[1].ExternalID)
	assert.Equal(t, may(1), pe[1].EntryDate.UTC())

	// Идемпотентность: повтор → 0 новых.
	e2, _ := s.chargeSubscriptionMonthly(db, 186, subP, 88, 0, "RUB", "prepaid", "cbr_rf", 5, rc, cutoff, target)
	assert.Equal(t, 0, e2)

	// --- Postpaid: апрель списывается 1-го МАЯ; май ещё не наступил (1 июня > target) = 1 проводка ---
	subPost := mkSub(301, apr(1))
	mkObj(301, apr(1), nil)
	ep, _ := s.chargeSubscriptionMonthly(db, 186, subPost, 89, 0, "RUB", "postpaid", "cbr_rf", 5, rc, cutoff, target)
	assert.Equal(t, 1, ep, "postpaid: только апрель")
	var poe []models.LedgerEntry
	db.Where("subscription_id = ?", 301).Find(&poe)
	require.Len(t, poe, 1)
	assert.Equal(t, "autocharge:sub:301:2026-04", poe[0].ExternalID)
	assert.Equal(t, may(1), poe[0].EntryDate.UTC(), "postpaid: дата = 1-е мая (за апрель)")

	// --- Анти-двойное-списание: за апрель уже есть ДНЕВНАЯ проводка → monthly пропускает апрель ---
	subSwitch := mkSub(302, apr(1))
	mkObj(302, apr(1), nil)
	require.NoError(t, db.Create(&models.LedgerEntry{AdminAccountID: 1, CompanyID: 186, ContractID: 90,
		SubscriptionID: func() *uint { v := uint(302); return &v }(), EntryType: "charge", Amount: d("-20"),
		Currency: "RUB", Source: "auto_charge", ExternalID: "autocharge:sub:302:2026-04-15", EntryDate: apr(15)}).Error)
	es, _ := s.chargeSubscriptionMonthly(db, 186, subSwitch, 90, 0, "RUB", "prepaid", "cbr_rf", 5, rc, cutoff, target)
	var sw []models.LedgerEntry
	db.Where("subscription_id = ? AND external_id LIKE ?", 302, "autocharge:sub:302:2026-04%").Find(&sw)
	// апрель: 1 дневная (старая), monthly НЕ добавил за апрель.
	assert.Len(t, sw, 1, "за апрель только дневная, monthly пропущен")
	assert.Equal(t, "autocharge:sub:302:2026-04-15", sw[0].ExternalID)
	// май monthly добавился (за май дневных нет).
	var swMay int64
	db.Model(&models.LedgerEntry{}).Where("external_id = ?", "autocharge:sub:302:2026-05").Count(&swMay)
	assert.EqualValues(t, 1, swMay, "май начислен monthly")
	_ = es
}

// П6 Ф3: yearly lump_sum — вся годовая сумма 1 проводкой + анти-дубль.
func TestChargeSubscriptionPeriodLump(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ContractObject{}, &models.LedgerEntry{}))
	s := &LedgerChargeScheduler{db: db}
	apr := func(dd int) time.Time { return time.Date(2026, 4, dd, 0, 0, 0, 0, time.UTC) }

	mkObj := func(subID uint, start time.Time) {
		co := models.ContractObject{ContractID: 1, SubscriptionID: &subID, ObjectID: uint(time.Now().UnixNano() % 1000000),
			ObjectCompanyID: 186, ObjectSchema: "tenant_186", Status: "active", StartDate: start}
		require.NoError(t, db.Create(&co).Error)
	}
	mkSub := func(id uint) *models.Subscription {
		sub := &models.Subscription{AdminAccountID: 1, StartDate: apr(1),
			BillingPlan: models.BillingPlan{Price: d("12000"), BillingPeriod: "yearly", Currency: "RUB"}}
		sub.ID = id
		return sub
	}
	cutoff, target := apr(1), time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	rc := map[string]cachedRate{}

	// Prepaid yearly lump: 1 проводка -12000, дата = 1-е апреля, extID :y:2026-04.
	sub := mkSub(400)
	mkObj(400, apr(1))
	e, _ := s.chargeSubscriptionPeriodLump(db, 186, sub, 88, 0, "RUB", "prepaid", "cbr_rf", 5, rc, cutoff, target)
	assert.Equal(t, 1, e, "один годовой charge")
	var le []models.LedgerEntry
	db.Where("subscription_id = ?", 400).Find(&le)
	require.Len(t, le, 1)
	assert.Equal(t, "autocharge:sub:400:y:2026-04", le[0].ExternalID)
	assert.Equal(t, apr(1), le[0].EntryDate.UTC())
	assert.True(t, le[0].Amount.Equal(d("-12000")), "вся годовая сумма")

	// Идемпотентность.
	e2, _ := s.chargeSubscriptionPeriodLump(db, 186, sub, 88, 0, "RUB", "prepaid", "cbr_rf", 5, rc, cutoff, target)
	assert.Equal(t, 0, e2)

	// Анти-дубль: за период уже есть дневная проводка → годовая пропускается.
	sub2 := mkSub(401)
	mkObj(401, apr(1))
	require.NoError(t, db.Create(&models.LedgerEntry{AdminAccountID: 1, CompanyID: 186, ContractID: 89,
		SubscriptionID: func() *uint { v := uint(401); return &v }(), EntryType: "charge", Amount: d("-30"),
		Currency: "RUB", Source: "auto_charge", ExternalID: "autocharge:sub:401:2026-04-10", EntryDate: apr(10)}).Error)
	e3, _ := s.chargeSubscriptionPeriodLump(db, 186, sub2, 89, 0, "RUB", "prepaid", "cbr_rf", 5, rc, cutoff, target)
	assert.Equal(t, 0, e3, "год покрыт дневной → пропуск")
	var le2 int64
	db.Model(&models.LedgerEntry{}).Where("external_id = ?", "autocharge:sub:401:y:2026-04").Count(&le2)
	assert.EqualValues(t, 0, le2)

	// Codex H4: late-start. Подписка с 27 апреля — в анкор-месяце всего 4 дня (<5), но
	// window-based считает присутствие за период → к маю набирается ≥5 → год списывается.
	subLate := &models.Subscription{AdminAccountID: 1, StartDate: apr(27),
		BillingPlan: models.BillingPlan{Price: d("12000"), BillingPeriod: "yearly", Currency: "RUB"}}
	subLate.ID = 402
	mkObj(402, apr(27))
	// cutoff=apr(1): период не режется e1-guard'ом; объект с 27-го через window-окно
	// набирает ≥5 дней за период (к маю) → год списывается (а не 0 из-за анкор-месяца).
	e4, _ := s.chargeSubscriptionPeriodLump(db, 186, subLate, 90, 0, "RUB", "prepaid", "cbr_rf", 5, rc, apr(1), target)
	assert.Equal(t, 1, e4, "late-start: год списан (присутствие за период ≥5 дней)")
}

// П6 Ф4: override каденции тарифа перебивает глобальную каденцию компании.
// routeCadence — pure-функция выбора charge-пути; покрываем матрицу override.
func TestRouteCadence_Override(t *testing.T) {
	cases := []struct {
		name                           string
		company, plan, period, longSub string
		want                           string
	}{
		// Ф4 главный кейс: глобал daily, тариф override monthly → monthly.
		{"override-up_daily→monthly", "daily", "monthly", "monthly", "monthly", "monthly"},
		// Без override: пустой план берёт глобал.
		{"no-override_monthly", "monthly", "", "monthly", "monthly", "monthly"},
		{"no-override_daily", "daily", "", "monthly", "monthly", "daily"},
		// Override вниз: глобал monthly, тариф override daily → daily.
		{"override-down_monthly→daily", "monthly", "daily", "monthly", "monthly", "daily"},
		// Override на period+yearly+lump_sum → lump.
		{"override_period-yearly-lump", "daily", "period", "yearly", "lump_sum", "lump"},
		// period+yearly но longSub=monthly → НЕ lump, помесячно.
		{"period-yearly_monthly-longsub", "daily", "period", "yearly", "monthly", "monthly"},
		// period на месячном тарифе → monthly (price/1), не lump.
		{"override_period-monthly-plan", "daily", "period", "monthly", "lump_sum", "monthly"},
		// Дефолт: оба пусты → daily.
		{"default_empty", "", "", "monthly", "monthly", "daily"},
		// Глобал period+lump, тариф override monthly перебивает lump → monthly.
		{"override_lump→monthly", "period", "monthly", "yearly", "lump_sum", "monthly"},
		// Пробелы в override игнорируются (TrimSpace) → глобал.
		{"whitespace-override_ignored", "monthly", "  ", "monthly", "monthly", "monthly"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := routeCadence(c.company, c.plan, c.period, c.longSub)
			assert.Equal(t, c.want, got)
		})
	}
}

// П6 Ф5 forward-only: при смене каденции monthly/lump→daily дни месяца, уже покрытого
// одной проводкой, НЕ списываются повторно посуточно (закрытие латентного двойного счёта).
func TestDailyChargeSkipsCoveredMonths(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ContractObject{}, &models.LedgerEntry{}))
	s := &LedgerChargeScheduler{db: db}

	jun := func(dd int) time.Time { return time.Date(2026, 6, dd, 0, 0, 0, 0, time.UTC) }
	jul := func(dd int) time.Time { return time.Date(2026, 7, dd, 0, 0, 0, 0, time.UTC) }
	rc := map[string]cachedRate{}
	pu := func(v uint) *uint { return &v }

	mkSub := func(id uint, start time.Time, period string) *models.Subscription {
		sub := &models.Subscription{AdminAccountID: 1, StartDate: start,
			BillingPlan: models.BillingPlan{Price: d("600"), BillingPeriod: period, Currency: "RUB"}}
		sub.ID = id
		return sub
	}
	mkObj := func(subID uint, start time.Time) {
		co := models.ContractObject{ContractID: 1, SubscriptionID: &subID, ObjectID: uint(time.Now().UnixNano() % 1000000),
			ObjectCompanyID: 186, ObjectSchema: "tenant_186", Status: "active", StartDate: start}
		require.NoError(t, db.Create(&co).Error)
	}

	// --- monthly→daily: июнь покрыт месячной проводкой → daily НЕ списывает июнь, постит июль ---
	subM := mkSub(500, jun(1), "monthly")
	mkObj(500, jun(1))
	require.NoError(t, db.Create(&models.LedgerEntry{AdminAccountID: 1, CompanyID: 186, ContractID: 95,
		SubscriptionID: pu(500), EntryType: "charge", Amount: d("-600"), Currency: "RUB", Source: "auto_charge",
		ExternalID: "autocharge:sub:500:2026-06", EntryDate: jun(1)}).Error)
	e, _ := s.chargeSubscription(db, 186, subM, 95, 0, "RUB", "cbr_rf", rc, jun(1), jul(31))
	var junDaily, julDaily int64
	db.Model(&models.LedgerEntry{}).Where("external_id LIKE ?", "autocharge:sub:500:2026-06-%").Count(&junDaily)
	db.Model(&models.LedgerEntry{}).Where("external_id LIKE ?", "autocharge:sub:500:2026-07-%").Count(&julDaily)
	assert.EqualValues(t, 0, junDaily, "июнь покрыт monthly → 0 дневных")
	assert.Greater(t, julDaily, int64(0), "июль не покрыт → daily постит")
	assert.Greater(t, e, 0)

	// --- lump→daily: годовой lump с июня покрывает 12 мес → daily не списывает июнь/июль ---
	subL := mkSub(501, jun(1), "yearly")
	mkObj(501, jun(1))
	require.NoError(t, db.Create(&models.LedgerEntry{AdminAccountID: 1, CompanyID: 186, ContractID: 96,
		SubscriptionID: pu(501), EntryType: "charge", Amount: d("-7200"), Currency: "RUB", Source: "auto_charge",
		ExternalID: "autocharge:sub:501:y:2026-06", EntryDate: jun(1)}).Error)
	el, _ := s.chargeSubscription(db, 186, subL, 96, 0, "RUB", "cbr_rf", rc, jun(1), jul(31))
	var lumpDaily int64
	db.Model(&models.LedgerEntry{}).Where("subscription_id = ? AND external_id LIKE ?", 501, "autocharge:sub:501:2026-%-%").Count(&lumpDaily)
	assert.EqualValues(t, 0, lumpDaily, "период lump покрыт → 0 дневных")
	assert.Equal(t, 0, el)

	// --- контроль: без покрытия daily постит как обычно ---
	subD := mkSub(502, jun(1), "monthly")
	mkObj(502, jun(1))
	ed, _ := s.chargeSubscription(db, 186, subD, 97, 0, "RUB", "cbr_rf", rc, jun(1), jun(10))
	assert.Equal(t, 10, ed, "без покрытия — 10 дневных проводок (1-10 июня)")
}

// П6 Ф5 #1 (Codex): postpaid monthly→daily НЕ теряет 1-е число след. месяца. Postpaid
// monthly за июнь имеет entry_date=01.07; общий MAX(entry_date) сместил бы daily-старт на
// 02.07 и потерял 01.07. Daily-checkpoint по дневным проводкам (их нет) → старт от cutoff.
func TestDailyChargePostpaidMonthlyNoLostDay(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ContractObject{}, &models.LedgerEntry{}))
	s := &LedgerChargeScheduler{db: db}

	jun := func(dd int) time.Time { return time.Date(2026, 6, dd, 0, 0, 0, 0, time.UTC) }
	jul := func(dd int) time.Time { return time.Date(2026, 7, dd, 0, 0, 0, 0, time.UTC) }
	pu := func(v uint) *uint { return &v }
	rc := map[string]cachedRate{}

	sub := &models.Subscription{AdminAccountID: 1, StartDate: jun(1),
		BillingPlan: models.BillingPlan{Price: d("600"), BillingPeriod: "monthly", Currency: "RUB"}}
	sub.ID = 510
	co := models.ContractObject{ContractID: 1, SubscriptionID: pu(510), ObjectID: 77,
		ObjectCompanyID: 186, ObjectSchema: "tenant_186", Status: "active", StartDate: jun(1)}
	require.NoError(t, db.Create(&co).Error)
	// Postpaid monthly за июнь: external_id :2026-06, entry_date = 01.07.
	require.NoError(t, db.Create(&models.LedgerEntry{AdminAccountID: 1, CompanyID: 186, ContractID: 98,
		SubscriptionID: pu(510), EntryType: "charge", Amount: d("-600"), Currency: "RUB", Source: "auto_charge",
		ExternalID: "autocharge:sub:510:2026-06", EntryDate: jul(1)}).Error)

	s.chargeSubscription(db, 186, sub, 98, 0, "RUB", "cbr_rf", rc, jun(1), jul(5))
	// Июнь покрыт monthly → 0 дневных. Июль 1-5 непокрыт → 5 дневных, ВКЛЮЧАЯ 01.07.
	var junDaily int64
	db.Model(&models.LedgerEntry{}).Where("external_id LIKE ?", "autocharge:sub:510:2026-06-%").Count(&junDaily)
	assert.EqualValues(t, 0, junDaily, "июнь покрыт monthly")
	var jul1 int64
	db.Model(&models.LedgerEntry{}).Where("external_id = ?", "autocharge:sub:510:2026-07-01").Count(&jul1)
	assert.EqualValues(t, 1, jul1, "01.07 НЕ потерян (Codex #1)")
	var julDaily int64
	db.Model(&models.LedgerEntry{}).Where("external_id LIKE ?", "autocharge:sub:510:2026-07-%").Count(&julDaily)
	assert.EqualValues(t, 5, julDaily, "июль 1-5 списан посуточно")
}
