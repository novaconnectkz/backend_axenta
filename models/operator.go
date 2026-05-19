package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Operator — учётная запись SaaS-ОПЕРАТОРА (владельца ACRM) для
// control-plane монетизации (Фаза 2). ПОЛНОСТЬЮ отделён от tenant-
// пользователей (LocalUser): своя таблица, свой auth-контур, свой
// секрет подписи JWT, своя cookie. Оператор НЕ привязан к company/
// tenant — он управляет пакетами/подписками/фичами, а не данными
// тенантов.
type Operator struct {
	ID           uint   `json:"id" gorm:"primaryKey"`
	Username     string `json:"username" gorm:"uniqueIndex;not null;size:64"`
	PasswordHash string `json:"-" gorm:"not null;size:255"`
	Email        string `json:"email" gorm:"size:255;index"`
	Name         string `json:"name" gorm:"size:255"`
	IsActive     bool   `json:"is_active" gorm:"default:true;not null"`

	// Мгновенный отзыв уже выданных access-JWT (как B7 в Ф1).
	TokenVersion int `json:"-" gorm:"default:1;not null"`

	// Anti-bruteforce.
	FailedAttempts    int        `json:"-" gorm:"default:0;not null"`
	LockedUntil       *time.Time `json:"-"`
	PasswordChangedAt *time.Time `json:"-"`

	LastLogin  *time.Time     `json:"last_login"`
	LoginCount int            `json:"login_count" gorm:"default:0"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Operator) TableName() string { return "operators" }

// SetPassword — bcrypt cost 12 (общая константа BcryptCost), фиксирует
// PasswordChangedAt.
func (o *Operator) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return err
	}
	o.PasswordHash = string(hash)
	now := time.Now()
	o.PasswordChangedAt = &now
	return nil
}

func (o *Operator) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(o.PasswordHash), []byte(password)) == nil
}

func (o *Operator) IsLocked() bool {
	return o.LockedUntil != nil && time.Now().Before(*o.LockedUntil)
}

func (o *Operator) ToPublic() map[string]interface{} {
	var last *string
	if o.LastLogin != nil {
		s := o.LastLogin.Format(time.RFC3339)
		last = &s
	}
	return map[string]interface{}{
		"id":         o.ID,
		"username":   o.Username,
		"email":      o.Email,
		"name":       o.Name,
		"is_active":  o.IsActive,
		"last_login": last,
		"role":       "operator",
	}
}

// OperatorRefreshToken — refresh-токены операторского контура.
// Та же модель безопасности, что Ф1 RefreshToken: в БД только
// HMAC-хэш, family-ротация, reuse-detection. Отдельная таблица —
// никакого пересечения с tenant refresh_tokens.
type OperatorRefreshToken struct {
	ID          uint       `json:"-" gorm:"primaryKey"`
	OperatorID  uint       `json:"-" gorm:"not null;index"`
	TokenHash   string     `json:"-" gorm:"column:token_hash;uniqueIndex;not null;size:64"`
	FamilyID    string     `json:"-" gorm:"index;not null;size:64"`
	RotatedFrom *uint      `json:"-" gorm:"index"`
	RotatedAt   *time.Time `json:"-"`
	ReplacedBy  *uint      `json:"-"`
	ExpiresAt   time.Time  `json:"-" gorm:"not null"`
	CreatedAt   time.Time  `json:"-"`
	IsRevoked   bool       `json:"-" gorm:"default:false;not null"`
}

func (OperatorRefreshToken) TableName() string { return "operator_refresh_tokens" }

// OperatorBootstrapState — singleton первичного создания оператора
// (аналог BootstrapState для tenant). Отдельный маркер: контуры
// инициализируются независимо.
type OperatorBootstrapState struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	Singleton     bool      `json:"-" gorm:"uniqueIndex;not null;default:true"`
	OperatorID    uint      `json:"operator_id" gorm:"not null"`
	InitializedAt time.Time `json:"initialized_at" gorm:"not null"`
}

func (OperatorBootstrapState) TableName() string { return "operator_bootstrap_state" }
