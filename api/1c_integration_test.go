package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"backend_axenta/models"
	"backend_axenta/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupOneCAPITest() (*gin.Engine, *gorm.DB, *services.OneCClientMock, *models.Company) {
	// Создаем временную БД в памяти
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	// Мигрируем схему
	db.AutoMigrate(
		&models.Company{},
		&models.User{},
		&models.Invoice{},
		&models.InvoiceItem{},
		&models.Integration{},
		&services.OneCIntegrationError{},
		&models.Contract{},
		&models.TariffPlan{},
	)

	// Создаем тестовую компанию
	company := &models.Company{
		Name:      "Тестовая компания",
		Domain:    "test.example.com",
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	db.Create(company)

	// Создаем мок клиент и сервисы
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	mockClient := services.NewOneCClientMock(logger)
	mockClient.SetupMockData()

	cacheService := services.NewCacheService(nil, logger)
	integrationService := services.NewOneCIntegrationService(db, nil, cacheService, logger)

	// Устанавливаем mock client
	integrationService.SetOneCClient(mockClient)

	// Создаем API с прямым присваиванием зависимостей
	api := &OneCIntegrationAPI{
		db:                     db,
		oneCIntegrationService: integrationService,
	}

	// Настраиваем Gin
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Добавляем middleware для установки company_id
	r.Use(func(c *gin.Context) {
		c.Set("company_id", company.ID)
		c.Next()
	})

	// Регистрируем маршруты
	apiGroup := r.Group("/api")
	api.RegisterRoutes(apiGroup)

	return r, db, mockClient, company
}

func createTestOneCIntegration(db *gorm.DB, companyID uuid.UUID) *models.Integration {
	config := services.OneCIntegrationConfig{
		CompanyID:         companyID,
		BaseURL:           "http://test.1c.local",
		Username:          "testuser",
		Password:          "testpass",
		Database:          "testdb",
		APIVersion:        "v1",
		OrganizationCode:  "ORG001",
		BankAccountCode:   "BANK001",
		PaymentTypeCode:   "PAYMENT001",
		ContractTypeCode:  "CONTRACT001",
		CurrencyCode:      "RUB",
		AutoExportEnabled: true,
		AutoImportEnabled: true,
		SyncInterval:      60,
	}

	configJSON, _ := json.Marshal(config)
	integration := &models.Integration{
		CompanyID:       companyID,
		IntegrationType: "1c",
		Name:            "Интеграция с 1С",
		Description:     "Тестовая интеграция",
		Settings:        string(configJSON),
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	db.Create(integration)
	return integration
}

func createTestOneCInvoice(db *gorm.DB, companyID uuid.UUID, status string) *models.Invoice {
	invoice := &models.Invoice{
		Number:             "INV-TEST-001",
		Title:              "Тестовый счет",
		Description:        "Описание тестового счета",
		InvoiceDate:        time.Now(),
		DueDate:            time.Now().AddDate(0, 0, 14),
		CompanyID:          companyID,
		BillingPeriodStart: time.Now().AddDate(0, -1, 0),
		BillingPeriodEnd:   time.Now(),
		SubtotalAmount:     decimal.NewFromFloat(10000),
		TaxRate:            decimal.NewFromFloat(20),
		TaxAmount:          decimal.NewFromFloat(2000),
		TotalAmount:        decimal.NewFromFloat(12000),
		Currency:           "RUB",
		Status:             status,
		PaidAmount:         decimal.Zero,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if status == "paid" {
		paidAt := time.Now()
		invoice.PaidAt = &paidAt
		invoice.PaidAmount = invoice.TotalAmount
	}

	db.Create(invoice)
	return invoice
}

func TestOneCIntegrationAPI_SetupIntegration(t *testing.T) {
	router, db, _, company := setupOneCAPITest()

	setupReq := SetupIntegrationRequest{
		BaseURL:           "http://test.1c.local",
		Username:          "testuser",
		Password:          "testpass",
		Database:          "testdb",
		APIVersion:        "v1",
		OrganizationCode:  "ORG001",
		BankAccountCode:   "BANK001",
		PaymentTypeCode:   "PAYMENT001",
		ContractTypeCode:  "CONTRACT001",
		CurrencyCode:      "RUB",
		AutoExportEnabled: true,
		AutoImportEnabled: true,
		SyncInterval:      60,
	}

	reqBody, _ := json.Marshal(setupReq)
	req, _ := http.NewRequest("POST", "/api/1c/setup", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response, "message")
	assert.Contains(t, response, "integration_id")

	// Проверяем, что интеграция создана в БД
	var integration models.Integration
	err = db.Where("company_id = ? AND integration_type = ?", company.ID, "1c").First(&integration).Error
	require.NoError(t, err)
	assert.Equal(t, "Интеграция с 1С", integration.Name)
}

func TestOneCIntegrationAPI_SetupIntegration_AlreadyExists(t *testing.T) {
	router, db, _, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	setupReq := SetupIntegrationRequest{
		BaseURL:          "http://test.1c.local",
		Username:         "testuser",
		Password:         "testpass",
		Database:         "testdb",
		OrganizationCode: "ORG001",
		BankAccountCode:  "BANK001",
		PaymentTypeCode:  "PAYMENT001",
		ContractTypeCode: "CONTRACT001",
	}

	reqBody, _ := json.Marshal(setupReq)
	req, _ := http.NewRequest("POST", "/api/1c/setup", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "уже настроена")
}

func TestOneCIntegrationAPI_UpdateIntegration(t *testing.T) {
	router, db, _, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	updateReq := SetupIntegrationRequest{
		BaseURL:           "http://updated.1c.local",
		Username:          "updateduser",
		Password:          "updatedpass",
		Database:          "updateddb",
		OrganizationCode:  "ORG002",
		BankAccountCode:   "BANK002",
		PaymentTypeCode:   "PAYMENT002",
		ContractTypeCode:  "CONTRACT002",
		AutoExportEnabled: false,
		SyncInterval:      120,
	}

	reqBody, _ := json.Marshal(updateReq)
	req, _ := http.NewRequest("PUT", "/api/1c/setup", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем, что настройки обновились
	var integration models.Integration
	err := db.Where("company_id = ? AND integration_type = ?", company.ID, "1c").First(&integration).Error
	require.NoError(t, err)

	var config services.OneCIntegrationConfig
	err = json.Unmarshal([]byte(integration.Settings), &config)
	require.NoError(t, err)
	assert.Equal(t, "http://updated.1c.local", config.BaseURL)
	assert.Equal(t, "updateduser", config.Username)
	assert.False(t, config.AutoExportEnabled)
	assert.Equal(t, 120, config.SyncInterval)
}

func TestOneCIntegrationAPI_GetIntegrationConfig(t *testing.T) {
	router, db, _, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	req, _ := http.NewRequest("GET", "/api/1c/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response, "integration")
	assert.Contains(t, response, "config")

	config := response["config"].(map[string]interface{})
	assert.Equal(t, "http://test.1c.local", config["base_url"])
	assert.Equal(t, "testuser", config["username"])
	assert.Equal(t, "***", config["password"]) // Пароль должен быть скрыт
}

func TestOneCIntegrationAPI_DeleteIntegration(t *testing.T) {
	router, db, _, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	req, _ := http.NewRequest("DELETE", "/api/1c/setup", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем, что интеграция удалена
	var count int64
	db.Model(&models.Integration{}).Where("company_id = ? AND integration_type = ?", company.ID, "1c").Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestOneCIntegrationAPI_TestConnection(t *testing.T) {
	router, db, mockClient, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	// Тест успешного подключения
	req, _ := http.NewRequest("POST", "/api/1c/test-connection", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.True(t, response["connected"].(bool))

	// Тест неуспешного подключения
	mockClient.SetConnectionHealth(false)

	req, _ = http.NewRequest("POST", "/api/1c/test-connection", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.False(t, response["connected"].(bool))
	assert.Contains(t, response, "error")
}

func TestOneCIntegrationAPI_ExportPaymentRegistry(t *testing.T) {
	router, db, mockClient, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	// Создаем тестовые оплаченные счета
	invoice1 := createTestOneCInvoice(db, company.ID, "paid")
	invoice2 := createTestOneCInvoice(db, company.ID, "paid")
	invoice2.Number = "INV-TEST-002"
	db.Save(invoice2)

	exportReq := ExportPaymentRegistryRequest{
		InvoiceIDs:     []uint{invoice1.ID, invoice2.ID},
		RegistryNumber: "TEST-REG-001",
	}

	reqBody, _ := json.Marshal(exportReq)
	req, _ := http.NewRequest("POST", "/api/1c/export/payment-registry", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response, "message")
	assert.Equal(t, "TEST-REG-001", response["registry_number"])
	assert.Equal(t, float64(2), response["invoices_count"])

	// Проверяем, что мок клиент получил вызов
	exportedRegistries := mockClient.GetExportedRegistries()
	require.Len(t, exportedRegistries, 1)
	assert.Equal(t, "TEST-REG-001", exportedRegistries[0].RegistryNumber)
}

func TestOneCIntegrationAPI_ExportPaymentRegistry_ByDateRange(t *testing.T) {
	router, db, mockClient, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	// Создаем счет с оплатой в определенную дату
	invoice := createTestOneCInvoice(db, company.ID, "paid")
	paidAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	invoice.PaidAt = &paidAt
	db.Save(invoice)

	startDate := "2024-01-01"
	endDate := "2024-01-31"

	exportReq := ExportPaymentRegistryRequest{
		StartDate: &startDate,
		EndDate:   &endDate,
	}

	reqBody, _ := json.Marshal(exportReq)
	req, _ := http.NewRequest("POST", "/api/1c/export/payment-registry", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, float64(1), response["invoices_count"])

	// Проверяем, что реестр экспортирован
	exportedRegistries := mockClient.GetExportedRegistries()
	require.Len(t, exportedRegistries, 1)
}

func TestOneCIntegrationAPI_ExportPaymentRegistry_NoInvoices(t *testing.T) {
	router, db, _, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	exportReq := ExportPaymentRegistryRequest{
		InvoiceIDs: []uint{999}, // Несуществующий ID
	}

	reqBody, _ := json.Marshal(exportReq)
	req, _ := http.NewRequest("POST", "/api/1c/export/payment-registry", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "Нет оплаченных счетов")
}

func TestOneCIntegrationAPI_ImportCounterparties(t *testing.T) {
	router, db, _, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	req, _ := http.NewRequest("POST", "/api/1c/import/counterparties", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["message"], "успешно импортированы")

	// Проверяем, что пользователи созданы
	var users []models.User
	db.Where("company_id = ? AND user_type = ?", company.ID, "client").Find(&users)
	assert.Len(t, users, 2) // В моке есть 2 активных контрагента
}

func TestOneCIntegrationAPI_SyncPaymentStatuses(t *testing.T) {
	router, db, mockClient, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	// Создаем неоплаченный счет
	invoice := createTestOneCInvoice(db, company.ID, "sent")

	// Добавляем платеж в мок
	payment := &services.OneCPayment{
		ID:         "payment-test",
		Number:     "PAY-TEST",
		Date:       time.Now(),
		Posted:     true,
		Amount:     12000.0,
		ExternalID: fmt.Sprintf("invoice_%d", invoice.ID),
	}
	mockClient.AddPayment(fmt.Sprintf("invoice_%d", invoice.ID), payment)

	req, _ := http.NewRequest("POST", "/api/1c/sync/payment-statuses", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["message"], "успешно синхронизированы")

	// Проверяем, что статус счета обновился
	var updatedInvoice models.Invoice
	db.Where("id = ?", invoice.ID).First(&updatedInvoice)
	assert.Equal(t, "paid", updatedInvoice.Status)
}

func TestOneCIntegrationAPI_GetIntegrationErrors(t *testing.T) {
	router, db, _, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	// Создаем тестовые ошибки
	error1 := services.OneCIntegrationError{
		CompanyID:    company.ID,
		Operation:    "export_payment",
		EntityType:   "registry",
		ErrorCode:    "CONNECTION_ERROR",
		ErrorMessage: "Ошибка подключения",
		Resolved:     false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&error1)

	error2 := services.OneCIntegrationError{
		CompanyID:    company.ID,
		Operation:    "import_counterparty",
		EntityType:   "counterparty",
		ErrorCode:    "VALIDATION_ERROR",
		ErrorMessage: "Ошибка валидации",
		Resolved:     true,
		ResolvedAt:   &[]time.Time{time.Now()}[0],
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&error2)

	// Получаем нерешенные ошибки
	req, _ := http.NewRequest("GET", "/api/1c/errors?resolved=false", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, float64(1), response["count"])

	errors := response["errors"].([]interface{})
	assert.Len(t, errors, 1)
	errorData := errors[0].(map[string]interface{})
	assert.Equal(t, "CONNECTION_ERROR", errorData["error_code"])
}

func TestOneCIntegrationAPI_ResolveError(t *testing.T) {
	router, db, _, company := setupOneCAPITest()

	// Создаем тестовую ошибку
	error1 := services.OneCIntegrationError{
		CompanyID:    company.ID,
		Operation:    "export_payment",
		ErrorCode:    "CONNECTION_ERROR",
		ErrorMessage: "Ошибка подключения",
		Resolved:     false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&error1)

	req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/1c/errors/%d/resolve", error1.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["message"], "решенная")

	// Проверяем, что ошибка помечена как решенная
	var resolvedError services.OneCIntegrationError
	db.Where("id = ?", error1.ID).First(&resolvedError)
	assert.True(t, resolvedError.Resolved)
}

func TestOneCIntegrationAPI_GetIntegrationStatus(t *testing.T) {
	router, db, _, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	req, _ := http.NewRequest("GET", "/api/1c/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.True(t, response["configured"].(bool))
	assert.True(t, response["active"].(bool))
	assert.True(t, response["connection_ok"].(bool))
	assert.Equal(t, float64(0), response["errors_count"])
}

func TestOneCIntegrationAPI_GetIntegrationStatus_NotConfigured(t *testing.T) {
	router, _, _, _ := setupOneCAPITest()

	req, _ := http.NewRequest("GET", "/api/1c/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.False(t, response["configured"].(bool))
	assert.False(t, response["active"].(bool))
	assert.False(t, response["connection_ok"].(bool))
}

func TestOneCIntegrationAPI_ScheduleAutoExport(t *testing.T) {
	router, db, mockClient, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	// Создаем оплаченный счет за последний день
	invoice := createTestOneCInvoice(db, company.ID, "paid")
	paidAt := time.Now().Add(-time.Hour * 12)
	invoice.PaidAt = &paidAt
	db.Save(invoice)

	req, _ := http.NewRequest("POST", "/api/1c/export/payment-registry/auto", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["message"], "запланирован")

	// Проверяем, что реестр был экспортирован
	exportedRegistries := mockClient.GetExportedRegistries()
	require.Len(t, exportedRegistries, 1)
}

// Тесты валидации данных
func TestOneCIntegrationAPI_SetupIntegration_InvalidData(t *testing.T) {
	router, _, _, _ := setupOneCAPITest()

	// Тест с пустыми обязательными полями
	setupReq := SetupIntegrationRequest{
		BaseURL: "http://test.1c.local",
		// Отсутствуют обязательные поля
	}

	reqBody, _ := json.Marshal(setupReq)
	req, _ := http.NewRequest("POST", "/api/1c/setup", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "Неверный формат данных")
}

func TestOneCIntegrationAPI_ExportPaymentRegistry_InvalidDateFormat(t *testing.T) {
	router, db, _, company := setupOneCAPITest()
	createTestOneCIntegration(db, company.ID)

	invalidStartDate := "invalid-date"
	exportReq := ExportPaymentRegistryRequest{
		StartDate: &invalidStartDate,
		EndDate:   &invalidStartDate,
	}

	reqBody, _ := json.Marshal(exportReq)
	req, _ := http.NewRequest("POST", "/api/1c/export/payment-registry", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "Неверный формат даты")
}
