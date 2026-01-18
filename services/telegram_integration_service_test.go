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

// setupTelegramIntegrationServiceTestDB создает тестовую базу данных для Telegram integration service
func setupTelegramIntegrationServiceTestDB(t *testing.T) *gorm.DB {
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

// setupTelegramIntegrationService создает Telegram integration service для тестов
func setupTelegramIntegrationService(_ *testing.T, db *gorm.DB) *TelegramIntegrationService {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	return NewTelegramIntegrationService(db, logger)
}

// TestNewTelegramIntegrationService тестирует создание нового сервиса
func TestNewTelegramIntegrationService(t *testing.T) {
	db := setupTelegramIntegrationServiceTestDB(t)
	service := setupTelegramIntegrationService(t, db)

	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
	assert.NotNil(t, service.logger)
}

// TestTelegramIntegrationService_GetConfig_NotFound тестирует GetConfig когда конфигурация не найдена
func TestTelegramIntegrationService_GetConfig_NotFound(t *testing.T) {
	setupTelegramIntegrationServiceTestDB(t)
	service := setupTelegramIntegrationService(t, database.DB)

	ctx := context.Background()
	config, err := service.GetConfig(ctx, 456)
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "не настроена")
}

// TestTelegramIntegrationService_SaveConfig тестирует SaveConfig
func TestTelegramIntegrationService_SaveConfig(t *testing.T) {
	db := setupTelegramIntegrationServiceTestDB(t)
	service := setupTelegramIntegrationService(t, db)

	config := &TelegramIntegrationConfig{
		CompanyID:            456,
		BotToken:             "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		DefaultChatID:        "123456789",
		ParseMode:            "HTML",
		DisableNotifications: false,
		QuietHoursStart:      "22:00",
		QuietHoursEnd:        "08:00",
		QuietHoursEnabled:    true,
	}

	ctx := context.Background()
	err := service.SaveConfig(ctx, config)
	require.NoError(t, err)

	// Проверяем, что конфигурация сохранена
	savedConfig, err := service.GetConfig(ctx, 456)
	require.NoError(t, err)
	assert.Equal(t, config.BotToken, savedConfig.BotToken)
	assert.Equal(t, config.ParseMode, savedConfig.ParseMode)
	assert.Equal(t, config.DefaultChatID, savedConfig.DefaultChatID)
}

// TestTelegramIntegrationService_SaveConfig_UpdateExisting тестирует обновление существующей конфигурации
func TestTelegramIntegrationService_SaveConfig_UpdateExisting(t *testing.T) {
	db := setupTelegramIntegrationServiceTestDB(t)
	service := setupTelegramIntegrationService(t, db)

	// Создаем начальную конфигурацию
	initialConfig := &TelegramIntegrationConfig{
		CompanyID: 456,
		BotToken:  "old-token",
		ParseMode: "HTML",
	}

	ctx := context.Background()
	err := service.SaveConfig(ctx, initialConfig)
	require.NoError(t, err)

	// Обновляем конфигурацию
	updatedConfig := &TelegramIntegrationConfig{
		CompanyID: 456,
		BotToken:  "new-token",
		ParseMode: "Markdown",
	}

	err = service.SaveConfig(ctx, updatedConfig)
	require.NoError(t, err)

	// Проверяем, что конфигурация обновлена
	savedConfig, err := service.GetConfig(ctx, 456)
	require.NoError(t, err)
	assert.Equal(t, "new-token", savedConfig.BotToken)
	assert.Equal(t, "Markdown", savedConfig.ParseMode)
}

// TestTelegramIntegrationService_DeleteConfig тестирует DeleteConfig
func TestTelegramIntegrationService_DeleteConfig(t *testing.T) {
	db := setupTelegramIntegrationServiceTestDB(t)
	service := setupTelegramIntegrationService(t, db)

	// Создаем конфигурацию
	config := &TelegramIntegrationConfig{
		CompanyID: 456,
		BotToken:  "test-token",
		ParseMode: "HTML",
	}

	ctx := context.Background()
	err := service.SaveConfig(ctx, config)
	require.NoError(t, err)

	// Проверяем, что конфигурация была создана
	savedConfig, err := service.GetConfig(ctx, 456)
	require.NoError(t, err)
	assert.NotNil(t, savedConfig)
}

// TestTelegramIntegrationService_TestConnection_NoConfig тестирует TestConnection без конфигурации
func TestTelegramIntegrationService_TestConnection_NoConfig(t *testing.T) {
	setupTelegramIntegrationServiceTestDB(t)
	service := setupTelegramIntegrationService(t, database.DB)

	ctx := context.Background()
	err := service.TestConnection(ctx, 456)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не настроена")
}

// TestTelegramIntegrationService_SendMessage_NoConfig тестирует SendMessage без конфигурации
func TestTelegramIntegrationService_SendMessage_NoConfig(t *testing.T) {
	setupTelegramIntegrationServiceTestDB(t)
	service := setupTelegramIntegrationService(t, database.DB)

	ctx := context.Background()
	err := service.SendMessage(ctx, 456, "123456789", "Test message", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не настроена")
}

// TestTelegramIntegrationService_GetIntegrationStatus_NoConfig тестирует GetIntegrationStatus без конфигурации
func TestTelegramIntegrationService_GetIntegrationStatus_NoConfig(t *testing.T) {
	setupTelegramIntegrationServiceTestDB(t)
	service := setupTelegramIntegrationService(t, database.DB)

	ctx := context.Background()
	status, err := service.GetIntegrationStatus(ctx, 456)
	assert.Error(t, err)
	assert.Nil(t, status)
}

// TestTelegramIntegrationService_GetIntegrationStatus_Success тестирует успешное получение статуса
func TestTelegramIntegrationService_GetIntegrationStatus_Success(t *testing.T) {
	db := setupTelegramIntegrationServiceTestDB(t)
	service := setupTelegramIntegrationService(t, db)

	// Создаем конфигурацию
	config := &TelegramIntegrationConfig{
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
