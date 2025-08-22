package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NotificationTemplate представляет шаблон уведомления
type NotificationTemplate struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	Name        string `json:"name" gorm:"not null;uniqueIndex"`   // Уникальное имя шаблона
	Type        string `json:"type" gorm:"not null"`               // installation_reminder, billing_alert, etc.
	Channel     string `json:"channel" gorm:"not null"`            // telegram, email, sms
	Subject     string `json:"subject"`                            // Тема сообщения (для email)
	Template    string `json:"template" gorm:"type:text;not null"` // Шаблон с плейсхолдерами
	Description string `json:"description"`                        // Описание шаблона
	IsActive    bool   `json:"is_active" gorm:"default:true"`      // Активен ли шаблон
	Language    string `json:"language" gorm:"default:'ru'"`       // Язык шаблона

	// Настройки отправки
	Priority      string `json:"priority" gorm:"default:'normal'"` // low, normal, high, urgent
	RetryAttempts int    `json:"retry_attempts" gorm:"default:3"`  // Количество попыток отправки
	DelaySeconds  int    `json:"delay_seconds" gorm:"default:0"`   // Задержка перед отправкой

	// Для мультитенантности
	CompanyID uuid.UUID `json:"company_id" gorm:"type:uuid;index"`
}

// NotificationLog представляет лог отправленных уведомлений
type NotificationLog struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	Type         string     `json:"type" gorm:"not null"`              // Тип уведомления
	Channel      string     `json:"channel" gorm:"not null"`           // Канал отправки
	Recipient    string     `json:"recipient" gorm:"not null"`         // Получатель
	Subject      string     `json:"subject"`                           // Тема (для email)
	Message      string     `json:"message" gorm:"type:text;not null"` // Текст сообщения
	Status       string     `json:"status" gorm:"default:'pending'"`   // pending, sent, failed, retry
	ErrorMessage string     `json:"error_message" gorm:"type:text"`    // Сообщение об ошибке
	SentAt       *time.Time `json:"sent_at"`                           // Время отправки

	// Связанные сущности
	RelatedID   *uint  `json:"related_id"`           // ID связанной сущности
	RelatedType string `json:"related_type"`         // Тип связанной сущности
	UserID      *uint  `json:"user_id" gorm:"index"` // ID пользователя-получателя

	// Метаданные
	TemplateID   *uint      `json:"template_id" gorm:"index"`       // ID использованного шаблона
	AttemptCount int        `json:"attempt_count" gorm:"default:0"` // Количество попыток
	NextRetryAt  *time.Time `json:"next_retry_at"`                  // Время следующей попытки
	ExternalID   string     `json:"external_id"`                    // ID во внешней системе (Telegram message_id)

	// Для мультитенантности
	CompanyID uuid.UUID `json:"company_id" gorm:"type:uuid;index"`

	// Связи
	Template *NotificationTemplate `json:"template,omitempty" gorm:"foreignKey:TemplateID"`
	User     *User                 `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// NotificationSettings представляет настройки уведомлений для компании
type NotificationSettings struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Telegram настройки
	TelegramBotToken   string `json:"telegram_bot_token" gorm:"type:varchar(500)"` // Токен бота
	TelegramWebhookURL string `json:"telegram_webhook_url"`                        // URL для вебхуков
	TelegramEnabled    bool   `json:"telegram_enabled" gorm:"default:false"`       // Включен ли Telegram

	// Email настройки
	SMTPHost      string `json:"smtp_host"`                          // SMTP сервер
	SMTPPort      int    `json:"smtp_port" gorm:"default:587"`       // SMTP порт
	SMTPUsername  string `json:"smtp_username"`                      // SMTP логин
	SMTPPassword  string `json:"smtp_password"`                      // SMTP пароль
	SMTPFromEmail string `json:"smtp_from_email"`                    // Email отправителя
	SMTPFromName  string `json:"smtp_from_name"`                     // Имя отправителя
	SMTPUseTLS    bool   `json:"smtp_use_tls" gorm:"default:true"`   // Использовать TLS
	EmailEnabled  bool   `json:"email_enabled" gorm:"default:false"` // Включен ли Email

	// SMS настройки
	SMSProvider   string `json:"sms_provider"`                     // Провайдер SMS
	SMSApiKey     string `json:"sms_api_key"`                      // API ключ
	SMSApiSecret  string `json:"sms_api_secret"`                   // API секрет
	SMSFromNumber string `json:"sms_from_number"`                  // Номер отправителя
	SMSEnabled    bool   `json:"sms_enabled" gorm:"default:false"` // Включен ли SMS

	// Общие настройки
	DefaultLanguage   string `json:"default_language" gorm:"default:'ru'"` // Язык по умолчанию
	MaxRetryAttempts  int    `json:"max_retry_attempts" gorm:"default:3"`  // Максимум попыток
	RetryDelayMinutes int    `json:"retry_delay_minutes" gorm:"default:5"` // Задержка между попытками

	// Для мультитенантности
	CompanyID uuid.UUID `json:"company_id" gorm:"type:uuid;uniqueIndex"`

	// Связи
	Company *Company `json:"company,omitempty" gorm:"foreignKey:CompanyID"`
}

// UserNotificationPreferences представляет предпочтения пользователя по уведомлениям
type UserNotificationPreferences struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связи
	UserID uint  `json:"user_id" gorm:"not null;index"`
	User   *User `json:"user,omitempty" gorm:"foreignKey:UserID"`

	// Настройки каналов
	TelegramEnabled bool `json:"telegram_enabled" gorm:"default:true"`
	EmailEnabled    bool `json:"email_enabled" gorm:"default:true"`
	SMSEnabled      bool `json:"sms_enabled" gorm:"default:false"`

	// Настройки типов уведомлений
	InstallationReminders bool `json:"installation_reminders" gorm:"default:true"`
	InstallationUpdates   bool `json:"installation_updates" gorm:"default:true"`
	BillingAlerts         bool `json:"billing_alerts" gorm:"default:true"`
	WarehouseAlerts       bool `json:"warehouse_alerts" gorm:"default:true"`
	SystemNotifications   bool `json:"system_notifications" gorm:"default:true"`

	// Настройки времени
	QuietHoursStart string `json:"quiet_hours_start" gorm:"default:'22:00'"` // Начало тихих часов
	QuietHoursEnd   string `json:"quiet_hours_end" gorm:"default:'08:00'"`   // Конец тихих часов
	Timezone        string `json:"timezone" gorm:"default:'Europe/Moscow'"`  // Часовой пояс

	// Для мультитенантности
	CompanyID uuid.UUID `json:"company_id" gorm:"type:uuid;index"`
}

// GetStatusDisplayName возвращает читаемое название статуса
func (nl *NotificationLog) GetStatusDisplayName() string {
	switch nl.Status {
	case "pending":
		return "Ожидает отправки"
	case "sent":
		return "Отправлено"
	case "failed":
		return "Ошибка отправки"
	case "retry":
		return "Повторная попытка"
	default:
		return "Неизвестно"
	}
}

// GetChannelDisplayName возвращает читаемое название канала
func (nl *NotificationLog) GetChannelDisplayName() string {
	switch nl.Channel {
	case "telegram":
		return "Telegram"
	case "email":
		return "Email"
	case "sms":
		return "SMS"
	default:
		return "Неизвестно"
	}
}

// GetPriorityDisplayName возвращает читаемое название приоритета
func (nt *NotificationTemplate) GetPriorityDisplayName() string {
	switch nt.Priority {
	case "low":
		return "Низкий"
	case "normal":
		return "Обычный"
	case "high":
		return "Высокий"
	case "urgent":
		return "Срочный"
	default:
		return "Обычный"
	}
}

// IsInQuietHours проверяет, находится ли текущее время в тихих часах
func (unp *UserNotificationPreferences) IsInQuietHours() bool {
	now := time.Now()
	currentTime := now.Format("15:04")

	// Простая проверка - если тихие часы не пересекают полночь
	if unp.QuietHoursStart < unp.QuietHoursEnd {
		return currentTime >= unp.QuietHoursStart && currentTime <= unp.QuietHoursEnd
	}

	// Если тихие часы пересекают полночь (например, с 22:00 до 08:00)
	return currentTime >= unp.QuietHoursStart || currentTime <= unp.QuietHoursEnd
}
