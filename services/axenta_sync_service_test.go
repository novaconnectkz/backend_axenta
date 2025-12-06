package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAxentaSyncServiceTestDB создает тестовую базу данных
func setupAxentaSyncServiceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Company{},
		&models.SnapshotSettings{},
		&models.UserToken{},
	)
	require.NoError(t, err)

	database.DB = db
	return db
}

// TestNewAxentaSyncService тестирует создание нового AxentaSyncService
func TestNewAxentaSyncService(t *testing.T) {
	db := setupAxentaSyncServiceTestDB(t)
	service := NewAxentaSyncService(db)

	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
	assert.NotNil(t, service.httpClient)
}

// TestAxentaSyncService_SyncAllAdmins_NoCompanies тестирует SyncAllAdmins без компаний
func TestAxentaSyncService_SyncAllAdmins_NoCompanies(t *testing.T) {
	db := setupAxentaSyncServiceTestDB(t)
	service := NewAxentaSyncService(db)

	// Вызываем синхронизацию без компаний
	// Функция не возвращает ошибку, просто логирует
	service.SyncAllAdmins()
	// Не должно паниковать
}

// TestAxentaSyncService_SyncAllAdmins_WithCompanies тестирует SyncAllAdmins с компаниями
func TestAxentaSyncService_SyncAllAdmins_WithCompanies(t *testing.T) {
	db := setupAxentaSyncServiceTestDB(t)
	service := NewAxentaSyncService(db)

	// Создаем тестовую компанию
	company := models.Company{
		ID:             1,
		Name:           "Test Company",
		IsActive:       true,
		DatabaseSchema: "test_schema",
	}
	db.Table("public.companies").Create(&company)

	// Вызываем синхронизацию
	// Функция может вернуть ошибку из-за отсутствия токена, но не должна паниковать
	service.SyncAllAdmins()
	// Не должно паниковать
}
