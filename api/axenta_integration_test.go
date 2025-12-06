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

// setupAxentaIntegrationTestDB создает тестовую базу данных для Axenta интеграции
func setupAxentaIntegrationTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Integration{},
		&models.IntegrationError{},
		&models.Company{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupAxentaIntegrationTestRouter создает тестовый роутер с middleware для установки company_id
func setupAxentaIntegrationTestRouter(_ *testing.T, db *gorm.DB, companyID uint) (*gin.Engine, *AxentaIntegrationAPI) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки company_id в контекст
	router.Use(func(c *gin.Context) {
		c.Set("user", map[string]interface{}{
			"id":        1,
			"username":  "testuser",
			"accountId": float64(123),
		})
		c.Set("company_id", companyID)
		c.Next()
	})

	api := NewAxentaIntegrationAPI(db)
	return router, api
}

// TestAxentaIntegrationAPI_SetupIntegration_Unauthorized тестирует SetupIntegration без company_id
func TestAxentaIntegrationAPI_SetupIntegration_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	db := setupAxentaIntegrationTestDB(t)
	api := NewAxentaIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	reqBody := map[string]interface{}{
		"api_url":  "https://axenta.cloud/api",
		"username": "testuser",
		"password": "password123",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/axenta/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestAxentaIntegrationAPI_SetupIntegration_ValidationError тестирует SetupIntegration с ошибкой валидации
func TestAxentaIntegrationAPI_SetupIntegration_ValidationError(t *testing.T) {
	db := setupAxentaIntegrationTestDB(t)
	router, _ := setupAxentaIntegrationTestRouter(t, db, 456)

	api := NewAxentaIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	// Тест с пустым api_url
	reqBody := map[string]interface{}{
		"api_url":  "",
		"username": "testuser",
		"password": "password123",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/axenta/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "Неверный формат данных")
}

// TestAxentaIntegrationAPI_SetupIntegration_Success тестирует успешную настройку интеграции
func TestAxentaIntegrationAPI_SetupIntegration_Success(t *testing.T) {
	db := setupAxentaIntegrationTestDB(t)
	router, _ := setupAxentaIntegrationTestRouter(t, db, 456)

	api := NewAxentaIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	reqBody := map[string]interface{}{
		"api_url":           "https://axenta.cloud/api",
		"username":          "testuser",
		"password":          "password123",
		"sync_interval":     15,
		"auto_sync_enabled": true,
		"retry_attempts":    3,
		"timeout":           30,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/axenta/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["message"], "успешно настроена")

	// Проверяем, что интеграция создана в БД
	var integration models.Integration
	err = db.Where("company_id = ? AND integration_type = ?", 456, "axenta_cloud").First(&integration).Error
	require.NoError(t, err)
	assert.Equal(t, "axenta_cloud", integration.IntegrationType)
	assert.True(t, integration.IsActive)
}

// TestAxentaIntegrationAPI_SetupIntegration_Duplicate тестирует SetupIntegration когда интеграция уже существует
func TestAxentaIntegrationAPI_SetupIntegration_Duplicate(t *testing.T) {
	db := setupAxentaIntegrationTestDB(t)
	router, _ := setupAxentaIntegrationTestRouter(t, db, 456)

	// Создаем существующую интеграцию
	existingIntegration := models.Integration{
		CompanyID:       456,
		IntegrationType: "axenta_cloud",
		Name:            "Axenta Cloud API",
		IsActive:        true,
	}
	db.Create(&existingIntegration)

	api := NewAxentaIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	reqBody := map[string]interface{}{
		"api_url":  "https://axenta.cloud/api",
		"username": "testuser",
		"password": "password123",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/axenta/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "уже настроена")
}

// TestAxentaIntegrationAPI_GetIntegrationConfig_NotFound тестирует GetIntegrationConfig когда интеграция не найдена
func TestAxentaIntegrationAPI_GetIntegrationConfig_NotFound(t *testing.T) {
	db := setupAxentaIntegrationTestDB(t)
	router, _ := setupAxentaIntegrationTestRouter(t, db, 456)

	api := NewAxentaIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/axenta/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestAxentaIntegrationAPI_GetIntegrationConfig_Success тестирует успешное получение конфигурации
func TestAxentaIntegrationAPI_GetIntegrationConfig_Success(t *testing.T) {
	db := setupAxentaIntegrationTestDB(t)
	router, _ := setupAxentaIntegrationTestRouter(t, db, 456)

	// Создаем интеграцию
	integration := models.Integration{
		CompanyID:       456,
		IntegrationType: "axenta_cloud",
		Name:            "Axenta Cloud API",
		IsActive:        true,
		Settings:        `{"api_url":"https://axenta.cloud/api","username":"testuser"}`,
	}
	db.Create(&integration)

	api := NewAxentaIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/axenta/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotNil(t, response["config"])
}

// TestAxentaIntegrationAPI_DeleteIntegration_NotFound тестирует DeleteIntegration когда интеграция не найдена
func TestAxentaIntegrationAPI_DeleteIntegration_NotFound(t *testing.T) {
	db := setupAxentaIntegrationTestDB(t)
	router, _ := setupAxentaIntegrationTestRouter(t, db, 456)

	api := NewAxentaIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("DELETE", "/api/axenta/setup", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestAxentaIntegrationAPI_DeleteIntegration_Success тестирует успешное удаление интеграции
func TestAxentaIntegrationAPI_DeleteIntegration_Success(t *testing.T) {
	db := setupAxentaIntegrationTestDB(t)
	router, _ := setupAxentaIntegrationTestRouter(t, db, 456)

	// Создаем интеграцию
	integration := models.Integration{
		CompanyID:       456,
		IntegrationType: "axenta_cloud",
		Name:            "Axenta Cloud API",
		IsActive:        true,
	}
	db.Create(&integration)

	api := NewAxentaIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("DELETE", "/api/axenta/setup", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["message"], "удалена")

	// Проверяем, что интеграция удалена
	var deletedIntegration models.Integration
	err = db.Where("id = ?", integration.ID).First(&deletedIntegration).Error
	assert.Error(t, err) // Должна быть ошибка, так как интеграция удалена
}

// TestAxentaIntegrationAPI_GetIntegrationStatus_NotFound тестирует GetIntegrationStatus когда интеграция не найдена
func TestAxentaIntegrationAPI_GetIntegrationStatus_NotFound(t *testing.T) {
	db := setupAxentaIntegrationTestDB(t)
	router, _ := setupAxentaIntegrationTestRouter(t, db, 456)

	api := NewAxentaIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/axenta/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestAxentaIntegrationAPI_GetIntegrationStatus_Success тестирует успешное получение статуса
func TestAxentaIntegrationAPI_GetIntegrationStatus_Success(t *testing.T) {
	db := setupAxentaIntegrationTestDB(t)
	router, _ := setupAxentaIntegrationTestRouter(t, db, 456)

	// Создаем интеграцию
	integration := models.Integration{
		CompanyID:       456,
		IntegrationType: "axenta_cloud",
		Name:            "Axenta Cloud API",
		IsActive:        true,
	}
	db.Create(&integration)

	api := NewAxentaIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/axenta/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotNil(t, response["status"])
}

// TestAxentaIntegrationAPI_GetIntegrationErrors_NoErrors тестирует GetIntegrationErrors без ошибок
func TestAxentaIntegrationAPI_GetIntegrationErrors_NoErrors(t *testing.T) {
	db := setupAxentaIntegrationTestDB(t)
	router, _ := setupAxentaIntegrationTestRouter(t, db, 456)

	api := NewAxentaIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/axenta/errors", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotNil(t, response["errors"])
}

// TestAxentaIntegrationAPI_GetIntegrationErrors_WithErrors тестирует GetIntegrationErrors с ошибками
func TestAxentaIntegrationAPI_GetIntegrationErrors_WithErrors(t *testing.T) {
	db := setupAxentaIntegrationTestDB(t)
	router, _ := setupAxentaIntegrationTestRouter(t, db, 456)

	// Создаем интеграцию
	integration := models.Integration{
		CompanyID:       456,
		IntegrationType: "axenta_cloud",
		Name:            "Axenta Cloud API",
		IsActive:        true,
	}
	db.Create(&integration)

	// Создаем ошибку интеграции
	integrationError := models.IntegrationError{
		TenantID:     456,
		Service:      models.IntegrationServiceAxetnaCloud,
		Operation:    models.IntegrationOperationSync,
		ErrorCode:    "connection_error",
		ErrorMessage: "Connection timeout",
		Status:       models.IntegrationErrorStatusPending,
	}
	db.Create(&integrationError)

	api := NewAxentaIntegrationAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/axenta/errors", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	errors, ok := response["errors"].([]interface{})
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(errors), 1)
}
