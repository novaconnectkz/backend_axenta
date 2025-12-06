package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupDashboardTestDB создает тестовую базу данных для dashboard
func setupDashboardTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Object{},
		&models.User{},
		&models.Contract{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupDashboardTestRouter создает тестовый роутер с middleware
func setupDashboardTestRouter(_ *testing.T, db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки tenant_db
	router.Use(func(c *gin.Context) {
		c.Set("tenant_db", db)
		c.Next()
	})

	return router
}

// TestGetDashboardStats_NoTenantDB тестирует GetDashboardStats без tenant_db
func TestGetDashboardStats_NoTenantDB(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/api/dashboard/stats", GetDashboardStats)

	req, _ := http.NewRequest("GET", "/api/dashboard/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestGetDashboardStats_Success тестирует успешное получение статистики
func TestGetDashboardStats_Success(t *testing.T) {
	db := setupDashboardTestDB(t)
	router := setupDashboardTestRouter(t, db)

	router.GET("/api/dashboard/stats", GetDashboardStats)

	req, _ := http.NewRequest("GET", "/api/dashboard/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestGetDashboardActivity_NoTenantDB тестирует GetDashboardActivity без tenant_db
func TestGetDashboardActivity_NoTenantDB(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/api/dashboard/activity", GetDashboardActivity)

	req, _ := http.NewRequest("GET", "/api/dashboard/activity", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestGetDashboardActivity_Success тестирует успешное получение активности
func TestGetDashboardActivity_Success(t *testing.T) {
	db := setupDashboardTestDB(t)
	router := setupDashboardTestRouter(t, db)

	router.GET("/api/dashboard/activity", GetDashboardActivity)

	req, _ := http.NewRequest("GET", "/api/dashboard/activity", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestGetDashboardNotifications_NoTenantDB тестирует GetDashboardNotifications без tenant_db
func TestGetDashboardNotifications_NoTenantDB(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/api/dashboard/notifications", GetDashboardNotifications)

	req, _ := http.NewRequest("GET", "/api/dashboard/notifications", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestGetDashboardNotifications_Success тестирует успешное получение уведомлений
func TestGetDashboardNotifications_Success(t *testing.T) {
	db := setupDashboardTestDB(t)
	router := setupDashboardTestRouter(t, db)

	router.GET("/api/dashboard/notifications", GetDashboardNotifications)

	req, _ := http.NewRequest("GET", "/api/dashboard/notifications", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetDashboardStatsSimple тестирует GetDashboardStatsSimple
func TestGetDashboardStatsSimple(t *testing.T) {
	db := setupDashboardTestDB(t)
	router := setupDashboardTestRouter(t, db)

	router.GET("/api/dashboard/stats/simple", GetDashboardStatsSimple)

	req, _ := http.NewRequest("GET", "/api/dashboard/stats/simple", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetDashboardActivitySimple тестирует GetDashboardActivitySimple
func TestGetDashboardActivitySimple(t *testing.T) {
	db := setupDashboardTestDB(t)
	router := setupDashboardTestRouter(t, db)

	router.GET("/api/dashboard/activity/simple", GetDashboardActivitySimple)

	req, _ := http.NewRequest("GET", "/api/dashboard/activity/simple", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetDashboardNotificationsSimple тестирует GetDashboardNotificationsSimple
func TestGetDashboardNotificationsSimple(t *testing.T) {
	db := setupDashboardTestDB(t)
	router := setupDashboardTestRouter(t, db)

	router.GET("/api/dashboard/notifications/simple", GetDashboardNotificationsSimple)

	req, _ := http.NewRequest("GET", "/api/dashboard/notifications/simple", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
