package api

import (
	"backend_axenta/database"
	"backend_axenta/services"
	"bytes"
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

// setupAxentaSyncSettingsTestDB создает тестовую базу данных
func setupAxentaSyncSettingsTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	database.DB = db
	return db
}

// setupAxentaSyncSettingsTestRouter создает тестовый роутер с middleware
func setupAxentaSyncSettingsTestRouter(_ *testing.T, adminAccountID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки admin_account_id
	router.Use(func(c *gin.Context) {
		c.Set("user", map[string]interface{}{
			"id":        1,
			"username":  "testuser",
			"accountId": float64(adminAccountID),
		})
		c.Set("admin_account_id", adminAccountID)
		c.Next()
	})

	return router
}

// TestGetAxentaSyncSettings_NoScheduler тестирует GetAxentaSyncSettings когда планировщик не инициализирован
func TestGetAxentaSyncSettings_NoScheduler(t *testing.T) {
	setupAxentaSyncSettingsTestDB(t)
	router := setupAxentaSyncSettingsTestRouter(t, 123)

	// Убеждаемся, что планировщик не установлен
	services.SetAxentaSyncScheduler(nil)

	router.GET("/api/axenta-sync/settings", GetAxentaSyncSettings)

	req, _ := http.NewRequest("GET", "/api/axenta-sync/settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestUpdateAxentaSyncSettings_Unauthorized тестирует UpdateAxentaSyncSettings без авторизации
func TestUpdateAxentaSyncSettings_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.PUT("/api/axenta-sync/settings", UpdateAxentaSyncSettings)

	reqBody := map[string]interface{}{
		"sync_interval": 15,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/api/axenta-sync/settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestUpdateAxentaSyncSettings_ValidationError тестирует UpdateAxentaSyncSettings с ошибкой валидации
func TestUpdateAxentaSyncSettings_ValidationError(t *testing.T) {
	setupAxentaSyncSettingsTestDB(t)
	router := setupAxentaSyncSettingsTestRouter(t, 123)

	router.PUT("/api/axenta-sync/settings", UpdateAxentaSyncSettings)

	// Тест с неверным интервалом (меньше 1)
	reqBody := map[string]interface{}{
		"sync_interval": 0,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/api/axenta-sync/settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestUpdateAxentaSyncSettings_NoScheduler тестирует UpdateAxentaSyncSettings когда планировщик не инициализирован
func TestUpdateAxentaSyncSettings_NoScheduler(t *testing.T) {
	setupAxentaSyncSettingsTestDB(t)
	router := setupAxentaSyncSettingsTestRouter(t, 123)

	// Убеждаемся, что планировщик не установлен
	services.SetAxentaSyncScheduler(nil)

	router.PUT("/api/axenta-sync/settings", UpdateAxentaSyncSettings)

	reqBody := map[string]interface{}{
		"sync_interval": 15,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/api/axenta-sync/settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
