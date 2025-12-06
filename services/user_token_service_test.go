package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupUserTokenServiceTestDB создает тестовую базу данных для user token service
func setupUserTokenServiceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.UserToken{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// TestNewUserTokenService тестирует создание нового сервиса
func TestNewUserTokenService(t *testing.T) {
	db := setupUserTokenServiceTestDB(t)

	service := NewUserTokenService(db)
	assert.NotNil(t, service)
	assert.NotNil(t, service.db)
}

// TestUserTokenService_SaveUserToken тестирует сохранение токена пользователя
func TestUserTokenService_SaveUserToken(t *testing.T) {
	db := setupUserTokenServiceTestDB(t)
	service := NewUserTokenService(db)

	err := service.SaveUserToken(1, "testuser", 123, "token123", "Mozilla/5.0", "192.168.1.1")
	require.NoError(t, err)

	// Проверяем, что токен сохранен
	var userToken models.UserToken
	err = db.Where("user_id = ? AND token = ?", 1, "token123").First(&userToken).Error
	require.NoError(t, err)
	assert.Equal(t, uint(1), userToken.UserID)
	assert.Equal(t, "testuser", userToken.Username)
	assert.Equal(t, uint(123), userToken.AccountID)
	assert.Equal(t, "token123", userToken.Token)
	assert.True(t, userToken.IsActive)
}

// TestUserTokenService_SaveUserToken_DeactivatesOldTokens тестирует деактивацию старых токенов
func TestUserTokenService_SaveUserToken_DeactivatesOldTokens(t *testing.T) {
	db := setupUserTokenServiceTestDB(t)
	service := NewUserTokenService(db)

	// Создаем старый активный токен
	oldToken := models.UserToken{
		UserID:    1,
		Username:  "testuser",
		AccountID: 123,
		Token:     "old-token",
		IsActive:  true,
	}
	db.Create(&oldToken)

	// Сохраняем новый токен
	err := service.SaveUserToken(1, "testuser", 123, "new-token", "Mozilla/5.0", "192.168.1.1")
	require.NoError(t, err)

	// Проверяем, что старый токен деактивирован
	var deactivatedToken models.UserToken
	db.First(&deactivatedToken, oldToken.ID)
	assert.False(t, deactivatedToken.IsActive)

	// Проверяем, что новый токен активен
	var newToken models.UserToken
	err = db.Where("token = ?", "new-token").First(&newToken).Error
	require.NoError(t, err)
	assert.True(t, newToken.IsActive)
}

// TestUserTokenService_GetUserToken_NotFound тестирует GetUserToken когда токен не найден
func TestUserTokenService_GetUserToken_NotFound(t *testing.T) {
	setupUserTokenServiceTestDB(t)
	service := NewUserTokenService(database.DB)

	token, err := service.GetUserToken(999)
	assert.Error(t, err)
	assert.Nil(t, token)
}

// TestUserTokenService_GetUserToken_Expired тестирует GetUserToken когда токен истек
func TestUserTokenService_GetUserToken_Expired(t *testing.T) {
	db := setupUserTokenServiceTestDB(t)
	service := NewUserTokenService(db)

	// Создаем истекший токен
	expiredTime := time.Now().Add(-1 * time.Hour)
	expiredToken := models.UserToken{
		UserID:    1,
		Username:  "testuser",
		AccountID: 123,
		Token:     "expired-token",
		IsActive:  true,
		ExpiresAt: &expiredTime,
	}
	db.Create(&expiredToken)

	token, err := service.GetUserToken(1)
	assert.Error(t, err)
	assert.Nil(t, token)
}

// TestUserTokenService_GetUserToken_Success тестирует успешное получение токена
func TestUserTokenService_GetUserToken_Success(t *testing.T) {
	db := setupUserTokenServiceTestDB(t)
	service := NewUserTokenService(db)

	// Создаем активный токен
	expiresAt := time.Now().Add(24 * time.Hour)
	userToken := models.UserToken{
		UserID:    1,
		Username:  "testuser",
		AccountID: 123,
		Token:     "valid-token",
		IsActive:  true,
		ExpiresAt: &expiresAt,
	}
	db.Create(&userToken)

	token, err := service.GetUserToken(1)
	require.NoError(t, err)
	assert.NotNil(t, token)
	assert.Equal(t, "valid-token", token.Token)
}

// TestUserTokenService_GetUserTokenByUsername_NotFound тестирует GetUserTokenByUsername когда токен не найден
func TestUserTokenService_GetUserTokenByUsername_NotFound(t *testing.T) {
	setupUserTokenServiceTestDB(t)
	service := NewUserTokenService(database.DB)

	token, err := service.GetUserTokenByUsername("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, token)
}

// TestUserTokenService_GetUserTokenByUsername_Success тестирует успешное получение токена по username
func TestUserTokenService_GetUserTokenByUsername_Success(t *testing.T) {
	db := setupUserTokenServiceTestDB(t)
	service := NewUserTokenService(db)

	// Создаем активный токен
	expiresAt := time.Now().Add(24 * time.Hour)
	userToken := models.UserToken{
		UserID:    1,
		Username:  "testuser",
		AccountID: 123,
		Token:     "valid-token",
		IsActive:  true,
		ExpiresAt: &expiresAt,
	}
	db.Create(&userToken)

	token, err := service.GetUserTokenByUsername("testuser")
	require.NoError(t, err)
	assert.NotNil(t, token)
	assert.Equal(t, "valid-token", token.Token)
}

// TestUserTokenService_UpdateLastUsed тестирует обновление времени последнего использования
func TestUserTokenService_UpdateLastUsed(t *testing.T) {
	db := setupUserTokenServiceTestDB(t)
	service := NewUserTokenService(db)

	// Создаем активный токен
	expiresAt := time.Now().Add(24 * time.Hour)
	userToken := models.UserToken{
		UserID:    1,
		Username:  "testuser",
		AccountID: 123,
		Token:     "valid-token",
		IsActive:  true,
		ExpiresAt: &expiresAt,
	}
	db.Create(&userToken)

	err := service.UpdateLastUsed(1)
	require.NoError(t, err)

	// Проверяем, что время обновлено
	var updatedToken models.UserToken
	db.First(&updatedToken, userToken.ID)
	assert.NotNil(t, updatedToken.LastUsedAt)
}

// TestUserTokenService_DeactivateToken тестирует деактивацию токена
func TestUserTokenService_DeactivateToken(t *testing.T) {
	db := setupUserTokenServiceTestDB(t)
	service := NewUserTokenService(db)

	// Создаем активный токен
	expiresAt := time.Now().Add(24 * time.Hour)
	userToken := models.UserToken{
		UserID:    1,
		Username:  "testuser",
		AccountID: 123,
		Token:     "valid-token",
		IsActive:  true,
		ExpiresAt: &expiresAt,
	}
	db.Create(&userToken)

	err := service.DeactivateToken(1)
	require.NoError(t, err)

	// Проверяем, что токен деактивирован
	var deactivatedToken models.UserToken
	db.First(&deactivatedToken, userToken.ID)
	assert.False(t, deactivatedToken.IsActive)
}

// TestUserTokenService_CleanupExpiredTokens тестирует очистку истекших токенов
func TestUserTokenService_CleanupExpiredTokens(t *testing.T) {
	db := setupUserTokenServiceTestDB(t)
	service := NewUserTokenService(db)

	// Создаем истекший токен
	expiredTime := time.Now().Add(-1 * time.Hour)
	expiredToken := models.UserToken{
		UserID:    1,
		Username:  "testuser",
		AccountID: 123,
		Token:     "expired-token",
		IsActive:  true,
		ExpiresAt: &expiredTime,
	}
	db.Create(&expiredToken)

	// Создаем активный токен
	expiresAt := time.Now().Add(24 * time.Hour)
	activeToken := models.UserToken{
		UserID:    2,
		Username:  "testuser2",
		AccountID: 123,
		Token:     "active-token",
		IsActive:  true,
		ExpiresAt: &expiresAt,
	}
	db.Create(&activeToken)

	err := service.CleanupExpiredTokens()
	require.NoError(t, err)

	// Проверяем, что истекший токен удален
	var deletedToken models.UserToken
	err = db.First(&deletedToken, expiredToken.ID).Error
	assert.Error(t, err) // Должна быть ошибка, так как токен удален

	// Проверяем, что активный токен остался
	var keptToken models.UserToken
	err = db.First(&keptToken, activeToken.ID).Error
	require.NoError(t, err)
	assert.Equal(t, "active-token", keptToken.Token)
}
