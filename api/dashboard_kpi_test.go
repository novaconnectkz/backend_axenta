package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend_axenta/models"
)

// =====================================================================
// Setup
// =====================================================================

func setupKPITestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Object{},
		&models.Invoice{},
		&models.Installation{},
		&models.StockAlert{},
	))
	return db
}

func newKPIRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/dashboard/kpi", func(c *gin.Context) {
		c.Set("tenant_db", db)
		GetDashboardKPI(c)
	})
	return r
}

func callKPI(t *testing.T, db *gorm.DB) (int, KPIResponse) {
	t.Helper()
	r := newKPIRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/kpi", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Status string      `json:"status"`
		Data   KPIResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp.Data
}

func findMetric(metrics []KPIMetric, id string) *KPIMetric {
	for i := range metrics {
		if metrics[i].ID == id {
			return &metrics[i]
		}
	}
	return nil
}

// =====================================================================
// Контракт ответа
// =====================================================================

func TestGetDashboardKPI_NoTenantDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/dashboard/kpi", GetDashboardKPI)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/kpi", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetDashboardKPI_EmptyDB_Returns4Metrics(t *testing.T) {
	db := setupKPITestDB(t)
	code, data := callKPI(t, db)

	assert.Equal(t, http.StatusOK, code)
	require.Len(t, data.Metrics, 4)

	ids := []string{}
	for _, m := range data.Metrics {
		ids = append(ids, m.ID)
	}
	assert.ElementsMatch(t, []string{"active_objects", "monthly_revenue", "today_installations", "alert"}, ids)

	// Все нули, все flat (или alert = "OK")
	objects := findMetric(data.Metrics, "active_objects")
	require.NotNil(t, objects)
	assert.Equal(t, "0", objects.Value)
	assert.Equal(t, "flat", objects.DeltaDirection)

	alert := findMetric(data.Metrics, "alert")
	require.NotNil(t, alert)
	assert.Equal(t, "OK", alert.Value, "пустая БД → alert метрика = OK")
}

// =====================================================================
// Метрика 1: active_objects + delta vs неделю назад
// =====================================================================

func TestGetDashboardKPI_ActiveObjects_PositiveDelta(t *testing.T) {
	db := setupKPITestDB(t)
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)

	// 5 активных, существуют > недели — попадут в prev
	for i := 0; i < 5; i++ {
		require.NoError(t, db.Exec(
			"INSERT INTO objects (name, type, status, company_id, location_id, created_at, updated_at) VALUES (?, ?, ?, 1, 1, ?, ?)",
			fmt.Sprintf("old-%d", i), "vehicle", "active", weekAgo.Add(-24*time.Hour), now,
		).Error)
	}
	// 3 активных, добавлены вчера — current=8, prev=5, delta=+3
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Exec(
			"INSERT INTO objects (name, type, status, company_id, location_id, created_at, updated_at) VALUES (?, ?, ?, 1, 1, ?, ?)",
			fmt.Sprintf("new-%d", i), "vehicle", "active", now.Add(-24*time.Hour), now,
		).Error)
	}
	// 2 inactive — не должны попасть
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Exec(
			"INSERT INTO objects (name, type, status, company_id, location_id, created_at, updated_at) VALUES (?, ?, ?, 1, 1, ?, ?)",
			fmt.Sprintf("inact-%d", i), "vehicle", "inactive", weekAgo.Add(-24*time.Hour), now,
		).Error)
	}

	_, data := callKPI(t, db)
	m := findMetric(data.Metrics, "active_objects")
	require.NotNil(t, m)
	assert.Equal(t, "8", m.Value)
	assert.Equal(t, "up", m.DeltaDirection)
	assert.Equal(t, float64(3), m.DeltaValue)
	assert.Contains(t, m.Delta, "+3")
	assert.Contains(t, m.Delta, "за неделю")
}

func TestGetDashboardKPI_ActiveObjects_FlatDelta(t *testing.T) {
	db := setupKPITestDB(t)
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -8)

	// 4 активных, все старше недели — current=4, prev=4, delta=0
	for i := 0; i < 4; i++ {
		require.NoError(t, db.Exec(
			"INSERT INTO objects (name, type, status, company_id, location_id, created_at, updated_at) VALUES (?, ?, ?, 1, 1, ?, ?)",
			fmt.Sprintf("o-%d", i), "vehicle", "active", weekAgo, now,
		).Error)
	}

	_, data := callKPI(t, db)
	m := findMetric(data.Metrics, "active_objects")
	require.NotNil(t, m)
	assert.Equal(t, "4", m.Value)
	assert.Equal(t, "flat", m.DeltaDirection)
	assert.Contains(t, m.Delta, "без изменений")
}

// =====================================================================
// Метрика 2: monthly_revenue + delta vs прошлый месяц
// =====================================================================

func TestGetDashboardKPI_MonthlyRevenue(t *testing.T) {
	db := setupKPITestDB(t)
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, now.Location())
	prevMonthMid := monthStart.AddDate(0, 0, -15)

	mkInvoice := func(num string, paidAt time.Time, paid float64) {
		require.NoError(t, db.Create(&models.Invoice{
			Number: num, DueDate: paidAt, Status: "paid",
			SubtotalAmount: decimal.NewFromFloat(paid),
			TotalAmount:    decimal.NewFromFloat(paid),
			PaidAmount:     decimal.NewFromFloat(paid),
			PaidAt:         &paidAt,
		}).Error)
	}

	// Текущий месяц: 10000 + 5500 = 15500
	mkInvoice("CUR-1", monthStart, 10000)
	mkInvoice("CUR-2", monthStart.Add(48*time.Hour), 5500)
	// Прошлый месяц: 8000
	mkInvoice("PREV-1", prevMonthMid, 8000)

	_, data := callKPI(t, db)
	m := findMetric(data.Metrics, "monthly_revenue")
	require.NotNil(t, m)
	assert.Equal(t, "15500 ₽", m.Value)
	assert.Equal(t, "up", m.DeltaDirection)
	assert.InDelta(t, 7500, m.DeltaValue, 0.01, "delta = 15500 - 8000 = 7500")
	// Percentage = 7500/8000 * 100 = 93.75
	assert.InDelta(t, 93.75, m.DeltaPercentage, 0.5)
	assert.Contains(t, m.Delta, "vs прошлый месяц")
}

// =====================================================================
// Метрика 3: today_installations + delta vs неделю назад
// =====================================================================

func TestGetDashboardKPI_TodayInstallations(t *testing.T) {
	db := setupKPITestDB(t)
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
	sameDayLastWeek := dayStart.AddDate(0, 0, -7)
	twoDaysAgo := dayStart.AddDate(0, 0, -2)

	mk := func(at time.Time) {
		require.NoError(t, db.Create(&models.Installation{
			Type: "монтаж", Status: "planned", ScheduledAt: at,
			ObjectID: 1, InstallerID: 1,
		}).Error)
	}
	// сегодня: 3
	mk(dayStart)
	mk(dayStart.Add(2 * time.Hour))
	mk(dayStart.Add(5 * time.Hour))
	// неделю назад в тот же день: 2
	mk(sameDayLastWeek)
	mk(sameDayLastWeek.Add(3 * time.Hour))
	// 2 дня назад: не должно попасть
	mk(twoDaysAgo)

	_, data := callKPI(t, db)
	m := findMetric(data.Metrics, "today_installations")
	require.NotNil(t, m)
	assert.Equal(t, "3", m.Value)
	assert.Equal(t, "up", m.DeltaDirection)
	assert.Equal(t, float64(1), m.DeltaValue)
}

// =====================================================================
// Метрика 4: alert (самый severe)
// =====================================================================

func TestGetDashboardKPI_Alert_PicksHighestSeverity(t *testing.T) {
	db := setupKPITestDB(t)
	now := time.Now()

	// medium: 1 просроченный монтаж
	require.NoError(t, db.Create(&models.Installation{
		Type: "монтаж", Status: "planned", ScheduledAt: now.Add(-1 * time.Hour),
		ObjectID: 1, InstallerID: 1,
	}).Error)
	// critical: 12 просроченных счетов
	for i := 0; i < 12; i++ {
		require.NoError(t, db.Create(&models.Invoice{
			Number:  fmt.Sprintf("OVR-%d", i),
			DueDate: now.Add(-1 * time.Hour), Status: "sent",
			SubtotalAmount: decimal.NewFromInt(100), TotalAmount: decimal.NewFromInt(100),
		}).Error)
	}

	_, data := callKPI(t, db)
	m := findMetric(data.Metrics, "alert")
	require.NotNil(t, m)
	assert.Equal(t, "Просроченные счета", m.Title, "critical (счета) > medium (монтажи)")
	assert.Equal(t, "12", m.Value)
	assert.Equal(t, "/billing/overdue", m.ActionURL)
	assert.Contains(t, m.Delta, "critical")
}

func TestGetDashboardKPI_Alert_OK_WhenNoAlerts(t *testing.T) {
	db := setupKPITestDB(t)
	_, data := callKPI(t, db)
	m := findMetric(data.Metrics, "alert")
	require.NotNil(t, m)
	assert.Equal(t, "OK", m.Value)
	assert.Equal(t, "flat", m.DeltaDirection)
}

// =====================================================================
// Форматтеры
// =====================================================================

func TestFormatCountDelta(t *testing.T) {
	dir, txt := formatCountDelta(5, "за неделю")
	assert.Equal(t, "up", dir)
	assert.Contains(t, txt, "+5")

	dir, txt = formatCountDelta(-3, "за неделю")
	assert.Equal(t, "down", dir)
	assert.Contains(t, txt, "-3")

	dir, txt = formatCountDelta(0, "за неделю")
	assert.Equal(t, "flat", dir)
	assert.Contains(t, txt, "без изменений")
}

func TestFormatRublesDelta_PercentageHandlesZeroPrev(t *testing.T) {
	current := decimal.NewFromInt(1000)
	delta := decimal.NewFromInt(1000)
	prev := decimal.NewFromInt(0)

	dir, txt, pct := formatRublesDelta(current, delta, prev, "vs прошлый месяц")
	assert.Equal(t, "up", dir)
	assert.Equal(t, float64(0), pct, "prev = 0 → pct = 0 без деления на ноль")
	assert.Contains(t, txt, "1000")
}
