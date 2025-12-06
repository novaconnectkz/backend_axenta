package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"bytes"
	"encoding/json"
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
)

// setupPartnerSnapshotsTestDB создает тестовую базу данных для partner snapshots
func setupPartnerSnapshotsTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Contract{},
		&models.PartnerDailySnapshot{},
		&models.Company{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupPartnerSnapshotsTestRouter создает тестовый роутер с middleware
func setupPartnerSnapshotsTestRouter(_ *testing.T, db *gorm.DB, adminAccountID uint, companyID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки admin_account_id и tenant_db в контекст
	router.Use(func(c *gin.Context) {
		c.Set("user", map[string]interface{}{
			"id":        1,
			"username":  "testuser",
			"accountId": float64(adminAccountID),
		})
		c.Set("admin_account_id", adminAccountID)
		c.Set("company_id", companyID)
		c.Set("tenant_db", db) // Используем ту же БД для tenant
		c.Next()
	})

	return router
}

// TestGetPartnerContractSnapshots_Unauthorized тестирует GetPartnerContractSnapshots без авторизации
func TestGetPartnerContractSnapshots_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/partner/contracts/:contract_id/snapshots", GetPartnerContractSnapshots)

	req, _ := http.NewRequest("GET", "/partner/contracts/1/snapshots", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestGetPartnerContractSnapshots_NoTenantDB тестирует GetPartnerContractSnapshots без tenant_db
func TestGetPartnerContractSnapshots_NoTenantDB(t *testing.T) {
	setupPartnerSnapshotsTestDB(t)
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.Use(func(c *gin.Context) {
		c.Set("admin_account_id", uint(123))
		c.Next()
	})

	router.GET("/partner/contracts/:contract_id/snapshots", GetPartnerContractSnapshots)

	req, _ := http.NewRequest("GET", "/partner/contracts/1/snapshots", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestGetPartnerContractSnapshots_ContractNotFound тестирует GetPartnerContractSnapshots когда договор не найден
func TestGetPartnerContractSnapshots_ContractNotFound(t *testing.T) {
	db := setupPartnerSnapshotsTestDB(t)
	router := setupPartnerSnapshotsTestRouter(t, db, 123, 456)

	router.GET("/partner/contracts/:contract_id/snapshots", GetPartnerContractSnapshots)

	req, _ := http.NewRequest("GET", "/partner/contracts/999/snapshots", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestGetPartnerContractSnapshots_InvalidDateFormat тестирует GetPartnerContractSnapshots с неверным форматом даты
func TestGetPartnerContractSnapshots_InvalidDateFormat(t *testing.T) {
	db := setupPartnerSnapshotsTestDB(t)
	router := setupPartnerSnapshotsTestRouter(t, db, 123, 456)

	// Создаем договор
	contract := models.Contract{
		ID:             1,
		AdminAccountID: 123,
		Number:         "T-001",
		Status:         "active",
	}
	db.Create(&contract)

	router.GET("/partner/contracts/:contract_id/snapshots", GetPartnerContractSnapshots)

	req, _ := http.NewRequest("GET", "/partner/contracts/1/snapshots?start_date=invalid&end_date=invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestGetPartnerContractSnapshots_Success тестирует успешное получение снимков
func TestGetPartnerContractSnapshots_Success(t *testing.T) {
	db := setupPartnerSnapshotsTestDB(t)
	router := setupPartnerSnapshotsTestRouter(t, db, 123, 456)

	// Создаем договор
	contract := models.Contract{
		ID:             1,
		AdminAccountID: 123,
		Number:         "T-001",
		Status:         "active",
	}
	db.Create(&contract)

	// Создаем тарифный план
	plan := models.BillingPlan{
		ID:             1,
		AdminAccountID: 123,
		Name:           "Test Plan",
		Price:          decimal.NewFromInt(30000),
		Currency:       "RUB",
		BillingPeriod:  "monthly",
		IsActive:       true,
	}
	db.Create(&plan)

	// Создаем снимок
	snapshot := models.PartnerDailySnapshot{
		ContractID:         1,
		AdminAccountID:     123,
		CompanyID:          456,
		PartnerCompanyID:   789,
		TariffPlanID:       plan.ID,
		SnapshotDate:       time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		TotalObjectsCount:  100,
		ActiveObjectsCount: 90,
		MonthlyPrice:       decimal.NewFromInt(30000),
		DailyPrice:         decimal.NewFromInt(1000),
		DailyCost:          decimal.NewFromInt(1000),
		CostBeforeDiscount: decimal.NewFromInt(1000),
	}
	db.Create(&snapshot)

	router.GET("/partner/contracts/:contract_id/snapshots", GetPartnerContractSnapshots)

	req, _ := http.NewRequest("GET", "/partner/contracts/1/snapshots?start_date=2025-01-01&end_date=2025-01-31", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	data, ok := response["data"].([]interface{})
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(data), 1)
}

// TestGetPartnerContractSnapshots_DefaultPeriod тестирует GetPartnerContractSnapshots с периодом по умолчанию
func TestGetPartnerContractSnapshots_DefaultPeriod(t *testing.T) {
	db := setupPartnerSnapshotsTestDB(t)
	router := setupPartnerSnapshotsTestRouter(t, db, 123, 456)

	// Создаем договор
	contract := models.Contract{
		ID:             1,
		AdminAccountID: 123,
		Number:         "T-001",
		Status:         "active",
	}
	db.Create(&contract)

	router.GET("/partner/contracts/:contract_id/snapshots", GetPartnerContractSnapshots)

	req, _ := http.NewRequest("GET", "/partner/contracts/1/snapshots", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestCreatePartnerSnapshots_Unauthorized тестирует CreatePartnerSnapshots без авторизации
func TestCreatePartnerSnapshots_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.POST("/partner/contracts/:contract_id/snapshots", CreatePartnerSnapshots)

	reqBody := map[string]interface{}{
		"start_date": "2025-01-01",
		"end_date":   "2025-01-31",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/partner/contracts/1/snapshots", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestCreatePartnerSnapshots_ValidationError тестирует CreatePartnerSnapshots с ошибкой валидации
func TestCreatePartnerSnapshots_ValidationError(t *testing.T) {
	db := setupPartnerSnapshotsTestDB(t)
	router := setupPartnerSnapshotsTestRouter(t, db, 123, 456)

	router.POST("/partner/contracts/:contract_id/snapshots", CreatePartnerSnapshots)

	// Тест с неверным форматом даты
	reqBody := map[string]interface{}{
		"start_date": "invalid",
		"end_date":   "invalid",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/partner/contracts/1/snapshots", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestCreatePartnerSnapshots_ContractNotFound тестирует CreatePartnerSnapshots когда договор не найден
func TestCreatePartnerSnapshots_ContractNotFound(t *testing.T) {
	db := setupPartnerSnapshotsTestDB(t)
	router := setupPartnerSnapshotsTestRouter(t, db, 123, 456)

	router.POST("/partner/contracts/:contract_id/snapshots", CreatePartnerSnapshots)

	reqBody := map[string]interface{}{
		"start_date": "2025-01-01",
		"end_date":   "2025-01-31",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/partner/contracts/999/snapshots", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestGeneratePartnerSnapshotsForPeriod_Unauthorized тестирует GeneratePartnerSnapshotsForPeriod без авторизации
func TestGeneratePartnerSnapshotsForPeriod_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.POST("/partner/contracts/:contract_id/snapshots/generate", GeneratePartnerSnapshotsForPeriod)

	reqBody := map[string]interface{}{
		"start_date": "2025-01-01",
		"end_date":   "2025-01-31",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/partner/contracts/1/snapshots/generate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestGeneratePartnerSnapshotsForPeriod_ValidationError тестирует GeneratePartnerSnapshotsForPeriod с ошибкой валидации
func TestGeneratePartnerSnapshotsForPeriod_ValidationError(t *testing.T) {
	db := setupPartnerSnapshotsTestDB(t)
	router := setupPartnerSnapshotsTestRouter(t, db, 123, 456)

	router.POST("/partner/contracts/:contract_id/snapshots/generate", GeneratePartnerSnapshotsForPeriod)

	// Тест с неверным форматом даты
	reqBody := map[string]interface{}{
		"start_date": "invalid",
		"end_date":   "invalid",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/partner/contracts/1/snapshots/generate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGeneratePartnerSnapshotsForPeriod_ContractNotFound тестирует GeneratePartnerSnapshotsForPeriod когда договор не найден
func TestGeneratePartnerSnapshotsForPeriod_ContractNotFound(t *testing.T) {
	db := setupPartnerSnapshotsTestDB(t)
	router := setupPartnerSnapshotsTestRouter(t, db, 123, 456)

	router.POST("/partner/contracts/:contract_id/snapshots/generate", GeneratePartnerSnapshotsForPeriod)

	reqBody := map[string]interface{}{
		"start_date": "2025-01-01",
		"end_date":   "2025-01-31",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/partner/contracts/999/snapshots/generate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
