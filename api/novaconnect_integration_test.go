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

// setupNovaConnectIntegrationTestDB создает тестовую базу данных для NovaConnect интеграции
func setupNovaConnectIntegrationTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Integration{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupNovaConnectIntegrationTestRouter создает тестовый роутер с middleware
func setupNovaConnectIntegrationTestRouter(_ *testing.T, _ *gorm.DB, companyID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки company_id
	router.Use(func(c *gin.Context) {
		c.Set("user", map[string]interface{}{
			"id":        1,
			"username":  "testuser",
			"accountId": float64(123),
		})
		c.Set("company_id", companyID)
		c.Next()
	})

	return router
}

// TestNovaConnectIntegrationAPI_SetupIntegration_NoCompanyID тестирует SetupIntegration без company_id
func TestNovaConnectIntegrationAPI_SetupIntegration_NoCompanyID(t *testing.T) {
	db := setupNovaConnectIntegrationTestDB(t)
	router := setupNovaConnectIntegrationTestRouter(t, db, 0) // company_id = 0

	api := NewNovaConnectIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	reqBody := map[string]interface{}{
		"api_url": "https://api.novaconnect.kz/api",
		"token":   "test-token",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/novaconnect/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestNovaConnectIntegrationAPI_SetupIntegration_NoToken тестирует SetupIntegration без токена
func TestNovaConnectIntegrationAPI_SetupIntegration_NoToken(t *testing.T) {
	db := setupNovaConnectIntegrationTestDB(t)
	router := setupNovaConnectIntegrationTestRouter(t, db, 456)

	api := NewNovaConnectIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	reqBody := map[string]interface{}{
		"api_url": "https://api.novaconnect.kz/api",
		"token":   "",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/novaconnect/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "Токен обязателен")
}

// TestNovaConnectIntegrationAPI_SetupIntegration_Success тестирует успешную настройку интеграции
func TestNovaConnectIntegrationAPI_SetupIntegration_Success(t *testing.T) {
	db := setupNovaConnectIntegrationTestDB(t)
	router := setupNovaConnectIntegrationTestRouter(t, db, 456)

	api := NewNovaConnectIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	reqBody := map[string]interface{}{
		"api_url":           "https://api.novaconnect.kz/api",
		"token":             "test-token",
		"language":          "ru",
		"webhook_url":       "https://example.com/webhook",
		"webhook_enabled":   true,
		"sync_interval":     15,
		"auto_sync_enabled": true,
		"enabled":           true,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/novaconnect/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["message"], "успешно настроена")
}

// TestNovaConnectIntegrationAPI_GetIntegrationConfig_NotFound тестирует GetIntegrationConfig когда интеграция не найдена
func TestNovaConnectIntegrationAPI_GetIntegrationConfig_NotFound(t *testing.T) {
	db := setupNovaConnectIntegrationTestDB(t)
	router := setupNovaConnectIntegrationTestRouter(t, db, 456)

	api := NewNovaConnectIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/novaconnect/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestNovaConnectIntegrationAPI_DeleteIntegration_NotFound тестирует DeleteIntegration когда интеграция не найдена
func TestNovaConnectIntegrationAPI_DeleteIntegration_NotFound(t *testing.T) {
	db := setupNovaConnectIntegrationTestDB(t)
	router := setupNovaConnectIntegrationTestRouter(t, db, 456)

	api := NewNovaConnectIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("DELETE", "/api/novaconnect/setup", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestNovaConnectIntegrationAPI_TestConnection_NoConfig тестирует TestConnection без конфигурации
func TestNovaConnectIntegrationAPI_TestConnection_NoConfig(t *testing.T) {
	db := setupNovaConnectIntegrationTestDB(t)
	router := setupNovaConnectIntegrationTestRouter(t, db, 456)

	api := NewNovaConnectIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("POST", "/api/novaconnect/test-connection", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
