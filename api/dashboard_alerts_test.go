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

func setupAlertsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Invoice{},
		&models.StockAlert{},
		&models.Installation{},
		&models.NotificationLog{},
	))
	return db
}

func newAlertsRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/dashboard/alerts", func(c *gin.Context) {
		c.Set("tenant_db", db)
		GetDashboardAlerts(c)
	})
	return r
}

func callAlerts(t *testing.T, db *gorm.DB) (int, []DashboardAlert) {
	t.Helper()
	r := newAlertsRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/alerts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Status string           `json:"status"`
		Data   []DashboardAlert `json:"data"`
		Error  string           `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp.Data
}

// =====================================================================
// pluralize — формы русских слов
// =====================================================================

func TestPluralize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{1, "1 счёт"},
		{2, "2 счёта"},
		{4, "4 счёта"},
		{5, "5 счетов"},
		{11, "11 счетов"}, // 11-19 — особый случай
		{14, "14 счетов"},
		{19, "19 счетов"},
		{21, "21 счёт"},
		{22, "22 счёта"},
		{25, "25 счетов"},
		{101, "101 счёт"},
		{111, "111 счетов"},
		{0, "0 счетов"},
	}
	for _, tc := range cases {
		got := pluralize(tc.n, "счёт", "счёта", "счетов")
		assert.Equal(t, tc.want, got, "n=%d", tc.n)
	}
}

// =====================================================================
// severityRank — порядок сортировки
// =====================================================================

func TestSeverityRank_OrderingDescending(t *testing.T) {
	assert.True(t, severityRank("critical") > severityRank("high"))
	assert.True(t, severityRank("high") > severityRank("medium"))
	assert.True(t, severityRank("medium") > severityRank("low"))
	assert.Equal(t, 0, severityRank("unknown"))
}

// =====================================================================
// GetDashboardAlerts — пустая БД и без tenant_db
// =====================================================================

func TestGetDashboardAlerts_NoTenantDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/dashboard/alerts", GetDashboardAlerts) // без c.Set("tenant_db", ...)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/alerts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Ошибка подключения")
}

func TestGetDashboardAlerts_EmptyDB(t *testing.T) {
	db := setupAlertsTestDB(t)
	code, alerts := callAlerts(t, db)

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 0, len(alerts), "пустая БД → нет алертов")
}

// =====================================================================
// Источник: просроченные счета
// =====================================================================

func TestGetDashboardAlerts_OverdueInvoices(t *testing.T) {
	db := setupAlertsTestDB(t)
	past := time.Now().Add(-48 * time.Hour)

	// 3 просроченных счёта на сумму 12500 + 5000 + 1000 = 18500 (paid=0)
	for i, total := range []float64{12500, 5000, 1000} {
		require.NoError(t, db.Create(&models.Invoice{
			Number:         fmt.Sprintf("INV-%d", i+1),
			DueDate:        past,
			Status:         "sent",
			SubtotalAmount: decimal.NewFromFloat(total),
			TotalAmount:    decimal.NewFromFloat(total),
		}).Error)
	}
	// + 1 оплаченный (не должен попасть)
	require.NoError(t, db.Create(&models.Invoice{
		Number:         "INV-paid",
		DueDate:        past,
		Status:         "paid",
		SubtotalAmount: decimal.NewFromFloat(999),
		TotalAmount:    decimal.NewFromFloat(999),
	}).Error)

	code, alerts := callAlerts(t, db)
	assert.Equal(t, http.StatusOK, code)
	require.Len(t, alerts, 1)
	a := alerts[0]
	assert.Equal(t, "billing.overdue", a.ID)
	assert.Equal(t, "billing", a.Category)
	assert.Equal(t, "high", a.Severity, "3 счёта < 10 → high")
	assert.Equal(t, 3, a.Count)
	assert.Contains(t, a.Description, "18500", "сумма ровно 18500 ₽")
	assert.Equal(t, "/billing/overdue", a.ActionURL)
}

func TestGetDashboardAlerts_OverdueInvoices_CriticalAt10Plus(t *testing.T) {
	db := setupAlertsTestDB(t)
	past := time.Now().Add(-1 * time.Hour)

	for i := 0; i < 12; i++ {
		require.NoError(t, db.Create(&models.Invoice{
			Number:         fmt.Sprintf("CRIT-%d", i),
			DueDate:        past,
			Status:         "sent",
			SubtotalAmount: decimal.NewFromFloat(100),
			TotalAmount:    decimal.NewFromFloat(100),
		}).Error)
	}

	_, alerts := callAlerts(t, db)
	require.Len(t, alerts, 1)
	assert.Equal(t, "critical", alerts[0].Severity, "≥10 просроченных → critical")
	assert.Equal(t, 12, alerts[0].Count)
}

// =====================================================================
// Источник: low-stock
// =====================================================================

func TestGetDashboardAlerts_LowStock(t *testing.T) {
	db := setupAlertsTestDB(t)
	require.NoError(t, db.Create(&models.StockAlert{
		Type: "low_stock", Title: "Мало X", Status: "active", Severity: "high",
	}).Error)
	require.NoError(t, db.Create(&models.StockAlert{
		Type: "low_stock", Title: "Мало Y", Status: "active", Severity: "medium",
	}).Error)
	// resolved — не должен попасть
	require.NoError(t, db.Create(&models.StockAlert{
		Type: "low_stock", Title: "Resolved", Status: "resolved", Severity: "high",
	}).Error)

	_, alerts := callAlerts(t, db)
	require.Len(t, alerts, 1)
	assert.Equal(t, "warehouse.low_stock", alerts[0].ID)
	assert.Equal(t, "high", alerts[0].Severity, "есть severity=high → high")
	assert.Equal(t, 2, alerts[0].Count)
}

// =====================================================================
// Источник: просроченные монтажи
// =====================================================================

func TestGetDashboardAlerts_OverdueInstallations(t *testing.T) {
	db := setupAlertsTestDB(t)
	past := time.Now().Add(-2 * time.Hour)
	future := time.Now().Add(2 * time.Hour)

	// 2 просроченных активных
	for _, st := range []string{"planned", "in_progress"} {
		require.NoError(t, db.Create(&models.Installation{
			Type: "монтаж", Status: st, ScheduledAt: past,
			ObjectID: 1, InstallerID: 1,
		}).Error)
	}
	// completed — не считается
	require.NoError(t, db.Create(&models.Installation{
		Type: "монтаж", Status: "completed", ScheduledAt: past,
		ObjectID: 1, InstallerID: 1,
	}).Error)
	// future planned — не просрочен
	require.NoError(t, db.Create(&models.Installation{
		Type: "монтаж", Status: "planned", ScheduledAt: future,
		ObjectID: 1, InstallerID: 1,
	}).Error)

	_, alerts := callAlerts(t, db)
	require.Len(t, alerts, 1)
	assert.Equal(t, "installations.overdue", alerts[0].ID)
	assert.Equal(t, 2, alerts[0].Count)
	assert.Equal(t, "medium", alerts[0].Severity, "<5 → medium")
}

// =====================================================================
// Источник: failed уведомления (24ч)
// =====================================================================

func TestGetDashboardAlerts_FailedNotifications_24hWindow(t *testing.T) {
	db := setupAlertsTestDB(t)

	// Создаём через прямой SQL чтобы контролировать created_at (GORM перезатрёт).
	now := time.Now()
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Exec(
			"INSERT INTO notification_logs (type, channel, recipient, message, status, attempt_count, company_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			"billing_alert", "email", "x@y.com", "msg", "failed", 1, 0, now.Add(-1*time.Hour), now,
		).Error)
	}
	// Старая (>24ч) — не должна попасть
	require.NoError(t, db.Exec(
		"INSERT INTO notification_logs (type, channel, recipient, message, status, attempt_count, company_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"billing_alert", "email", "x@y.com", "msg", "failed_final", 3, 0, now.Add(-48*time.Hour), now,
	).Error)
	// sent — не считается
	require.NoError(t, db.Exec(
		"INSERT INTO notification_logs (type, channel, recipient, message, status, attempt_count, company_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"billing_alert", "email", "x@y.com", "msg", "sent", 1, 0, now.Add(-1*time.Hour), now,
	).Error)

	_, alerts := callAlerts(t, db)
	require.Len(t, alerts, 1)
	assert.Equal(t, "notifications.failed_24h", alerts[0].ID)
	assert.Equal(t, 3, alerts[0].Count)
	assert.Equal(t, "low", alerts[0].Severity, "<10 → low")
}

// =====================================================================
// Сортировка по severity
// =====================================================================

func TestGetDashboardAlerts_SortedBySeverityDescending(t *testing.T) {
	db := setupAlertsTestDB(t)
	now := time.Now()

	// low: failed notif
	require.NoError(t, db.Exec(
		"INSERT INTO notification_logs (type, channel, recipient, message, status, attempt_count, company_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"x", "email", "r", "m", "failed", 1, 0, now.Add(-1*time.Hour), now,
	).Error)

	// medium: 1 просроченный монтаж
	require.NoError(t, db.Create(&models.Installation{
		Type: "монтаж", Status: "planned", ScheduledAt: now.Add(-1 * time.Hour),
		ObjectID: 1, InstallerID: 1,
	}).Error)

	// critical: 12 просроченных счетов
	for i := 0; i < 12; i++ {
		require.NoError(t, db.Create(&models.Invoice{
			Number:  fmt.Sprintf("SORT-%d", i),
			DueDate: now.Add(-1 * time.Hour), Status: "sent",
			SubtotalAmount: decimal.NewFromInt(100), TotalAmount: decimal.NewFromInt(100),
		}).Error)
	}

	_, alerts := callAlerts(t, db)
	require.Len(t, alerts, 3)
	assert.Equal(t, "critical", alerts[0].Severity)
	assert.Equal(t, "medium", alerts[1].Severity)
	assert.Equal(t, "low", alerts[2].Severity)
}
