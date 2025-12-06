package api

import (
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

// setupEmailTestDB создает тестовую базу данных для Email тестов
func setupEmailTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.NotificationSettings{},
		&models.Company{},
	)
	require.NoError(t, err)

	return db
}

// setupEmailTestRouter создает тестовый роутер с middleware для установки контекста
func setupEmailTestRouter(t *testing.T, db *gorm.DB, companyID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки user и company_id в контекст
	router.Use(func(c *gin.Context) {
		c.Set("user", map[string]interface{}{
			"id":        1,
			"username":  "testuser",
			"accountId": companyID,
		})
		// Устанавливаем company_id для middleware.GetCompanyID
		c.Set("company_id", companyID)
		c.Next()
	})

	return router
}

// TestSetupEmailIntegration_Unauthorized тестирует SetupEmailIntegration без авторизации
func TestSetupEmailIntegration_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.POST("/email/setup", SetupEmailIntegration)

	reqBody := map[string]interface{}{
		"smtp_host":       "smtp.example.com",
		"smtp_port":       587,
		"smtp_username":   "user@example.com",
		"smtp_password":   "password123",
		"smtp_from_email": "noreply@example.com",
		"smtp_from_name":  "Test",
		"smtp_use_tls":    true,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/email/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestSetupEmailIntegration_ValidationError тестирует SetupEmailIntegration с ошибкой валидации
func TestSetupEmailIntegration_ValidationError(t *testing.T) {
	db := setupEmailTestDB(t)
	router := setupEmailTestRouter(t, db, 1)

	router.POST("/email/setup", SetupEmailIntegration)

	// Тест с неполными данными
	reqBody := map[string]interface{}{
		"smtp_host": "smtp.example.com",
		// Отсутствуют обязательные поля
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/email/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestSetupEmailIntegration_Success тестирует успешную настройку Email интеграции
func TestSetupEmailIntegration_Success(t *testing.T) {
	db := setupEmailTestDB(t)
	router := setupEmailTestRouter(t, db, 1)

	// Создаем тестовую компанию
	company := models.Company{
		ID:   1,
		Name: "Test Company",
	}
	db.Create(&company)

	router.POST("/email/setup", SetupEmailIntegration)

	reqBody := map[string]interface{}{
		"smtp_host":       "smtp.example.com",
		"smtp_port":       587,
		"smtp_username":   "user@example.com",
		"smtp_password":   "password123",
		"smtp_from_email": "noreply@example.com",
		"smtp_from_name":  "Test Company",
		"smtp_use_tls":    true,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/email/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	// Проверяем, что настройки сохранены
	var settings models.NotificationSettings
	err = db.Where("company_id = ?", 1).First(&settings).Error
	require.NoError(t, err)
	assert.Equal(t, "smtp.example.com", settings.SMTPHost)
	assert.Equal(t, 587, settings.SMTPPort)
	assert.Equal(t, "user@example.com", settings.SMTPUsername)
	assert.True(t, settings.EmailEnabled)
}

// TestSetupEmailIntegration_MaskedPassword тестирует SetupEmailIntegration с замаскированным паролем
func TestSetupEmailIntegration_MaskedPassword(t *testing.T) {
	db := setupEmailTestDB(t)
	router := setupEmailTestRouter(t, db, 1)

	// Создаем тестовую компанию
	company := models.Company{
		ID:   1,
		Name: "Test Company",
	}
	db.Create(&company)

	// Создаем существующие настройки с паролем
	existingSettings := models.NotificationSettings{
		CompanyID:    1,
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPUsername: "user@example.com",
		SMTPPassword: "original_password",
		EmailEnabled: true,
	}
	db.Create(&existingSettings)

	router.POST("/email/setup", SetupEmailIntegration)

	// Отправляем запрос с замаскированным паролем
	reqBody := map[string]interface{}{
		"smtp_host":       "smtp.example.com",
		"smtp_port":       587,
		"smtp_username":   "user@example.com",
		"smtp_password":   "*********************", // Замаскированный пароль
		"smtp_from_email": "noreply@example.com",
		"smtp_from_name":  "Test Company",
		"smtp_use_tls":    true,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/email/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем, что пароль не изменился
	var settings models.NotificationSettings
	err := db.Where("company_id = ?", 1).First(&settings).Error
	require.NoError(t, err)
	assert.Equal(t, "original_password", settings.SMTPPassword)
}

// TestGetEmailConfig_Unauthorized тестирует GetEmailConfig без авторизации
func TestGetEmailConfig_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/email/config", GetEmailConfig)

	req, _ := http.NewRequest("GET", "/email/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestGetEmailConfig_NotFound тестирует GetEmailConfig когда настройки не найдены
func TestGetEmailConfig_NotFound(t *testing.T) {
	db := setupEmailTestDB(t)
	router := setupEmailTestRouter(t, db, 1)

	router.GET("/email/config", GetEmailConfig)

	req, _ := http.NewRequest("GET", "/email/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
	assert.Nil(t, response["config"])
}

// TestGetEmailConfig_Success тестирует успешное получение конфигурации Email
func TestGetEmailConfig_Success(t *testing.T) {
	db := setupEmailTestDB(t)
	router := setupEmailTestRouter(t, db, 1)

	// Создаем настройки
	settings := models.NotificationSettings{
		CompanyID:     1,
		SMTPHost:      "smtp.example.com",
		SMTPPort:      587,
		SMTPUsername:  "user@example.com",
		SMTPPassword:  "password123",
		SMTPFromEmail: "noreply@example.com",
		SMTPFromName:  "Test Company",
		SMTPUseTLS:    true,
		EmailEnabled:  true,
	}
	db.Create(&settings)

	router.GET("/email/config", GetEmailConfig)

	req, _ := http.NewRequest("GET", "/email/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	config, ok := response["config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "smtp.example.com", config["smtp_host"])
	assert.Equal(t, "*********************", config["smtp_password"]) // Пароль должен быть замаскирован
}

// TestGetEmailConfig_ShowPassword тестирует GetEmailConfig с параметром show_password
func TestGetEmailConfig_ShowPassword(t *testing.T) {
	db := setupEmailTestDB(t)
	router := setupEmailTestRouter(t, db, 1)

	// Создаем настройки
	settings := models.NotificationSettings{
		CompanyID:     1,
		SMTPHost:      "smtp.example.com",
		SMTPPort:      587,
		SMTPUsername:  "user@example.com",
		SMTPPassword:  "password123",
		SMTPFromEmail: "noreply@example.com",
		SMTPFromName:  "Test Company",
		SMTPUseTLS:    true,
		EmailEnabled:  true,
	}
	db.Create(&settings)

	router.GET("/email/config", GetEmailConfig)

	req, _ := http.NewRequest("GET", "/email/config?show_password=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	config, ok := response["config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "password123", config["smtp_password"]) // Пароль должен быть виден
}

// TestTestEmailConnection_Unauthorized тестирует TestEmailConnection без авторизации
func TestTestEmailConnection_Unauthorized(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.POST("/email/test-connection", TestEmailConnection)

	req, _ := http.NewRequest("POST", "/email/test-connection", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestTestEmailConnection_NotFound тестирует TestEmailConnection когда настройки не найдены
func TestTestEmailConnection_NotFound(t *testing.T) {
	db := setupEmailTestDB(t)
	router := setupEmailTestRouter(t, db, 1)

	router.POST("/email/test-connection", TestEmailConnection)

	req, _ := http.NewRequest("POST", "/email/test-connection", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["error"], "не найдены")
}

// TestTestEmailConnection_IncompleteSettings тестирует TestEmailConnection с неполными настройками
func TestTestEmailConnection_IncompleteSettings(t *testing.T) {
	db := setupEmailTestDB(t)
	router := setupEmailTestRouter(t, db, 1)

	// Создаем настройки с пустым хостом
	settings := models.NotificationSettings{
		CompanyID:    1,
		SMTPHost:     "",
		SMTPPort:     587,
		SMTPUsername: "",
		EmailEnabled: true,
	}
	db.Create(&settings)

	router.POST("/email/test-connection", TestEmailConnection)

	req, _ := http.NewRequest("POST", "/email/test-connection", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["error"], "не заполнены")
}

// TestUpdateEmailIntegration тестирует UpdateEmailIntegration (должен использовать ту же логику, что и SetupEmailIntegration)
func TestUpdateEmailIntegration(t *testing.T) {
	db := setupEmailTestDB(t)
	router := setupEmailTestRouter(t, db, 1)

	// Создаем тестовую компанию
	company := models.Company{
		ID:   1,
		Name: "Test Company",
	}
	db.Create(&company)

	router.PUT("/email/setup", UpdateEmailIntegration)

	reqBody := map[string]interface{}{
		"smtp_host":       "smtp.updated.com",
		"smtp_port":       465,
		"smtp_username":   "updated@example.com",
		"smtp_password":   "new_password",
		"smtp_from_email": "updated@example.com",
		"smtp_from_name":  "Updated Company",
		"smtp_use_tls":    true,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/email/setup", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	// Проверяем, что настройки обновлены
	var settings models.NotificationSettings
	err = db.Where("company_id = ?", 1).First(&settings).Error
	require.NoError(t, err)
	assert.Equal(t, "smtp.updated.com", settings.SMTPHost)
	assert.Equal(t, 465, settings.SMTPPort)
	assert.Equal(t, "updated@example.com", settings.SMTPUsername)
}
