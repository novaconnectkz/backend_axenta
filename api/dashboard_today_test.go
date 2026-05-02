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

func setupTodayTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Installation{},
		&models.Invoice{},
		&models.Contract{},
		&models.Installer{}, // нужен для Preload в today-installations
		&models.Object{},    // нужен для Preload
	))
	return db
}

func newTodayRouter(db *gorm.DB, handler gin.HandlerFunc, path string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET(path, func(c *gin.Context) {
		c.Set("tenant_db", db)
		handler(c)
	})
	return r
}

// =====================================================================
// today-installations
// =====================================================================

func TestGetTodayInstallations_EmptyDB(t *testing.T) {
	db := setupTodayTestDB(t)
	r := newTodayRouter(db, GetTodayInstallations, "/today")
	req := httptest.NewRequest(http.MethodGet, "/today", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []TodayInstallationItem `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Data, 0)
}

func TestGetTodayInstallations_FiltersTodayWindowAndSorts(t *testing.T) {
	db := setupTodayTestDB(t)
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	mk := func(at time.Time, status string) {
		require.NoError(t, db.Create(&models.Installation{
			Type: "монтаж", Status: status, ScheduledAt: at,
			ObjectID: 1, InstallerID: 1, Address: "addr-" + at.Format("15:04"),
		}).Error)
	}
	// сегодня — 3 разных времени, не по порядку
	mk(dayStart.Add(13*time.Hour), "planned")
	mk(dayStart.Add(9*time.Hour), "in_progress")
	mk(dayStart.Add(17*time.Hour+30*time.Minute), "planned")
	// вчера — не должен попасть
	mk(dayStart.Add(-12*time.Hour), "planned")
	// завтра — не должен попасть
	mk(dayStart.Add(36*time.Hour), "planned")

	r := newTodayRouter(db, GetTodayInstallations, "/today")
	req := httptest.NewRequest(http.MethodGet, "/today", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Data []TodayInstallationItem `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 3)

	// Проверяем порядок по времени ASC
	assert.Equal(t, "09:00", resp.Data[0].TimeLabel)
	assert.Equal(t, "13:00", resp.Data[1].TimeLabel)
	assert.Equal(t, "17:30", resp.Data[2].TimeLabel)
}

func TestGetTodayInstallations_NoTenantDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/today", GetTodayInstallations)
	req := httptest.NewRequest(http.MethodGet, "/today", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// =====================================================================
// recent-invoices
// =====================================================================

func TestGetRecentInvoices_OrdersDescAndLimitsTo10(t *testing.T) {
	db := setupTodayTestDB(t)
	base := time.Now().Add(-15 * 24 * time.Hour)

	// Создаём 12 счетов с разным created_at
	for i := 0; i < 12; i++ {
		inv := models.Invoice{
			Number:         fmt.Sprintf("INV-%03d", i),
			DueDate:        base.Add(time.Duration(i) * 24 * time.Hour),
			Status:         "sent",
			SubtotalAmount: decimal.NewFromInt(100),
			TotalAmount:    decimal.NewFromInt(int64(100 + i)),
		}
		require.NoError(t, db.Create(&inv).Error)
		// Устанавливаем созданную дату в порядке роста — последние 10 будут с большим created_at
		require.NoError(t, db.Model(&inv).
			UpdateColumn("created_at", base.Add(time.Duration(i)*time.Hour)).Error)
	}

	r := newTodayRouter(db, GetRecentInvoices, "/recent")
	req := httptest.NewRequest(http.MethodGet, "/recent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Data []RecentInvoiceItem `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 10, "limit=10")

	// Должны быть 11..2 (топ-10 по created_at DESC)
	assert.Equal(t, "INV-011", resp.Data[0].Number)
	assert.Equal(t, "INV-002", resp.Data[9].Number)
}

func TestGetRecentInvoices_OverdueFlag(t *testing.T) {
	db := setupTodayTestDB(t)
	now := time.Now()

	require.NoError(t, db.Create(&models.Invoice{
		Number:         "OVR-1",
		DueDate:        now.Add(-48 * time.Hour),
		Status:         "sent",
		SubtotalAmount: decimal.NewFromInt(100),
		TotalAmount:    decimal.NewFromInt(100),
	}).Error)
	require.NoError(t, db.Create(&models.Invoice{
		Number:         "OK-1",
		DueDate:        now.Add(48 * time.Hour),
		Status:         "sent",
		SubtotalAmount: decimal.NewFromInt(100),
		TotalAmount:    decimal.NewFromInt(100),
	}).Error)
	require.NoError(t, db.Create(&models.Invoice{
		Number:         "PAID-1",
		DueDate:        now.Add(-48 * time.Hour),
		Status:         "paid",
		SubtotalAmount: decimal.NewFromInt(100),
		TotalAmount:    decimal.NewFromInt(100),
	}).Error)

	r := newTodayRouter(db, GetRecentInvoices, "/recent")
	req := httptest.NewRequest(http.MethodGet, "/recent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Data []RecentInvoiceItem `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	flags := map[string]bool{}
	for _, it := range resp.Data {
		flags[it.Number] = it.IsOverdue
	}
	assert.True(t, flags["OVR-1"], "просроченный sent → IsOverdue")
	assert.False(t, flags["OK-1"], "будущая дата → не просрочен")
	assert.False(t, flags["PAID-1"], "оплаченный → не просрочен")
}

// =====================================================================
// installerDisplayName helper
// =====================================================================

func TestInstallerDisplayName(t *testing.T) {
	cases := []struct {
		first, last, want string
	}{
		{"Иван", "Петров", "Петров И."},
		{"Анна", "Сидорова", "Сидорова А."},
		{"", "Кузнецов", "Кузнецов"},
		{"Иван", "", "И."},
	}
	for _, tc := range cases {
		got := installerDisplayName(&models.Installer{FirstName: tc.first, LastName: tc.last})
		assert.Equal(t, tc.want, got)
	}
}
