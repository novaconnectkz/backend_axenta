package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAxentaIntegrationServiceTestDB создает тестовую базу данных для Axenta integration service
func setupAxentaIntegrationServiceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Integration{},
		&models.IntegrationError{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// TestNewAxentaIntegrationService тестирует создание нового сервиса
func TestNewAxentaIntegrationService(t *testing.T) {
	db := setupAxentaIntegrationServiceTestDB(t)

	service := NewAxentaIntegrationService(db)
	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
}

// TestAxentaIntegrationService_GetCredentials_NotFound тестирует GetCredentials когда интеграция не найдена
func TestAxentaIntegrationService_GetCredentials_NotFound(t *testing.T) {
	setupAxentaIntegrationServiceTestDB(t)
	service := NewAxentaIntegrationService(database.DB)

	ctx := context.Background()
	credentials, err := service.GetCredentials(ctx, 456)
	assert.Error(t, err)
	assert.Nil(t, credentials)
	assert.Contains(t, err.Error(), "не настроена")
}

// TestAxentaIntegrationService_GetCredentials_InvalidSettings тестирует GetCredentials с неверными настройками
func TestAxentaIntegrationService_GetCredentials_InvalidSettings(t *testing.T) {
	db := setupAxentaIntegrationServiceTestDB(t)
	service := NewAxentaIntegrationService(db)

	// Создаем интеграцию с неверными настройками
	integration := models.Integration{
		CompanyID:       456,
		IntegrationType: "axenta_cloud",
		Name:            "Axenta Cloud API",
		Settings:        "invalid json",
		IsActive:        true,
	}
	db.Create(&integration)

	ctx := context.Background()
	credentials, err := service.GetCredentials(ctx, 456)
	assert.Error(t, err)
	assert.Nil(t, credentials)
}

// TestAxentaIntegrationService_GetCredentials_Success тестирует успешное получение учетных данных
func TestAxentaIntegrationService_GetCredentials_Success(t *testing.T) {
	db := setupAxentaIntegrationServiceTestDB(t)
	service := NewAxentaIntegrationService(db)

	// Создаем конфигурацию
	config := AxentaIntegrationConfig{
		CompanyID:       456,
		APIURL:          "https://axenta.cloud/api",
		Username:        "testuser",
		Password:        "password123",
		SyncInterval:    15,
		AutoSyncEnabled: true,
		RetryAttempts:   3,
		Timeout:         30,
	}
	configJSON, _ := json.Marshal(config)

	// Создаем интеграцию
	integration := models.Integration{
		CompanyID:       456,
		IntegrationType: "axenta_cloud",
		Name:            "Axenta Cloud API",
		Settings:        string(configJSON),
		IsActive:        true,
	}
	db.Create(&integration)

	ctx := context.Background()
	credentials, err := service.GetCredentials(ctx, 456)
	require.NoError(t, err)
	assert.NotNil(t, credentials)
	assert.Equal(t, "https://axenta.cloud/api", credentials.APIURL)
	assert.Equal(t, "testuser", credentials.Username)
	assert.Equal(t, "password123", credentials.Password)
	assert.Equal(t, 30, credentials.Timeout)
}

// TestAxentaIntegrationService_GetIntegrationErrors_NoErrors тестирует GetIntegrationErrors без ошибок
func TestAxentaIntegrationService_GetIntegrationErrors_NoErrors(t *testing.T) {
	setupAxentaIntegrationServiceTestDB(t)
	service := NewAxentaIntegrationService(database.DB)

	ctx := context.Background()
	errors, err := service.GetIntegrationErrors(ctx, 456)
	require.NoError(t, err)
	assert.Empty(t, errors)
}

// TestAxentaIntegrationService_GetIntegrationErrors_WithErrors тестирует GetIntegrationErrors с ошибками
func TestAxentaIntegrationService_GetIntegrationErrors_WithErrors(t *testing.T) {
	db := setupAxentaIntegrationServiceTestDB(t)
	service := NewAxentaIntegrationService(db)

	// Создаем интеграцию
	integration := models.Integration{
		CompanyID:       456,
		IntegrationType: "axenta_cloud",
		Name:            "Axenta Cloud API",
		IsActive:        true,
	}
	db.Create(&integration)

	// Создаем ошибки интеграции
	error1 := models.IntegrationError{
		TenantID:     456,
		Service:      "axetna_cloud",
		Operation:    "sync",
		ErrorMessage: "Connection timeout",
		Status:       "pending",
	}
	error2 := models.IntegrationError{
		TenantID:     456,
		Service:      "axetna_cloud",
		Operation:    "sync",
		ErrorMessage: "Sync failed",
		Status:       "resolved",
	}
	db.Create(&error1)
	db.Create(&error2)

	ctx := context.Background()
	errors, err := service.GetIntegrationErrors(ctx, 456)
	require.NoError(t, err)
	assert.Len(t, errors, 2)
}

// TestAxentaIntegrationService_ResolveError_NotFound тестирует ResolveError когда ошибка не найдена
func TestAxentaIntegrationService_ResolveError_NotFound(t *testing.T) {
	setupAxentaIntegrationServiceTestDB(t)
	service := NewAxentaIntegrationService(database.DB)

	ctx := context.Background()
	err := service.ResolveError(ctx, 456, "999")
	assert.Error(t, err)
}

// TestAxentaIntegrationService_ResolveError_Success тестирует успешное разрешение ошибки
func TestAxentaIntegrationService_ResolveError_Success(t *testing.T) {
	db := setupAxentaIntegrationServiceTestDB(t)
	service := NewAxentaIntegrationService(db)

	// Создаем интеграцию
	integration := models.Integration{
		CompanyID:       456,
		IntegrationType: "axenta_cloud",
		Name:            "Axenta Cloud API",
		IsActive:        true,
	}
	db.Create(&integration)

	// Создаем ошибку
	integrationError := models.IntegrationError{
		TenantID:     456,
		Service:      "axetna_cloud",
		Operation:    "sync",
		ErrorMessage: "Connection timeout",
		Status:       "pending",
	}
	db.Create(&integrationError)

	ctx := context.Background()
	err := service.ResolveError(ctx, 456, "1")
	require.NoError(t, err)

	// Проверяем, что ошибка разрешена
	var resolvedError models.IntegrationError
	db.First(&resolvedError, integrationError.ID)
	assert.Equal(t, "resolved", resolvedError.Status)
	assert.NotNil(t, resolvedError.ResolvedAt)
}

// TestAxentaIntegrationService_GetIntegrationStatus_NotFound тестирует GetIntegrationStatus когда интеграция не найдена
func TestAxentaIntegrationService_GetIntegrationStatus_NotFound(t *testing.T) {
	setupAxentaIntegrationServiceTestDB(t)
	service := NewAxentaIntegrationService(database.DB)

	ctx := context.Background()
	status, err := service.GetIntegrationStatus(ctx, 456)
	assert.Error(t, err)
	assert.Nil(t, status)
}

// TestAxentaIntegrationService_GetIntegrationStatus_Success тестирует успешное получение статуса
func TestAxentaIntegrationService_GetIntegrationStatus_Success(t *testing.T) {
	db := setupAxentaIntegrationServiceTestDB(t)
	service := NewAxentaIntegrationService(db)

	// Создаем интеграцию
	integration := models.Integration{
		CompanyID:       456,
		IntegrationType: "axenta_cloud",
		Name:            "Axenta Cloud API",
		IsActive:        true,
		LastSyncAt:      timePtr(time.Now().Add(-1 * time.Hour)),
	}
	db.Create(&integration)

	ctx := context.Background()
	status, err := service.GetIntegrationStatus(ctx, 456)
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.True(t, status.IsActive)
}

// TestAxentaIntegrationService_TestConnection_NoIntegration тестирует TestConnection когда интеграция не найдена
func TestAxentaIntegrationService_TestConnection_NoIntegration(t *testing.T) {
	setupAxentaIntegrationServiceTestDB(t)
	service := NewAxentaIntegrationService(database.DB)

	ctx := context.Background()
	err := service.TestConnection(ctx, 456)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не настроена")
}

// TestAxentaIntegrationService_SyncObjects_NoIntegration тестирует SyncObjects когда интеграция не найдена
func TestAxentaIntegrationService_SyncObjects_NoIntegration(t *testing.T) {
	setupAxentaIntegrationServiceTestDB(t)
	service := NewAxentaIntegrationService(database.DB)

	ctx := context.Background()
	err := service.SyncObjects(ctx, 456)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не настроена")
}

// TestAxentaIntegrationService_ScheduleAutoSync_NoIntegration тестирует ScheduleAutoSync когда интеграция не найдена
func TestAxentaIntegrationService_ScheduleAutoSync_NoIntegration(t *testing.T) {
	setupAxentaIntegrationServiceTestDB(t)
	service := NewAxentaIntegrationService(database.DB)

	ctx := context.Background()
	err := service.ScheduleAutoSync(ctx, 456)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не настроена")
}

// timePtr возвращает указатель на time.Time
func timePtr(t time.Time) *time.Time {
	return &t
}
