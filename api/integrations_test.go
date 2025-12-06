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

// setupIntegrationsTestDB создает тестовую базу данных для integrations API
func setupIntegrationsTestDB(t *testing.T) *gorm.DB {
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

// setupIntegrationsTestRouter создает тестовый роутер с middleware
func setupIntegrationsTestRouter(_ *testing.T, _ *gorm.DB, companyID uint) *gin.Engine {
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

// TestIntegrationsAPI_GetIntegrations_NoCompanyID тестирует GetIntegrations без company_id
func TestIntegrationsAPI_GetIntegrations_NoCompanyID(t *testing.T) {
	db := setupIntegrationsTestDB(t)
	router := setupIntegrationsTestRouter(t, db, 0) // company_id = 0

	api := NewIntegrationsAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/integrations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestIntegrationsAPI_GetIntegrations_NoIntegrations тестирует GetIntegrations без интеграций
func TestIntegrationsAPI_GetIntegrations_NoIntegrations(t *testing.T) {
	db := setupIntegrationsTestDB(t)
	router := setupIntegrationsTestRouter(t, db, 456)

	api := NewIntegrationsAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/integrations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotNil(t, response["integrations"])
}

// TestIntegrationsAPI_GetIntegrations_WithIntegrations тестирует GetIntegrations с интеграциями
func TestIntegrationsAPI_GetIntegrations_WithIntegrations(t *testing.T) {
	db := setupIntegrationsTestDB(t)
	router := setupIntegrationsTestRouter(t, db, 456)

	// Создаем интеграцию
	integration := models.Integration{
		CompanyID:       456,
		IntegrationType: "axenta_cloud",
		Name:            "Axenta Cloud API",
		IsActive:        true,
	}
	db.Create(&integration)

	api := NewIntegrationsAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	req, _ := http.NewRequest("GET", "/api/integrations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	integrations, ok := response["integrations"].([]interface{})
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(integrations), 1)
}
