package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupJWTServiceTestDB создает тестовую базу данных для JWT сервиса
func setupJWTServiceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.LocalUser{},
		&models.RefreshToken{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupJWTService создает JWT сервис для тестов
func setupJWTService(_ *testing.T, db *gorm.DB) *JWTService {
	// Сохраняем оригинальное значение
	originalSecret := os.Getenv("JWT_SECRET")
	defer os.Setenv("JWT_SECRET", originalSecret)

	// Устанавливаем тестовый секрет
	os.Setenv("JWT_SECRET", "test-secret-key-for-testing-only")

	return NewJWTService(db)
}

// TestNewJWTService тестирует создание нового JWT сервиса
func TestNewJWTService(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)

	assert.NotNil(t, service)
	assert.NotNil(t, service.secretKey)
	assert.NotEmpty(t, service.accessTokenTTL)
	assert.NotEmpty(t, service.refreshTokenTTL)
}

// TestJWTService_GenerateAccessToken тестирует генерацию access токена
func TestJWTService_GenerateAccessToken(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)

	// Создаем тестового пользователя
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	token, err := service.GenerateAccessToken(user)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

// TestJWTService_ValidateAccessToken_ValidToken тестирует валидацию валидного токена
func TestJWTService_ValidateAccessToken_ValidToken(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)

	// Создаем тестового пользователя
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	// Генерируем токен
	token, err := service.GenerateAccessToken(user)
	require.NoError(t, err)

	// Валидируем токен
	claims, err := service.ValidateAccessToken(token)
	require.NoError(t, err)
	assert.NotNil(t, claims)
	assert.Equal(t, uint(1), claims.UserID)
	assert.Equal(t, "123", claims.CompanyID)
	assert.Equal(t, "admin", claims.Role)
	assert.Equal(t, "testuser", claims.Username)
}

// TestJWTService_ValidateAccessToken_InvalidToken тестирует валидацию неверного токена
func TestJWTService_ValidateAccessToken_InvalidToken(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)

	claims, err := service.ValidateAccessToken("invalid-token")
	assert.Error(t, err)
	assert.Nil(t, claims)
}

// TestJWTService_ValidateAccessToken_ExpiredToken тестирует валидацию истекшего токена
func TestJWTService_ValidateAccessToken_ExpiredToken(t *testing.T) {
	db := setupJWTServiceTestDB(t)

	// Создаем сервис с очень коротким TTL
	originalSecret := os.Getenv("JWT_SECRET")
	defer os.Setenv("JWT_SECRET", originalSecret)
	os.Setenv("JWT_SECRET", "test-secret-key-for-testing-only")
	os.Setenv("JWT_ACCESS_TTL", "0") // 0 часов = истекший токен

	service := NewJWTService(db)

	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	// Генерируем токен (он будет истекшим)
	token, err := service.GenerateAccessToken(user)
	require.NoError(t, err)

	// Ждем немного, чтобы токен точно истек
	time.Sleep(100 * time.Millisecond)

	// Валидируем токен
	claims, err := service.ValidateAccessToken(token)
	assert.Error(t, err)
	assert.Nil(t, claims)
}

// TestJWTService_GenerateRefreshToken тестирует генерацию refresh токена
func TestJWTService_GenerateRefreshToken(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)

	// Создаем тестового пользователя
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	token, err := service.GenerateRefreshToken(user)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// Проверяем, что токен сохранен в БД
	var refreshToken models.RefreshToken
	err = db.Where("user_id = ? AND token = ?", user.ID, token).First(&refreshToken).Error
	require.NoError(t, err)
	assert.Equal(t, user.ID, refreshToken.UserID)
	assert.Equal(t, token, refreshToken.Token)
}

// TestJWTService_ValidateRefreshToken_ValidToken тестирует валидацию валидного refresh токена
func TestJWTService_ValidateRefreshToken_ValidToken(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)

	// Создаем тестового пользователя
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	// Генерируем refresh токен
	token, err := service.GenerateRefreshToken(user)
	require.NoError(t, err)

	// Валидируем токен
	refreshToken, err := service.ValidateRefreshToken(token)
	require.NoError(t, err)
	assert.NotNil(t, refreshToken)
	assert.Equal(t, user.ID, refreshToken.UserID)
	assert.Equal(t, token, refreshToken.Token)
}

// TestJWTService_ValidateRefreshToken_InvalidToken тестирует валидацию неверного refresh токена
func TestJWTService_ValidateRefreshToken_InvalidToken(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)

	refreshToken, err := service.ValidateRefreshToken("invalid-token")
	assert.Error(t, err)
	assert.Nil(t, refreshToken)
}

// TestJWTService_ValidateRefreshToken_ExpiredToken тестирует валидацию истекшего refresh токена
func TestJWTService_ValidateRefreshToken_ExpiredToken(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)

	// Создаем тестового пользователя
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	// Создаем истекший refresh токен вручную
	expiredToken := &models.RefreshToken{
		UserID:    user.ID,
		Token:     "expired-token",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Истек час назад
	}
	db.Create(expiredToken)

	// Валидируем токен
	refreshToken, err := service.ValidateRefreshToken("expired-token")
	assert.Error(t, err)
	assert.Nil(t, refreshToken)
}

// TestJWTService_GenerateTokenPair тестирует генерацию пары токенов
func TestJWTService_GenerateTokenPair(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)

	// Создаем тестового пользователя
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	accessToken, refreshToken, err := service.GenerateTokenPair(user)
	require.NoError(t, err)
	assert.NotEmpty(t, accessToken)
	assert.NotEmpty(t, refreshToken)

	// Проверяем, что access токен валиден
	claims, err := service.ValidateAccessToken(accessToken)
	require.NoError(t, err)
	assert.NotNil(t, claims)

	// Проверяем, что refresh токен валиден
	refreshTokenModel, err := service.ValidateRefreshToken(refreshToken)
	require.NoError(t, err)
	assert.NotNil(t, refreshTokenModel)
}

// TestJWTService_RevokeRefreshToken тестирует отзыв refresh токена
func TestJWTService_RevokeRefreshToken(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)

	// Создаем тестового пользователя
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	// Генерируем refresh токен
	token, err := service.GenerateRefreshToken(user)
	require.NoError(t, err)

	// Отзываем токен
	err = service.RevokeRefreshToken(token)
	require.NoError(t, err)

	// Проверяем, что токен больше не валиден
	refreshToken, err := service.ValidateRefreshToken(token)
	assert.Error(t, err)
	assert.Nil(t, refreshToken)
}

// TestJWTService_RevokeAllUserTokens тестирует отзыв всех токенов пользователя
func TestJWTService_RevokeAllUserTokens(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)

	// Создаем тестового пользователя
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	// Генерируем несколько refresh токенов
	token1, err := service.GenerateRefreshToken(user)
	require.NoError(t, err)
	token2, err := service.GenerateRefreshToken(user)
	require.NoError(t, err)

	// Отзываем все токены пользователя
	err = service.RevokeAllUserTokens(user.ID)
	require.NoError(t, err)

	// Проверяем, что токены больше не валидны
	refreshToken1, err := service.ValidateRefreshToken(token1)
	assert.Error(t, err)
	assert.Nil(t, refreshToken1)

	refreshToken2, err := service.ValidateRefreshToken(token2)
	assert.Error(t, err)
	assert.Nil(t, refreshToken2)
}
