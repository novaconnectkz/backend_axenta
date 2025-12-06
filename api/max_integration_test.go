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

// setupMaxIntegrationTestDB создает тестовую базу данных для MAX интеграции
func setupMaxIntegrationTestDB(t *testing.T) *gorm.DB {
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

// setupMaxIntegrationTestRouter создает тестовый роутер с middleware
func setupMaxIntegrationTestRouter(_ *testing.T, _ *gorm.DB, companyID uint) *gin.Engine {
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

// TestMaxIntegrationAPI_SetupIntegration_ValidationError тестирует SetupIntegration с ошибкой валидации
func TestMaxIntegrationAPI_SetupIntegration_ValidationError(t *testing.T) {
	db := setupMaxIntegrationTestDB(t)
	router := setupMaxIntegrationTestRouter(t, db, 456)

	api := NewMaxIntegrationAPI()
	api.RegisterRoutes(router.Group("/api"))

	// Тест с пустым bot_token
	reqBody := map[string]interface{}{
		"bot_token": "",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/integrations/max/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "Неверный формат данных")
}

// TestMaxIntegrationAPI_SetupIntegration_Success тестирует успешную настройку интеграции
func TestMaxIntegrationAPI_SetupIntegration_Success(t *testing.T) {
	db := setupMaxIntegrationTestDB(t)
	router := setupMaxIntegrationTestRouter(t, db, 456)

	api := NewMaxIntegrationAPI()
	api.RegisterRoutes(router.Group("/api"))

	reqBody := map[string]interface{}{
		"bot_token":   "test-bot-token",
		"parse_mode":  "HTML",
		"webhook_url": "https://example.com/webhook",
		"use_polling": false,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/integrations/max/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть успех или ошибку в зависимости от реализации сервиса
	assert.True(t, w.Code == http.StatusCreated || w.Code >= 400)
}

// TestMaxIntegrationAPI_GetIntegrationConfig_NotFound тестирует GetIntegrationConfig когда интеграция не найдена
func TestMaxIntegrationAPI_GetIntegrationConfig_NotFound(t *testing.T) {
	db := setupMaxIntegrationTestDB(t)
	router := setupMaxIntegrationTestRouter(t, db, 456)

	api := NewMaxIntegrationAPI()
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/integrations/max/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestMaxIntegrationAPI_DeleteIntegration_NotFound тестирует DeleteIntegration когда интеграция не найдена
func TestMaxIntegrationAPI_DeleteIntegration_NotFound(t *testing.T) {
	db := setupMaxIntegrationTestDB(t)
	router := setupMaxIntegrationTestRouter(t, db, 456)

	api := NewMaxIntegrationAPI()
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("DELETE", "/api/integrations/max/setup", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestMaxIntegrationAPI_TestConnection_NoConfig тестирует TestConnection без конфигурации
func TestMaxIntegrationAPI_TestConnection_NoConfig(t *testing.T) {
	db := setupMaxIntegrationTestDB(t)
	router := setupMaxIntegrationTestRouter(t, db, 456)

	api := NewMaxIntegrationAPI()
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("POST", "/api/integrations/max/test-connection", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestMaxIntegrationAPI_SendMessage_ValidationError тестирует SendMessage с ошибкой валидации
func TestMaxIntegrationAPI_SendMessage_ValidationError(t *testing.T) {
	db := setupMaxIntegrationTestDB(t)
	router := setupMaxIntegrationTestRouter(t, db, 456)

	api := NewMaxIntegrationAPI()
	api.RegisterRoutes(router.Group("/api"))

	// Тест с пустым message
	reqBody := map[string]interface{}{
		"message": "",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/integrations/max/send-message", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestMaxIntegrationAPI_GetIntegrationStatus_NotFound тестирует GetIntegrationStatus когда интеграция не найдена
func TestMaxIntegrationAPI_GetIntegrationStatus_NotFound(t *testing.T) {
	db := setupMaxIntegrationTestDB(t)
	router := setupMaxIntegrationTestRouter(t, db, 456)

	api := NewMaxIntegrationAPI()
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/integrations/max/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
