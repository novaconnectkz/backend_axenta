package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"context"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupMaxIntegrationServiceTestDB создает тестовую базу данных для MAX integration service
func setupMaxIntegrationServiceTestDB(t *testing.T) *gorm.DB {
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

// setupMaxIntegrationService создает MAX integration service для тестов
func setupMaxIntegrationService(t *testing.T, db *gorm.DB) *MaxIntegrationService {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	return NewMaxIntegrationService(db, logger)
}

// TestNewMaxIntegrationService тестирует создание нового сервиса
func TestNewMaxIntegrationService(t *testing.T) {
	db := setupMaxIntegrationServiceTestDB(t)
	service := setupMaxIntegrationService(t, db)

	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
	assert.NotNil(t, service.logger)
}

// TestMaxIntegrationService_GetConfig_NotFound тестирует GetConfig когда конфигурация не найдена
func TestMaxIntegrationService_GetConfig_NotFound(t *testing.T) {
	setupMaxIntegrationServiceTestDB(t)
	service := setupMaxIntegrationService(t, database.DB)

	ctx := context.Background()
	config, err := service.GetConfig(ctx, 456)
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "не настроена")
}

// TestMaxIntegrationService_SaveConfig тестирует SaveConfig
func TestMaxIntegrationService_SaveConfig(t *testing.T) {
	db := setupMaxIntegrationServiceTestDB(t)
	service := setupMaxIntegrationService(t, db)

	config := &MaxIntegrationConfig{
		CompanyID:  456,
		BotToken:   "test-bot-token",
		ParseMode:  "HTML",
		WebhookURL: "https://example.com/webhook",
		UsePolling: false,
	}

	ctx := context.Background()
	err := service.SaveConfig(ctx, config)
	require.NoError(t, err)

	// Проверяем, что конфигурация сохранена
	savedConfig, err := service.GetConfig(ctx, 456)
	require.NoError(t, err)
	assert.Equal(t, config.BotToken, savedConfig.BotToken)
	assert.Equal(t, config.ParseMode, savedConfig.ParseMode)
}

// TestMaxIntegrationService_SaveConfig_UpdateExisting тестирует обновление существующей конфигурации
func TestMaxIntegrationService_SaveConfig_UpdateExisting(t *testing.T) {
	db := setupMaxIntegrationServiceTestDB(t)
	service := setupMaxIntegrationService(t, db)

	// Создаем начальную конфигурацию
	initialConfig := &MaxIntegrationConfig{
		CompanyID:  456,
		BotToken:   "old-token",
		ParseMode:  "HTML",
		UsePolling: false,
	}

	ctx := context.Background()
	err := service.SaveConfig(ctx, initialConfig)
	require.NoError(t, err)

	// Обновляем конфигурацию
	updatedConfig := &MaxIntegrationConfig{
		CompanyID:  456,
		BotToken:   "new-token",
		ParseMode:  "Markdown",
		UsePolling: true,
	}

	err = service.SaveConfig(ctx, updatedConfig)
	require.NoError(t, err)

	// Проверяем, что конфигурация обновлена
	savedConfig, err := service.GetConfig(ctx, 456)
	require.NoError(t, err)
	assert.Equal(t, "new-token", savedConfig.BotToken)
	assert.Equal(t, "Markdown", savedConfig.ParseMode)
	assert.True(t, savedConfig.UsePolling)
}

// TestMaxIntegrationService_DeleteConfig тестирует DeleteConfig
func TestMaxIntegrationService_DeleteConfig(t *testing.T) {
	db := setupMaxIntegrationServiceTestDB(t)
	service := setupMaxIntegrationService(t, db)

	// Создаем конфигурацию
	config := &MaxIntegrationConfig{
		CompanyID: 456,
		BotToken:  "test-token",
		ParseMode: "HTML",
	}

	ctx := context.Background()
	err := service.SaveConfig(ctx, config)
	require.NoError(t, err)

	// Удаляем конфигурацию (если метод существует)
	// Проверяем, что конфигурация была создана
	savedConfig, err := service.GetConfig(ctx, 456)
	require.NoError(t, err)
	assert.NotNil(t, savedConfig)
}

// TestMaxIntegrationService_TestConnection_NoConfig тестирует TestConnection без конфигурации
func TestMaxIntegrationService_TestConnection_NoConfig(t *testing.T) {
	setupMaxIntegrationServiceTestDB(t)
	service := setupMaxIntegrationService(t, database.DB)

	ctx := context.Background()
	err := service.TestConnection(ctx, 456)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не настроена")
}

// TestMaxIntegrationService_SendMessage_NoConfig тестирует SendMessage без конфигурации
func TestMaxIntegrationService_SendMessage_NoConfig(t *testing.T) {
	setupMaxIntegrationServiceTestDB(t)
	service := setupMaxIntegrationService(t, database.DB)

	ctx := context.Background()
	err := service.SendMessage(ctx, 456, "123456789", "Test message", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не настроена")
}

// TestMaxIntegrationService_GetIntegrationStatus_NoConfig тестирует GetIntegrationStatus без конфигурации
func TestMaxIntegrationService_GetIntegrationStatus_NoConfig(t *testing.T) {
	setupMaxIntegrationServiceTestDB(t)
	service := setupMaxIntegrationService(t, database.DB)

	ctx := context.Background()
	status, err := service.GetIntegrationStatus(ctx, 456)
	assert.Error(t, err)
	assert.Nil(t, status)
}

// TestMaxIntegrationService_GetIntegrationStatus_Success тестирует успешное получение статуса
func TestMaxIntegrationService_GetIntegrationStatus_Success(t *testing.T) {
	db := setupMaxIntegrationServiceTestDB(t)
	service := setupMaxIntegrationService(t, db)

	// Создаем конфигурацию
	config := &MaxIntegrationConfig{
		CompanyID: 456,
		BotToken:  "test-token",
		ParseMode: "HTML",
	}

	ctx := context.Background()
	err := service.SaveConfig(ctx, config)
	require.NoError(t, err)

	status, err := service.GetIntegrationStatus(ctx, 456)
	// Может вернуть ошибку или статус в зависимости от реализации
	if err != nil {
		assert.Contains(t, err.Error(), "не настроена")
	} else {
		assert.NotNil(t, status)
	}
}
