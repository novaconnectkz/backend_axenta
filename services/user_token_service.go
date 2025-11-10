package services

import (
	"backend_axenta/models"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type UserTokenService struct {
	db *gorm.DB
}

func NewUserTokenService(db *gorm.DB) *UserTokenService {
	return &UserTokenService{db: db}
}

// SaveUserToken сохраняет токен пользователя
func (s *UserTokenService) SaveUserToken(userID uint, username string, accountID uint, token, userAgent, ipAddress string) error {
	// Сначала деактивируем все существующие токены для этого пользователя
	if err := s.db.Model(&models.UserToken{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Update("is_active", false).Error; err != nil {
		return fmt.Errorf("failed to deactivate existing tokens: %w", err)
	}

	// Создаем новый токен
	userToken := &models.UserToken{
		UserID:     userID,
		Username:   username,
		AccountID:  accountID,
		Token:      token,
		IsActive:   true,
		UserAgent:  userAgent,
		IPAddress:  ipAddress,
		LastUsedAt: &time.Time{},
	}

	// Устанавливаем время истечения токена (например, через 24 часа)
	expiresAt := time.Now().Add(24 * time.Hour)
	userToken.ExpiresAt = &expiresAt

	if err := s.db.Create(userToken).Error; err != nil {
		return fmt.Errorf("failed to save user token: %w", err)
	}

	return nil
}

// GetUserToken получает активный токен пользователя
func (s *UserTokenService) GetUserToken(userID uint) (*models.UserToken, error) {
	var userToken models.UserToken
	err := s.db.Where("user_id = ? AND is_active = ? AND expires_at > ?", userID, true, time.Now()).
		First(&userToken).Error

	if err != nil {
		return nil, fmt.Errorf("no active token found for user %d: %w", userID, err)
	}

	return &userToken, nil
}

// GetUserTokenByUsername получает активный токен пользователя по имени
func (s *UserTokenService) GetUserTokenByUsername(username string) (*models.UserToken, error) {
	var userToken models.UserToken
	err := s.db.Where("username = ? AND is_active = ? AND expires_at > ?", username, true, time.Now()).
		First(&userToken).Error

	if err != nil {
		return nil, fmt.Errorf("no active token found for username %s: %w", username, err)
	}

	return &userToken, nil
}

// UpdateLastUsed обновляет время последнего использования токена
func (s *UserTokenService) UpdateLastUsed(userID uint) error {
	now := time.Now()
	return s.db.Model(&models.UserToken{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Update("last_used_at", now).Error
}

// DeactivateToken деактивирует токен пользователя
func (s *UserTokenService) DeactivateToken(userID uint) error {
	return s.db.Model(&models.UserToken{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Update("is_active", false).Error
}

// CleanupExpiredTokens удаляет истекшие токены
func (s *UserTokenService) CleanupExpiredTokens() error {
	return s.db.Where("expires_at < ?", time.Now()).Delete(&models.UserToken{}).Error
}
