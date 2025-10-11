package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// LocalUser представляет локального пользователя для альтернативной авторизации
type LocalUser struct {
	ID           uint           `json:"id" gorm:"primaryKey"`
	Username     string         `json:"username" gorm:"uniqueIndex;not null;size:64"`
	PasswordHash string         `json:"-" gorm:"not null;size:255"` // Скрыт в JSON
	CompanyID    string         `json:"company_id" gorm:"not null;size:36;index"`
	Role         string         `json:"role" gorm:"not null;size:32;default:'user'"`
	Email        string         `json:"email" gorm:"size:255;index"`
	Name         string         `json:"name" gorm:"size:255"`
	IsActive     bool           `json:"is_active" gorm:"default:true"`
	LastLogin    *time.Time     `json:"last_login"`
	LoginCount   int            `json:"login_count" gorm:"default:0"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`

	// Связи
	Company *Company `json:"company,omitempty" gorm:"foreignKey:CompanyID;references:ID"`
}

// TableName возвращает имя таблицы
func (LocalUser) TableName() string {
	return "local_users"
}

// SetPassword хеширует и устанавливает пароль
func (u *LocalUser) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

// CheckPassword проверяет пароль
func (u *LocalUser) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return err == nil
}

// UpdateLastLogin обновляет время последнего входа
func (u *LocalUser) UpdateLastLogin(db *gorm.DB) error {
	now := time.Now()
	u.LastLogin = &now
	u.LoginCount++
	return db.Model(u).Updates(map[string]interface{}{
		"last_login":  now,
		"login_count": u.LoginCount,
	}).Error
}

// ToPublicUser возвращает публичную версию пользователя без чувствительных данных
func (u *LocalUser) ToPublicUser() map[string]interface{} {
	var lastLogin *string
	if u.LastLogin != nil {
		loginStr := u.LastLogin.Format(time.RFC3339)
		lastLogin = &loginStr
	}

	return map[string]interface{}{
		"id":          u.ID,
		"username":    u.Username,
		"company_id":  u.CompanyID,
		"role":        u.Role,
		"email":       u.Email,
		"name":        u.Name,
		"is_active":   u.IsActive,
		"last_login":  lastLogin,
		"login_count": u.LoginCount,
		"created_at":  u.CreatedAt.Format(time.RFC3339),
		"updated_at":  u.UpdatedAt.Format(time.RFC3339),
	}
}

// RefreshToken представляет refresh токен
type RefreshToken struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"not null;index"`
	Token     string    `json:"token" gorm:"uniqueIndex;not null;size:255"`
	ExpiresAt time.Time `json:"expires_at" gorm:"not null"`
	CreatedAt time.Time `json:"created_at"`
	IsRevoked bool      `json:"is_revoked" gorm:"default:false"`

	// Связи
	User *LocalUser `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// TableName возвращает имя таблицы
func (RefreshToken) TableName() string {
	return "refresh_tokens"
}

// IsValid проверяет валидность токена
func (rt *RefreshToken) IsValid() bool {
	return !rt.IsRevoked && time.Now().Before(rt.ExpiresAt)
}

// Revoke отзывает токен
func (rt *RefreshToken) Revoke(db *gorm.DB) error {
	rt.IsRevoked = true
	return db.Save(rt).Error
}

// Константы для ролей
const (
	RoleAdmin      = "admin"
	RoleManager    = "manager"
	RoleTech       = "tech"
	RoleAccountant = "accountant"
	RoleUser       = "user"
)

// ValidRoles список допустимых ролей
var ValidRoles = []string{
	RoleAdmin,
	RoleManager,
	RoleTech,
	RoleAccountant,
	RoleUser,
}

// IsValidRole проверяет валидность роли
func IsValidRole(role string) bool {
	for _, validRole := range ValidRoles {
		if role == validRole {
			return true
		}
	}
	return false
}
