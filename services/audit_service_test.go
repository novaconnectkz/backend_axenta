package services

import (
	"backend_axenta/database"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAuditServiceTestDB создает тестовую базу данных для audit service
func setupAuditServiceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&AuditLog{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupAuditService создает audit service для тестов
func setupAuditService(_ *testing.T, db *gorm.DB) *AuditService {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	return NewAuditService(db, logger)
}

// TestNewAuditService тестирует создание нового сервиса
func TestNewAuditService(t *testing.T) {
	db := setupAuditServiceTestDB(t)
	service := setupAuditService(t, db)

	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
	assert.NotNil(t, service.logger)
}

// TestAuditService_Log тестирует запись аудит-лога
func TestAuditService_Log(t *testing.T) {
	db := setupAuditServiceTestDB(t)
	service := setupAuditService(t, db)

	userID := uint(1)
	ctx := AuditContext{
		TenantID:  123,
		UserID:    &userID,
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0",
		Action:    ActionUserLogin,
		Resource:  "user",
		Success:   true,
		Details: map[string]interface{}{
			"username": "testuser",
		},
	}

	err := service.Log(ctx)
	require.NoError(t, err)

	// Проверяем, что лог создан
	var auditLog AuditLog
	err = db.Where("tenant_id = ? AND action = ?", 123, "user.login").First(&auditLog).Error
	require.NoError(t, err)
	assert.Equal(t, uint(123), auditLog.TenantID)
	assert.Equal(t, userID, *auditLog.UserID)
	assert.Equal(t, "user.login", auditLog.Action)
	assert.True(t, auditLog.Success)
}

// TestAuditService_LogSuccess тестирует запись успешного действия
func TestAuditService_LogSuccess(t *testing.T) {
	db := setupAuditServiceTestDB(t)
	service := setupAuditService(t, db)

	userID := uint(1)
	ctx := AuditContext{
		TenantID:  123,
		UserID:    &userID,
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0",
		Action:    ActionUserCreate,
		Resource:  "user",
	}

	err := service.LogSuccess(ctx)
	require.NoError(t, err)

	// Проверяем, что лог создан с success = true
	var auditLog AuditLog
	err = db.Where("tenant_id = ? AND action = ?", 123, "user.create").First(&auditLog).Error
	require.NoError(t, err)
	assert.True(t, auditLog.Success)
}

// TestAuditService_LogFailure тестирует запись неуспешного действия
func TestAuditService_LogFailure(t *testing.T) {
	db := setupAuditServiceTestDB(t)
	service := setupAuditService(t, db)

	userID := uint(1)
	ctx := AuditContext{
		TenantID:  123,
		UserID:    &userID,
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0",
		Action:    ActionUserLogin,
		Resource:  "user",
	}

	testError := assert.AnError
	err := service.LogFailure(ctx, testError)
	require.NoError(t, err)

	// Проверяем, что лог создан с success = false
	var auditLog AuditLog
	err = db.Where("tenant_id = ? AND action = ?", 123, "user.login").First(&auditLog).Error
	require.NoError(t, err)
	assert.False(t, auditLog.Success)
	assert.NotEmpty(t, auditLog.ErrorMsg)
}

// TestAuditService_GetAuditLogs_NoLogs тестирует GetAuditLogs без логов
func TestAuditService_GetAuditLogs_NoLogs(t *testing.T) {
	db := setupAuditServiceTestDB(t)
	service := setupAuditService(t, db)

	filters := AuditFilters{}
	logs, err := service.GetAuditLogs(123, filters)
	require.NoError(t, err)
	assert.Empty(t, logs)
}

// TestAuditService_GetAuditLogs_WithLogs тестирует GetAuditLogs с логами
func TestAuditService_GetAuditLogs_WithLogs(t *testing.T) {
	db := setupAuditServiceTestDB(t)
	service := setupAuditService(t, db)

	// Создаем тестовые логи
	userID1 := uint(1)
	userID2 := uint(2)
	service.Log(AuditContext{
		TenantID: 123,
		UserID:   &userID1,
		Action:   ActionUserLogin,
		Resource: "user",
		Success:  true,
	})
	service.Log(AuditContext{
		TenantID: 123,
		UserID:   &userID2,
		Action:   ActionUserCreate,
		Resource: "user",
		Success:  true,
	})

	filters := AuditFilters{}
	logs, err := service.GetAuditLogs(123, filters)
	require.NoError(t, err)
	assert.Len(t, logs, 2)
}

// TestAuditService_GetAuditLogs_WithFilters тестирует GetAuditLogs с фильтрами
func TestAuditService_GetAuditLogs_WithFilters(t *testing.T) {
	db := setupAuditServiceTestDB(t)
	service := setupAuditService(t, db)

	// Создаем тестовые логи
	userID1 := uint(1)
	userID2 := uint(2)
	service.Log(AuditContext{
		TenantID: 123,
		UserID:   &userID1,
		Action:   ActionUserLogin,
		Resource: "user",
		Success:  true,
	})
	service.Log(AuditContext{
		TenantID: 123,
		UserID:   &userID2,
		Action:   ActionUserCreate,
		Resource: "user",
		Success:  true,
	})

	// Фильтруем по user_id
	filters := AuditFilters{
		UserID: &userID1,
	}
	logs, err := service.GetAuditLogs(123, filters)
	require.NoError(t, err)
	assert.Len(t, logs, 1)
	assert.Equal(t, userID1, *logs[0].UserID)
}

// TestAuditService_GetAuditStats тестирует GetAuditStats
func TestAuditService_GetAuditStats(t *testing.T) {
	db := setupAuditServiceTestDB(t)
	service := setupAuditService(t, db)

	// Создаем тестовые логи
	userID := uint(1)
	service.Log(AuditContext{
		TenantID: 123,
		UserID:   &userID,
		Action:   ActionUserLogin,
		Resource: "user",
		Success:  true,
	})

	stats, err := service.GetAuditStats(123, "7d")
	require.NoError(t, err)
	assert.NotNil(t, stats)
}

// TestAuditService_CleanupOldLogs тестирует CleanupOldLogs
func TestAuditService_CleanupOldLogs(t *testing.T) {
	db := setupAuditServiceTestDB(t)
	service := setupAuditService(t, db)

	// Создаем старый лог (вручную, так как CreatedAt устанавливается автоматически)
	oldLog := AuditLog{
		TenantID:  123,
		Action:    "user.login",
		Resource:  "user",
		Success:   true,
		CreatedAt: time.Now().AddDate(0, 0, -8), // 8 дней назад
	}
	db.Create(&oldLog)

	// Создаем новый лог
	userID := uint(1)
	service.Log(AuditContext{
		TenantID: 123,
		UserID:   &userID,
		Action:   ActionUserCreate,
		Resource: "user",
		Success:  true,
	})

	// Очищаем логи старше 7 дней
	err := service.CleanupOldLogs(123, 7)
	require.NoError(t, err)

	// Проверяем, что старый лог удален
	var deletedLog AuditLog
	err = db.First(&deletedLog, oldLog.ID).Error
	assert.Error(t, err) // Должна быть ошибка, так как лог удален
}

// TestAuditService_ExportAuditLogs тестирует ExportAuditLogs
func TestAuditService_ExportAuditLogs(t *testing.T) {
	db := setupAuditServiceTestDB(t)
	service := setupAuditService(t, db)

	// Создаем тестовые логи
	userID := uint(1)
	service.Log(AuditContext{
		TenantID: 123,
		UserID:   &userID,
		Action:   ActionUserLogin,
		Resource: "user",
		Success:  true,
	})

	filters := AuditFilters{}
	data, err := service.ExportAuditLogs(123, filters)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

// TestAuditService_GetSecurityAlerts тестирует GetSecurityAlerts
func TestAuditService_GetSecurityAlerts(t *testing.T) {
	db := setupAuditServiceTestDB(t)
	service := setupAuditService(t, db)

	alerts, err := service.GetSecurityAlerts(123, 24)
	require.NoError(t, err)
	assert.NotNil(t, alerts)
}
