package models

import (
	"math/rand"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Company представляет компанию (tenant) в мультитенантной системе
type Company struct {
	ID        uuid.UUID      `json:"id" gorm:"type:uuid;default:gen_random_uuid();primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля компании
	Name           string `json:"name" gorm:"not null;type:varchar(100)"`
	DatabaseSchema string `json:"database_schema" gorm:"uniqueIndex;not null;type:varchar(100)"` // Имя схемы БД
	Domain         string `json:"domain" gorm:"uniqueIndex;type:varchar(100)"`                   // Поддомен или домен

	// Интеграция с Axenta.cloud
	AxetnaLogin    string `json:"-" gorm:"not null;type:varchar(100)"` // Логин для axetna.cloud (скрыт в JSON)
	AxetnaPassword string `json:"-" gorm:"not null;type:text"`         // Зашифрованный пароль (скрыт в JSON)

	// Интеграция с Битрикс24
	Bitrix24WebhookURL   string `json:"-" gorm:"type:varchar(500)"` // URL вебхука Битрикс24 (скрыт в JSON)
	Bitrix24ClientID     string `json:"-" gorm:"type:varchar(100)"` // ID приложения Битрикс24 (скрыт в JSON)
	Bitrix24ClientSecret string `json:"-" gorm:"type:varchar(200)"` // Секрет приложения Битрикс24 (скрыт в JSON)

	// Контактная информация
	ContactEmail  string `json:"contact_email" gorm:"type:varchar(100)"`
	ContactPhone  string `json:"contact_phone" gorm:"type:varchar(20)"`
	ContactPerson string `json:"contact_person" gorm:"type:varchar(100)"`

	// Адрес
	Address string `json:"address" gorm:"type:text"`
	City    string `json:"city" gorm:"type:varchar(100)"`
	Country string `json:"country" gorm:"default:'Russia';type:varchar(100)"`

	// Настройки и статус
	IsActive     bool `json:"is_active" gorm:"default:true"`
	MaxUsers     int  `json:"max_users" gorm:"default:10"`       // Лимит пользователей
	MaxObjects   int  `json:"max_objects" gorm:"default:100"`    // Лимит объектов
	StorageQuota int  `json:"storage_quota" gorm:"default:1024"` // Квота в МБ

	// Настройки локализации
	Language string `json:"language" gorm:"default:'ru';type:varchar(5)"`
	Timezone string `json:"timezone" gorm:"default:'Europe/Moscow';type:varchar(50)"`
	Currency string `json:"currency" gorm:"default:'RUB';type:varchar(3)"`

	// Подписка и биллинг
	SubscriptionID *uuid.UUID `json:"subscription_id"`
	// Subscription   *Subscription `json:"subscription,omitempty" gorm:"foreignKey:SubscriptionID"`
}

// TableName задает имя таблицы для модели Company
func (Company) TableName() string {
	return "companies"
}

// BeforeCreate вызывается перед созданием записи
func (c *Company) BeforeCreate(tx *gorm.DB) error {
	// Генерируем имя схемы БД если не указано
	if c.DatabaseSchema == "" {
		c.DatabaseSchema = "tenant_default"
	}
	return nil
}

// GetSchemaName возвращает имя схемы БД для компании
func (c *Company) GetSchemaName() string {
	return c.DatabaseSchema
}

// IsValidForTenant проверяет, может ли компания использовать мультитенантность
func (c *Company) IsValidForTenant() bool {
	return c.IsActive && c.DatabaseSchema != "" && c.AxetnaLogin != ""
}

// Вспомогательная функция для генерации случайной строки
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := range b {
		b[i] = charset[r.Intn(len(charset))]
	}
	return string(b)
}
