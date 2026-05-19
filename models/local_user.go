package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// BcryptCost — стоимость bcrypt-хеширования паролей.
// Поднята с DefaultCost(10) до 12 (Ф1 hardening, риск B9 — баланс
// стойкость/CPU; ~250-400мс на хэш, проверено бенчмарком до релиза).
const BcryptCost = 12

// LocalUser представляет локального пользователя для альтернативной авторизации
type LocalUser struct {
	ID           uint   `json:"id" gorm:"primaryKey"`
	Username     string `json:"username" gorm:"uniqueIndex;not null;size:64"`
	PasswordHash string `json:"-" gorm:"not null;size:255"` // Скрыт в JSON
	CompanyID    string `json:"company_id" gorm:"not null;size:36;index"`
	Role         string `json:"role" gorm:"not null;size:32;default:'user'"`
	Email        string `json:"email" gorm:"size:255;index"`
	Name         string `json:"name" gorm:"size:255"`
	IsActive     bool   `json:"is_active" gorm:"default:true"`

	// IsSuperadmin — флаг суперадмина (создаётся bootstrap'ом /api/setup).
	// Суперадмин подключает системы мониторинга, настраивает ACRM,
	// приглашает пользователей и раздаёт права.
	IsSuperadmin bool `json:"is_superadmin" gorm:"default:false;not null"`

	// TokenVersion — версия токенов пользователя. Инкрементируется при
	// logout-all / смене пароля; access-JWT несёт этот номер в claim,
	// middleware сверяет (риск B7 — мгновенный отзыв уже выданных JWT,
	// без ожидания истечения 15-минутного access).
	TokenVersion int `json:"-" gorm:"default:1;not null"`

	// Anti-bruteforce (риск B9): счётчик неудачных входов и блокировка.
	FailedAttempts    int        `json:"-" gorm:"default:0;not null"`
	LockedUntil       *time.Time `json:"-"`
	PasswordChangedAt *time.Time `json:"-"`

	LastLogin  *time.Time     `json:"last_login"`
	LoginCount int            `json:"login_count" gorm:"default:0"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `json:"-" gorm:"index"`

	// Связи
	Company *Company `json:"company,omitempty" gorm:"foreignKey:CompanyID;constraint:-;references:ID"`
}

// TableName возвращает имя таблицы
func (LocalUser) TableName() string {
	return "local_users"
}

// SetPassword хеширует и устанавливает пароль (bcrypt cost 12).
// Фиксирует PasswordChangedAt. Инкремент TokenVersion (отзыв старых
// сессий после смены пароля) делается на уровне вызова — при bootstrap'е
// и регистрации это не нужно.
func (u *LocalUser) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	now := time.Now()
	u.PasswordChangedAt = &now
	return nil
}

// CheckPassword проверяет пароль
func (u *LocalUser) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return err == nil
}

// IsLocked сообщает, заблокирован ли аккаунт прямо сейчас (риск B9).
func (u *LocalUser) IsLocked() bool {
	return u.LockedUntil != nil && time.Now().Before(*u.LockedUntil)
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
		"id":            u.ID,
		"username":      u.Username,
		"company_id":    u.CompanyID,
		"role":          u.Role,
		"email":         u.Email,
		"name":          u.Name,
		"is_active":     u.IsActive,
		"is_superadmin": u.IsSuperadmin,
		"last_login":    lastLogin,
		"login_count":   u.LoginCount,
		"created_at":    u.CreatedAt.Format(time.RFC3339),
		"updated_at":    u.UpdatedAt.Format(time.RFC3339),
	}
}

// RefreshToken представляет refresh токен.
//
// Хранится ТОЛЬКО HMAC-SHA256(REFRESH_PEPPER, token) в TokenHash —
// сырой токен в БД не лежит (риск B6: утечка БД ≠ offline-перебор).
// FamilyID связывает цепочку ротаций одной сессии: предъявление уже
// ротированного токена вне grace-окна → отзыв всей семьи (риск B1
// reuse-detection, при этом без ложных массовых разлогинов на гонке —
// см. JWTService.RotateRefreshToken).
type RefreshToken struct {
	ID        uint   `json:"-" gorm:"primaryKey"`
	UserID    uint   `json:"-" gorm:"not null;index"`
	TokenHash string `json:"-" gorm:"column:token_hash;uniqueIndex;not null;size:64"`

	// FamilyID — идентификатор цепочки ротаций (одна логин-сессия).
	FamilyID string `json:"-" gorm:"index;not null;size:64"`
	// RotatedFrom — ID предшественника в цепочке (nil у корня семьи).
	RotatedFrom *uint `json:"-" gorm:"index"`
	// RotatedAt и ReplacedBy заполняются при ротации: предъявление
	// ротированного токена в пределах grace-окна идемпотентно отдаёт
	// того же преемника (ReplacedBy), вне окна → reuse → revoke family.
	RotatedAt  *time.Time `json:"-"`
	ReplacedBy *uint      `json:"-"`

	ExpiresAt time.Time `json:"-" gorm:"not null"`
	CreatedAt time.Time `json:"-"`
	IsRevoked bool      `json:"-" gorm:"default:false;not null"`

	// Связи
	User *LocalUser `json:"-" gorm:"foreignKey:UserID;constraint:-"`
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

// RoleSuperadmin — роль суперадмина (первый пользователь, bootstrap).
const RoleSuperadmin = "superadmin"

// BootstrapState — singleton-маркер первичной инициализации системы.
//
// Уникальный индекс по константному Singleton=true гарантирует не более
// одной строки на всю БД. Используется /api/setup вместе с
// pg_advisory_xact_lock для атомарного создания первого суперадмина
// (риск B2: TOCTOU между COUNT(*)=0 и созданием → несколько суперадминов).
type BootstrapState struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	Singleton        bool      `json:"-" gorm:"uniqueIndex;not null;default:true"`
	SuperadminUserID uint      `json:"superadmin_user_id" gorm:"not null"`
	InitializedAt    time.Time `json:"initialized_at" gorm:"not null"`
}

// TableName возвращает имя таблицы
func (BootstrapState) TableName() string {
	return "bootstrap_state"
}
