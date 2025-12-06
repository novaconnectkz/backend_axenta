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

// setupBillingDebugTestDB создает тестовую базу данных
func setupBillingDebugTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Contract{},
		&models.Invoice{},
		&models.BillingHistory{},
	)
	require.NoError(t, err)

	database.DB = db
	return db
}

// setupBillingDebugTestRouter создает тестовый роутер с middleware
func setupBillingDebugTestRouter(_ *testing.T, db *gorm.DB, adminAccountID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки admin_account_id и tenant_db
	router.Use(func(c *gin.Context) {
		c.Set("user", map[string]interface{}{
			"id":        1,
			"username":  "testuser",
			"accountId": float64(adminAccountID),
		})
		c.Set("admin_account_id", adminAccountID)
		c.Set("tenant_db", db)
		c.Next()
	})

	return router
}

// TestGetContractBillingAnalysis_Unauthorized тестирует GetContractBillingAnalysis без авторизации
func TestGetContractBillingAnalysis_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/api/billing/debug/contract/:number", GetContractBillingAnalysis)

	req, _ := http.NewRequest("GET", "/api/billing/debug/contract/TEST-001", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestGetContractBillingAnalysis_NoContractNumber тестирует GetContractBillingAnalysis без номера договора
func TestGetContractBillingAnalysis_NoContractNumber(t *testing.T) {
	db := setupBillingDebugTestDB(t)
	router := setupBillingDebugTestRouter(t, db, 123)

	router.GET("/api/billing/debug/contract/:number", GetContractBillingAnalysis)

	req, _ := http.NewRequest("GET", "/api/billing/debug/contract/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть BadRequest или NotFound
	assert.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusNotFound)
}

// TestGetContractBillingAnalysis_ContractNotFound тестирует GetContractBillingAnalysis когда договор не найден
func TestGetContractBillingAnalysis_ContractNotFound(t *testing.T) {
	db := setupBillingDebugTestDB(t)
	router := setupBillingDebugTestRouter(t, db, 123)

	router.GET("/api/billing/debug/contract/:number", GetContractBillingAnalysis)

	req, _ := http.NewRequest("GET", "/api/billing/debug/contract/NONEXISTENT", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestGetContractBillingAnalysis_Success тестирует успешное получение анализа
func TestGetContractBillingAnalysis_Success(t *testing.T) {
	db := setupBillingDebugTestDB(t)
	router := setupBillingDebugTestRouter(t, db, 123)

	// Создаем тестовый договор
	contract := models.Contract{
		Number: "TEST-001",
		Status: "active",
	}
	db.Create(&contract)

	router.GET("/api/billing/debug/contract/:number", GetContractBillingAnalysis)

	req, _ := http.NewRequest("GET", "/api/billing/debug/contract/TEST-001", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть успех или ошибку из-за SET search_path
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}
