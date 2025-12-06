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
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupBillingTestDB создает тестовую базу данных для биллинга
func setupBillingTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.BillingPlan{},
		&models.Company{},
		&models.Contract{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupBillingTestRouter создает тестовый роутер с middleware для установки admin_account_id
func setupBillingTestRouter(t *testing.T, adminAccountID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки admin_account_id в контекст
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

// TestGetBillingPlans_Unauthorized тестирует GetBillingPlans без авторизации
func TestGetBillingPlans_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/billing/plans", GetBillingPlans)

	req, _ := http.NewRequest("GET", "/billing/plans", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestGetBillingPlans_Success тестирует успешное получение списка тарифных планов
func TestGetBillingPlans_Success(t *testing.T) {
	db := setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	// Создаем тестовые тарифные планы
	plan1 := models.BillingPlan{
		AdminAccountID: 123,
		Name:           "Basic Plan",
		Price:          decimal.NewFromInt(1000),
		Currency:       "RUB",
		BillingPeriod:  "monthly",
		IsActive:       true,
	}
	plan2 := models.BillingPlan{
		AdminAccountID: 123,
		Name:           "Premium Plan",
		Price:          decimal.NewFromInt(2000),
		Currency:       "RUB",
		BillingPeriod:  "monthly",
		IsActive:       true,
	}
	db.Create(&plan1)
	db.Create(&plan2)

	router.GET("/billing/plans", GetBillingPlans)

	req, _ := http.NewRequest("GET", "/billing/plans", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	data, ok := response["data"].([]interface{})
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(data), 2)
}

// TestGetBillingPlans_WithCompanyFilter тестирует GetBillingPlans с фильтром по company_id
func TestGetBillingPlans_WithCompanyFilter(t *testing.T) {
	db := setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	companyID := uint(456)
	plan1 := models.BillingPlan{
		AdminAccountID: 123,
		Name:           "Company Plan",
		Price:          decimal.NewFromInt(1500),
		Currency:       "RUB",
		BillingPeriod:  "monthly",
		IsActive:       true,
		CompanyID:      &companyID,
	}
	db.Create(&plan1)

	router.GET("/billing/plans", GetBillingPlans)

	req, _ := http.NewRequest("GET", "/billing/plans?company_id=456", nil)
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

// TestGetBillingPlan_Unauthorized тестирует GetBillingPlan без авторизации
func TestGetBillingPlan_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/billing/plans/:id", GetBillingPlan)

	req, _ := http.NewRequest("GET", "/billing/plans/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestGetBillingPlan_NoCompanyID тестирует GetBillingPlan без company_id параметра
func TestGetBillingPlan_NoCompanyID(t *testing.T) {
	setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	router.GET("/billing/plans/:id", GetBillingPlan)

	req, _ := http.NewRequest("GET", "/billing/plans/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["error"], "company_id обязателен")
}

// TestGetBillingPlan_NotFound тестирует GetBillingPlan когда план не найден
func TestGetBillingPlan_NotFound(t *testing.T) {
	setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	router.GET("/billing/plans/:id", GetBillingPlan)

	req, _ := http.NewRequest("GET", "/billing/plans/999?company_id=456", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestGetBillingPlan_Success тестирует успешное получение тарифного плана
func TestGetBillingPlan_Success(t *testing.T) {
	db := setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	companyID := uint(456)
	plan := models.BillingPlan{
		AdminAccountID: 123,
		Name:           "Test Plan",
		Price:          decimal.NewFromInt(1000),
		Currency:       "RUB",
		BillingPeriod:  "monthly",
		IsActive:       true,
		CompanyID:      &companyID,
	}
	db.Create(&plan)

	router.GET("/billing/plans/:id", GetBillingPlan)

	req, _ := http.NewRequest("GET", "/billing/plans/1?company_id=456", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
	assert.NotNil(t, response["data"])
}

// TestCreateBillingPlan_Unauthorized тестирует CreateBillingPlan без авторизации
func TestCreateBillingPlan_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.POST("/billing/plans", CreateBillingPlan)

	reqBody := map[string]interface{}{
		"name":           "Test Plan",
		"price":          1000,
		"currency":       "RUB",
		"billing_period": "monthly",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/billing/plans", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestCreateBillingPlan_ValidationError тестирует CreateBillingPlan с ошибкой валидации
func TestCreateBillingPlan_ValidationError(t *testing.T) {
	setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	router.POST("/billing/plans", CreateBillingPlan)

	// Тест с пустым именем
	reqBody := map[string]interface{}{
		"name":  "",
		"price": 1000,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/billing/plans", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestCreateBillingPlan_NegativePrice тестирует CreateBillingPlan с отрицательной ценой
func TestCreateBillingPlan_NegativePrice(t *testing.T) {
	setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	router.POST("/billing/plans", CreateBillingPlan)

	reqBody := map[string]interface{}{
		"name":           "Test Plan",
		"price":          -100,
		"currency":       "RUB",
		"billing_period": "monthly",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/billing/plans", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["error"], "отрицательной")
}

// TestCreateBillingPlan_Success тестирует успешное создание тарифного плана
func TestCreateBillingPlan_Success(t *testing.T) {
	db := setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	router.POST("/billing/plans", CreateBillingPlan)

	reqBody := map[string]interface{}{
		"name":           "Test Plan",
		"description":    "Test Description",
		"price":          1000,
		"currency":       "RUB",
		"billing_period": "monthly",
		"max_devices":    10,
		"max_users":      5,
		"is_active":      true,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/billing/plans?company_id=456", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	// Проверяем, что план создан в БД
	var plan models.BillingPlan
	err = db.Where("name = ? AND admin_account_id = ?", "Test Plan", 123).First(&plan).Error
	require.NoError(t, err)
	assert.Equal(t, "Test Plan", plan.Name)
	assert.Equal(t, decimal.NewFromInt(1000), plan.Price)
}

// TestCreateBillingPlan_DuplicateName тестирует CreateBillingPlan с дублирующимся именем
func TestCreateBillingPlan_DuplicateName(t *testing.T) {
	db := setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	companyID := uint(456)
	// Создаем существующий план
	existingPlan := models.BillingPlan{
		AdminAccountID: 123,
		Name:           "Existing Plan",
		Price:          decimal.NewFromInt(1000),
		Currency:       "RUB",
		BillingPeriod:  "monthly",
		IsActive:       true,
		CompanyID:      &companyID,
	}
	db.Create(&existingPlan)

	router.POST("/billing/plans", CreateBillingPlan)

	reqBody := map[string]interface{}{
		"name":           "Existing Plan",
		"price":          2000,
		"currency":       "RUB",
		"billing_period": "monthly",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/billing/plans?company_id=456", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["error"], "уже существует")
}

// TestCalculateBilling_Unauthorized тестирует CalculateBilling без авторизации
func TestCalculateBilling_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/billing/contracts/:contract_id/calculate", CalculateBilling)

	req, _ := http.NewRequest("GET", "/billing/contracts/1/calculate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestCalculateBilling_InvalidContractID тестирует CalculateBilling с неверным contract_id
func TestCalculateBilling_InvalidContractID(t *testing.T) {
	setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	router.GET("/billing/contracts/:contract_id/calculate", CalculateBilling)

	req, _ := http.NewRequest("GET", "/billing/contracts/invalid/calculate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestCalculateBilling_InvalidPeriodFormat тестирует CalculateBilling с неверным форматом периода
func TestCalculateBilling_InvalidPeriodFormat(t *testing.T) {
	setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	router.GET("/billing/contracts/:contract_id/calculate", CalculateBilling)

	req, _ := http.NewRequest("GET", "/billing/contracts/1/calculate?period_start=invalid&period_end=invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestGenerateInvoice_Unauthorized тестирует GenerateInvoice без авторизации
func TestGenerateInvoice_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.POST("/billing/contracts/:contract_id/invoices", GenerateInvoice)

	reqBody := map[string]interface{}{
		"period_start": "2025-01-01",
		"period_end":   "2025-01-31",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/billing/contracts/1/invoices", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestGenerateInvoice_InvalidContractID тестирует GenerateInvoice с неверным contract_id
func TestGenerateInvoice_InvalidContractID(t *testing.T) {
	setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	router.POST("/billing/contracts/:contract_id/invoices", GenerateInvoice)

	reqBody := map[string]interface{}{
		"period_start": "2025-01-01",
		"period_end":   "2025-01-31",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/billing/contracts/invalid/invoices", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestGenerateInvoice_InvalidPeriodFormat тестирует GenerateInvoice с неверным форматом периода
func TestGenerateInvoice_InvalidPeriodFormat(t *testing.T) {
	setupBillingTestDB(t)
	router := setupBillingTestRouter(t, 123)

	router.POST("/billing/contracts/:contract_id/invoices", GenerateInvoice)

	reqBody := map[string]interface{}{
		"period_start": "invalid",
		"period_end":   "invalid",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/billing/contracts/1/invoices", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}
