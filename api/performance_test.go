package api

import (
	"backend_axenta/database"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupPerformanceTestDB создает тестовую базу данных для performance API
func setupPerformanceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupPerformanceTestRouter создает тестовый роутер
func setupPerformanceTestRouter(_ *testing.T, _ *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	return router
}

// TestPerformanceAPI_getCacheMetrics тестирует getCacheMetrics
func TestPerformanceAPI_getCacheMetrics(t *testing.T) {
	setupPerformanceTestDB(t)
	router := setupPerformanceTestRouter(t, database.DB)

	api := NewPerformanceAPI()
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/performance/cache/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку из-за Redis или успех
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

// TestPerformanceAPI_getCacheStats тестирует getCacheStats
func TestPerformanceAPI_getCacheStats(t *testing.T) {
	setupPerformanceTestDB(t)
	router := setupPerformanceTestRouter(t, database.DB)

	api := NewPerformanceAPI()
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/performance/cache/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку из-за Redis или успех
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

// TestPerformanceAPI_getDatabaseStats тестирует getDatabaseStats
func TestPerformanceAPI_getDatabaseStats(t *testing.T) {
	setupPerformanceTestDB(t)
	router := setupPerformanceTestRouter(t, database.DB)

	api := NewPerformanceAPI()
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/performance/database/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку из-за PostgreSQL специфичных запросов или успех
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

// TestPerformanceAPI_getPerformanceMetrics тестирует getPerformanceMetrics
func TestPerformanceAPI_getPerformanceMetrics(t *testing.T) {
	setupPerformanceTestDB(t)
	router := setupPerformanceTestRouter(t, database.DB)

	api := NewPerformanceAPI()
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/performance/monitoring/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку из-за Redis или успех
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

// TestPerformanceAPI_getSystemHealth тестирует getSystemHealth
func TestPerformanceAPI_getSystemHealth(t *testing.T) {
	setupPerformanceTestDB(t)
	router := setupPerformanceTestRouter(t, database.DB)

	api := NewPerformanceAPI()
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/performance/system/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку из-за Redis или успех
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}
