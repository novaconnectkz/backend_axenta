package models

import (
	"time"

	"gorm.io/gorm"
)

// ObjectTemplate представляет шаблон объекта с предустановленными настройками
type ObjectTemplate struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля шаблона
	Name        string `json:"name" gorm:"not null;type:varchar(100)"`
	Description string `json:"description" gorm:"type:text"`
	Category    string `json:"category" gorm:"type:varchar(50)"` // vehicle, equipment, asset, etc.

	// Иконка и цвет для UI
	Icon  string `json:"icon" gorm:"type:varchar(50)"` // Название иконки
	Color string `json:"color" gorm:"type:varchar(7)"` // HEX цвет

	// Конфигурация объекта (JSON)
	Config string `json:"config" gorm:"type:jsonb"` // Настройки мониторинга, уведомлений и т.д.

	// Настройки по умолчанию
	DefaultSettings string `json:"default_settings" gorm:"type:jsonb"` // Настройки, применяемые к новым объектам

	// Требуемое оборудование
	RequiredEquipment []string `json:"required_equipment" gorm:"type:text[]"` // Типы оборудования

	// Статус
	IsActive   bool `json:"is_active" gorm:"default:true"`
	IsSystem   bool `json:"is_system" gorm:"default:false"` // Системный шаблон (нельзя удалить)
	UsageCount int  `json:"usage_count" gorm:"default:0"`   // Количество использований

	// Связи
	Objects []Object `json:"objects,omitempty" gorm:"foreignKey:TemplateID"`
}

// TableName задает имя таблицы для модели ObjectTemplate
func (ObjectTemplate) TableName() string {
	return "object_templates"
}

// IncrementUsage увеличивает счетчик использований шаблона
func (ot *ObjectTemplate) IncrementUsage(db *gorm.DB) error {
	return db.Model(ot).UpdateColumn("usage_count", gorm.Expr("usage_count + ?", 1)).Error
}

// GetConfigValue извлекает значение из JSON конфигурации
func (ot *ObjectTemplate) GetConfigValue(key string) interface{} {
	// TODO: Реализовать парсинг JSON и извлечение значения по ключу
	return nil
}

// SetConfigValue устанавливает значение в JSON конфигурацию
func (ot *ObjectTemplate) SetConfigValue(key string, value interface{}) error {
	// TODO: Реализовать установку значения в JSON конфигурацию
	return nil
}

// MonitoringTemplate представляет шаблон настроек мониторинга
type MonitoringTemplate struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	Name        string `json:"name" gorm:"not null;type:varchar(100)"`
	Description string `json:"description" gorm:"type:text"`

	// Настройки мониторинга
	CheckInterval   int  `json:"check_interval" gorm:"default:300"`      // Интервал проверки в секундах
	AlertThreshold  int  `json:"alert_threshold" gorm:"default:600"`     // Порог для тревоги в секундах
	GeoFenceEnabled bool `json:"geo_fence_enabled" gorm:"default:false"` // Включен ли геозабор
	SpeedLimit      int  `json:"speed_limit" gorm:"default:0"`           // Ограничение скорости (0 = нет)

	// Уведомления
	NotifyOnOffline  bool `json:"notify_on_offline" gorm:"default:true"`
	NotifyOnMove     bool `json:"notify_on_move" gorm:"default:false"`
	NotifyOnSpeed    bool `json:"notify_on_speed" gorm:"default:false"`
	NotifyOnGeoFence bool `json:"notify_on_geo_fence" gorm:"default:false"`

	// Каналы уведомлений
	EmailEnabled    bool `json:"email_enabled" gorm:"default:true"`
	SMSEnabled      bool `json:"sms_enabled" gorm:"default:false"`
	TelegramEnabled bool `json:"telegram_enabled" gorm:"default:false"`
	WebhookEnabled  bool `json:"webhook_enabled" gorm:"default:false"`

	// Дополнительные настройки (JSON)
	Settings string `json:"settings" gorm:"type:jsonb"`

	// Статус
	IsActive   bool `json:"is_active" gorm:"default:true"`
	UsageCount int  `json:"usage_count" gorm:"default:0"`
}

// TableName задает имя таблицы для модели MonitoringTemplate
func (MonitoringTemplate) TableName() string {
	return "monitoring_templates"
}

// MonitoringNotificationTemplate представляет шаблон уведомлений для мониторинга
type MonitoringNotificationTemplate struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	Name        string `json:"name" gorm:"not null;type:varchar(100)"`
	Description string `json:"description" gorm:"type:text"`
	Type        string `json:"type" gorm:"not null;type:varchar(50)"` // alert, info, warning, reminder

	// Событие, для которого предназначен шаблон
	EventType string `json:"event_type" gorm:"not null;type:varchar(50)"` // offline, online, move, speed, etc.

	// Шаблоны сообщений
	EmailSubject    string `json:"email_subject" gorm:"type:varchar(200)"`
	EmailBody       string `json:"email_body" gorm:"type:text"`
	SMSMessage      string `json:"sms_message" gorm:"type:varchar(160)"`
	TelegramMessage string `json:"telegram_message" gorm:"type:text"`
	WebhookPayload  string `json:"webhook_payload" gorm:"type:text"`

	// Настройки доставки
	Priority      string `json:"priority" gorm:"default:'normal';type:varchar(20)"` // low, normal, high, urgent
	RetryCount    int    `json:"retry_count" gorm:"default:3"`
	RetryInterval int    `json:"retry_interval" gorm:"default:300"` // Интервал повтора в секундах

	// Ограничения
	MaxPerHour int `json:"max_per_hour" gorm:"default:0"` // Максимум сообщений в час (0 = без ограничений)
	MaxPerDay  int `json:"max_per_day" gorm:"default:0"`  // Максимум сообщений в день (0 = без ограничений)

	// Время действия
	ActiveFrom  *time.Time `json:"active_from"`  // С какого времени активен шаблон
	ActiveUntil *time.Time `json:"active_until"` // До какого времени активен шаблон

	// Дни недели (битовая маска: 1=Пн, 2=Вт, 4=Ср, 8=Чт, 16=Пт, 32=Сб, 64=Вс)
	WeekDays int `json:"week_days" gorm:"default:127"` // По умолчанию все дни

	// Время дня
	TimeFrom  string `json:"time_from" gorm:"type:varchar(5)"`  // Формат HH:MM
	TimeUntil string `json:"time_until" gorm:"type:varchar(5)"` // Формат HH:MM

	// Статус
	IsActive   bool `json:"is_active" gorm:"default:true"`
	UsageCount int  `json:"usage_count" gorm:"default:0"`

	// Переменные для подстановки в шаблон
	Variables string `json:"variables" gorm:"type:jsonb"` // Доступные переменные и их описания
}

// TableName задает имя таблицы для модели MonitoringNotificationTemplate
func (MonitoringNotificationTemplate) TableName() string {
	return "monitoring_notification_templates"
}

// IsActiveNow проверяет, активен ли шаблон в данный момент
func (nt *MonitoringNotificationTemplate) IsActiveNow() bool {
	if !nt.IsActive {
		return false
	}

	now := time.Now()

	// Проверяем период действия
	if nt.ActiveFrom != nil && now.Before(*nt.ActiveFrom) {
		return false
	}
	if nt.ActiveUntil != nil && now.After(*nt.ActiveUntil) {
		return false
	}

	// Проверяем день недели
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7 // Воскресенье = 7
	}
	weekdayBit := 1 << (weekday - 1)
	if nt.WeekDays&weekdayBit == 0 {
		return false
	}

	// Проверяем время дня
	if nt.TimeFrom != "" && nt.TimeUntil != "" {
		currentTime := now.Format("15:04")
		if currentTime < nt.TimeFrom || currentTime > nt.TimeUntil {
			return false
		}
	}

	return true
}

// RenderMessage рендерит сообщение с подстановкой переменных
func (nt *MonitoringNotificationTemplate) RenderMessage(messageType string, variables map[string]interface{}) string {
	var template string

	switch messageType {
	case "email_subject":
		template = nt.EmailSubject
	case "email_body":
		template = nt.EmailBody
	case "sms":
		template = nt.SMSMessage
	case "telegram":
		template = nt.TelegramMessage
	case "webhook":
		template = nt.WebhookPayload
	default:
		return ""
	}

	// TODO: Реализовать подстановку переменных в шаблон
	// Например, заменить {{object_name}} на значение из variables["object_name"]

	return template
}
