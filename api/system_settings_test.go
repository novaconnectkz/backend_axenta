package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
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

// setupSystemSettingsTestDB создает тестовую базу данных для system settings
func setupSystemSettingsTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.SystemSettings{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupSystemSettingsTestRouter создает тестовый роутер с middleware
func setupSystemSettingsTestRouter(_ *testing.T, _ *gorm.DB, adminAccountID uint) *gin.Engine {
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

// TestGetSystemSettings_Unauthorized тестирует GetSystemSettings без авторизации
func TestGetSystemSettings_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/api/system/settings", GetSystemSettings)

	req, _ := http.NewRequest("GET", "/api/system/settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestGetSystemSettings_NoCompanyID тестирует GetSystemSettings без company_id
func TestGetSystemSettings_NoCompanyID(t *testing.T) {
	setupSystemSettingsTestDB(t)
	router := setupSystemSettingsTestRouter(t, database.DB, 123)

	router.GET("/api/system/settings", GetSystemSettings)

	req, _ := http.NewRequest("GET", "/api/system/settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["error"], "company_id обязателен")
}

// TestGetSystemSettings_InvalidCompanyID тестирует GetSystemSettings с неверным company_id
func TestGetSystemSettings_InvalidCompanyID(t *testing.T) {
	setupSystemSettingsTestDB(t)
	router := setupSystemSettingsTestRouter(t, database.DB, 123)

	router.GET("/api/system/settings", GetSystemSettings)

	req, _ := http.NewRequest("GET", "/api/system/settings?company_id=invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetSystemSettings_CreatesDefault тестирует GetSystemSettings создание настроек по умолчанию
func TestGetSystemSettings_CreatesDefault(t *testing.T) {
	db := setupSystemSettingsTestDB(t)
	router := setupSystemSettingsTestRouter(t, db, 123)

	router.GET("/api/system/settings", GetSystemSettings)

	req, _ := http.NewRequest("GET", "/api/system/settings?company_id=456", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	// Проверяем, что настройки созданы в БД
	var settings models.SystemSettings
	err = db.Where("company_id = ? AND admin_account_id = ?", 456, 123).First(&settings).Error
	require.NoError(t, err)
	assert.Equal(t, uint(456), settings.CompanyID)
}

// TestUpdateSystemSettings_Unauthorized тестирует UpdateSystemSettings без авторизации
func TestUpdateSystemSettings_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.PUT("/api/system/settings", UpdateSystemSettings)

	reqBody := map[string]interface{}{
		"company_id": 456,
		"timezone":   "Europe/Moscow",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/api/system/settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestUpdateSystemSettings_NoCompanyID тестирует UpdateSystemSettings без company_id
func TestUpdateSystemSettings_NoCompanyID(t *testing.T) {
	setupSystemSettingsTestDB(t)
	router := setupSystemSettingsTestRouter(t, database.DB, 123)

	router.PUT("/api/system/settings", UpdateSystemSettings)

	reqBody := map[string]interface{}{
		"timezone": "Europe/Moscow",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/api/system/settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestUpdateSystemSettings_NotFound тестирует UpdateSystemSettings когда настройки не найдены
func TestUpdateSystemSettings_NotFound(t *testing.T) {
	setupSystemSettingsTestDB(t)
	router := setupSystemSettingsTestRouter(t, database.DB, 123)

	router.PUT("/api/system/settings", UpdateSystemSettings)

	reqBody := map[string]interface{}{
		"company_id": 456,
		"timezone":   "Europe/Moscow",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/api/system/settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestUpdateSystemSettings_Success тестирует успешное обновление настроек
func TestUpdateSystemSettings_Success(t *testing.T) {
	db := setupSystemSettingsTestDB(t)
	router := setupSystemSettingsTestRouter(t, db, 123)

	// Создаем настройки
	settings := models.SystemSettings{
		AdminAccountID: 123,
		CompanyID:      456,
		Timezone:       "Europe/Moscow",
		Currency:       "RUB",
	}
	db.Create(&settings)

	router.PUT("/api/system/settings", UpdateSystemSettings)

	reqBody := map[string]interface{}{
		"company_id": 456,
		"timezone":   "Europe/Kiev",
		"currency":   "USD",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/api/system/settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	// Проверяем, что настройки обновлены
	var updatedSettings models.SystemSettings
	db.First(&updatedSettings, settings.ID)
	assert.Equal(t, "Europe/Kiev", updatedSettings.Timezone)
	assert.Equal(t, "USD", updatedSettings.Currency)
}
