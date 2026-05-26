package models

import "time"

// LocalPasswordResetToken — токены сброса пароля для local-auth юзеров.
// Хранится sha256-хеш токена (не plaintext); plaintext шлётся юзеру в email
// один раз. Срок жизни — 1 час (см. handler forgot-password).
//
// При успешном reset помечаем `used_at` чтобы исключить повторное
// использование (single-use), и удаляем все refresh-tokens юзера.
//
// GC: задача внешняя — DELETE WHERE expires_at < now() OR used_at IS NOT NULL.
// (Сейчас не реализован, токены живут без cleanup. Объёмы малы → не блокер.)
type LocalPasswordResetToken struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	UserID    uint       `json:"user_id" gorm:"not null;index"`
	TokenHash string     `json:"-" gorm:"size:64;not null;uniqueIndex"` // sha256 hex
	ExpiresAt time.Time  `json:"expires_at" gorm:"not null;index"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// TableName — таблица в public-схеме.
func (LocalPasswordResetToken) TableName() string { return "local_password_reset_tokens" }
