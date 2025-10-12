package services

import (
	"backend_axenta/models"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// JWTClaims представляет claims для JWT токена
type JWTClaims struct {
	UserID    uint   `json:"user_id"`
	CompanyID string `json:"company_id"`
	Role      string `json:"role"`
	Username  string `json:"username"`
	jwt.RegisteredClaims
}

// JWTService сервис для работы с JWT токенами
type JWTService struct {
	secretKey       []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	db              *gorm.DB
}

// NewJWTService создает новый JWT сервис
func NewJWTService(db *gorm.DB) *JWTService {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "axenta-crm-default-secret-change-in-production"
	}

	// Настройка TTL из переменных окружения
	accessTTL := 24 * time.Hour // По умолчанию 24 часа (увеличено с 1 часа)
	if ttlStr := os.Getenv("JWT_ACCESS_TTL"); ttlStr != "" {
		if hours, err := strconv.Atoi(ttlStr); err == nil {
			accessTTL = time.Duration(hours) * time.Hour
		}
	}

	refreshTTL := 24 * 7 * time.Hour // По умолчанию 7 дней
	if ttlStr := os.Getenv("JWT_REFRESH_TTL"); ttlStr != "" {
		if hours, err := strconv.Atoi(ttlStr); err == nil {
			refreshTTL = time.Duration(hours) * time.Hour
		}
	}

	return &JWTService{
		secretKey:       []byte(secret),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
		db:              db,
	}
}

// GenerateTokenPair генерирует пару access и refresh токенов
func (j *JWTService) GenerateTokenPair(user *models.LocalUser) (string, string, error) {
	// Генерируем access токен
	accessToken, err := j.GenerateAccessToken(user)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	// Генерируем refresh токен
	refreshToken, err := j.GenerateRefreshToken(user)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

// GenerateAccessToken генерирует access токен
func (j *JWTService) GenerateAccessToken(user *models.LocalUser) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserID:    user.ID,
		CompanyID: user.CompanyID,
		Role:      user.Role,
		Username:  user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.accessTokenTTL)),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "axenta-crm",
			Subject:   fmt.Sprintf("user:%d", user.ID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secretKey)
}

// GenerateRefreshToken генерирует refresh токен и сохраняет в БД
func (j *JWTService) GenerateRefreshToken(user *models.LocalUser) (string, error) {
	// Генерируем случайный токен
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	tokenString := hex.EncodeToString(tokenBytes)

	// Создаем запись в БД
	refreshToken := &models.RefreshToken{
		UserID:    user.ID,
		Token:     tokenString,
		ExpiresAt: time.Now().Add(j.refreshTokenTTL),
	}

	if err := j.db.Create(refreshToken).Error; err != nil {
		return "", fmt.Errorf("failed to save refresh token: %w", err)
	}

	return tokenString, nil
}

// ValidateAccessToken валидирует access токен
func (j *JWTService) ValidateAccessToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// RefreshAccessToken обновляет access токен используя refresh токен
func (j *JWTService) RefreshAccessToken(refreshTokenString string) (string, error) {
	// Находим refresh токен в БД
	var refreshToken models.RefreshToken
	if err := j.db.Preload("User").Where("token = ? AND is_revoked = false", refreshTokenString).First(&refreshToken).Error; err != nil {
		return "", fmt.Errorf("refresh token not found: %w", err)
	}

	// Проверяем валидность
	if !refreshToken.IsValid() {
		return "", fmt.Errorf("refresh token expired or revoked")
	}

	// Генерируем новый access токен
	accessToken, err := j.GenerateAccessToken(refreshToken.User)
	if err != nil {
		return "", fmt.Errorf("failed to generate new access token: %w", err)
	}

	return accessToken, nil
}

// RevokeRefreshToken отзывает refresh токен
func (j *JWTService) RevokeRefreshToken(tokenString string) error {
	var refreshToken models.RefreshToken
	if err := j.db.Where("token = ?", tokenString).First(&refreshToken).Error; err != nil {
		return fmt.Errorf("refresh token not found: %w", err)
	}

	return refreshToken.Revoke(j.db)
}

// RevokeAllUserTokens отзывает все refresh токены пользователя
func (j *JWTService) RevokeAllUserTokens(userID uint) error {
	return j.db.Model(&models.RefreshToken{}).
		Where("user_id = ? AND is_revoked = false", userID).
		Update("is_revoked", true).Error
}

// CleanupExpiredTokens удаляет истекшие refresh токены
func (j *JWTService) CleanupExpiredTokens() error {
	return j.db.Where("expires_at < ? OR is_revoked = true", time.Now()).
		Delete(&models.RefreshToken{}).Error
}

// GetTokenInfo возвращает информацию о токене без валидации подписи
func (j *JWTService) GetTokenInfo(tokenString string) (*JWTClaims, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &JWTClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*JWTClaims); ok {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token claims")
}
