package models

import "time"

// OperatorSMTPConfig — глобальный SMTP-конфиг ACRM-платформы (control-plane).
// Singleton (UNIQUE по singleton-полю), хранится в public.
//
// Используется для системных email'ов от ACRM как продукта:
//   - восстановление пароля (forgot-password flow)
//   - приглашение новых пользователей (invite)
//   - системные уведомления оператору
//
// Отличается от per-tenant Email integration (api/email.go), которая
// настраивается каждой компанией для своих нотификаций. Этот — общий.
//
// Password хранится зашифрованным AES-256-GCM (utils.EncryptString с префиксом
// enc:v1:). При чтении дешифруется только для отправки/теста, в API наружу
// не отдаётся.
type OperatorSMTPConfig struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Singleton bool      `json:"-" gorm:"uniqueIndex;not null;default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Host             string `json:"host" gorm:"size:255;not null;default:''"`
	Port             int    `json:"port" gorm:"not null;default:587"`
	Username         string `json:"username" gorm:"size:255;not null;default:''"`
	PasswordEnc      string `json:"-" gorm:"column:password_enc;type:text;not null;default:''"`
	UseTLS           bool   `json:"use_tls" gorm:"not null;default:true"`
	FromEmail        string `json:"from_email" gorm:"size:255;not null;default:''"`
	FromName         string `json:"from_name" gorm:"size:255;not null;default:''"`
	HasPassword      bool   `json:"has_password" gorm:"-"` // computed: PasswordEnc != ""
	UpdatedByOpID    uint   `json:"updated_by_op_id" gorm:"default:0"`
}

// TableName — таблица в public-схеме.
func (OperatorSMTPConfig) TableName() string { return "operator_smtp_config" }
