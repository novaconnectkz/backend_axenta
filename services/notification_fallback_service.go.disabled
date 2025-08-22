package services

import (
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"backend_axenta/models"
)

// NotificationFallbackService представляет сервис для fallback уведомлений
type NotificationFallbackService struct {
	DB                  *gorm.DB
	notificationService *NotificationService
}

// NewNotificationFallbackService создает новый экземпляр NotificationFallbackService
func NewNotificationFallbackService(db *gorm.DB, notificationService *NotificationService) *NotificationFallbackService {
	return &NotificationFallbackService{
		DB:                  db,
		notificationService: notificationService,
	}
}

// FallbackChannel представляет канал для fallback
type FallbackChannel struct {
	Channel   string `json:"channel"`
	Priority  int    `json:"priority"`
	Enabled   bool   `json:"enabled"`
	Recipient string `json:"recipient"`
}

// SendWithFallback отправляет уведомление с fallback механизмом
func (s *NotificationFallbackService) SendWithFallback(notificationType string, recipient string, templateData map[string]interface{}, companyID, relatedID uint, relatedType string) error {
	// Получаем пользователя и его предпочтения
	var user models.User
	err := s.DB.Where("telegram_id = ? OR phone = ? OR email = ?", recipient, recipient, recipient).
		Where("company_id = ?", companyID).First(&user).Error
	if err != nil {
		// Если пользователь не найден, пробуем отправить напрямую
		return s.sendDirectFallback(notificationType, recipient, templateData, companyID, relatedID, relatedType)
	}

	// Получаем предпочтения пользователя
	var prefs models.UserNotificationPreferences
	err = s.DB.Where("user_id = ?", user.ID).First(&prefs).Error
	if err != nil {
		// Создаем предпочтения по умолчанию
		prefs = models.UserNotificationPreferences{
			UserID:                user.ID,
			CompanyID:             companyID,
			TelegramEnabled:       true,
			EmailEnabled:          true,
			SMSEnabled:            false,
			InstallationReminders: true,
			InstallationUpdates:   true,
			BillingAlerts:         true,
			WarehouseAlerts:       true,
			SystemNotifications:   true,
		}
	}

	// Проверяем тихие часы
	if prefs.IsInQuietHours() && !s.isUrgentNotification(notificationType) {
		// Планируем отправку после тихих часов
		return s.scheduleAfterQuietHours(&prefs, notificationType, templateData, companyID, relatedID, relatedType)
	}

	// Определяем каналы для отправки в порядке приоритета
	fallbackChannels := s.getFallbackChannels(&user, &prefs, notificationType)

	var lastError error
	for _, channel := range fallbackChannels {
		if !channel.Enabled {
			continue
		}

		err := s.notificationService.SendNotification(notificationType, channel.Channel, channel.Recipient, templateData, companyID, relatedID, relatedType)
		if err == nil {
			// Успешно отправлено
			return nil
		}

		lastError = err
		log.Printf("Ошибка отправки через %s: %v, пробуем следующий канал", channel.Channel, err)
	}

	// Если все каналы не сработали, возвращаем последнюю ошибку
	return fmt.Errorf("не удалось отправить уведомление ни через один канал: %w", lastError)
}

// getFallbackChannels возвращает список каналов в порядке приоритета
func (s *NotificationFallbackService) getFallbackChannels(user *models.User, prefs *models.UserNotificationPreferences, notificationType string) []FallbackChannel {
	channels := []FallbackChannel{}

	// Определяем приоритет каналов в зависимости от типа уведомления
	switch notificationType {
	case "installation_reminder", "installation_created", "installation_updated":
		// Для монтажей приоритет: Telegram -> SMS -> Email
		if prefs.TelegramEnabled && user.TelegramID != "" && prefs.InstallationReminders {
			channels = append(channels, FallbackChannel{
				Channel:   "telegram",
				Priority:  1,
				Enabled:   true,
				Recipient: user.TelegramID,
			})
		}
		if prefs.SMSEnabled && user.Phone != "" && prefs.InstallationReminders {
			channels = append(channels, FallbackChannel{
				Channel:   "sms",
				Priority:  2,
				Enabled:   true,
				Recipient: user.Phone,
			})
		}
		if prefs.EmailEnabled && user.Email != "" && prefs.InstallationUpdates {
			channels = append(channels, FallbackChannel{
				Channel:   "email",
				Priority:  3,
				Enabled:   true,
				Recipient: user.Email,
			})
		}

	case "billing_alert", "invoice_created", "payment_overdue":
		// Для биллинга приоритет: Email -> Telegram -> SMS
		if prefs.EmailEnabled && user.Email != "" && prefs.BillingAlerts {
			channels = append(channels, FallbackChannel{
				Channel:   "email",
				Priority:  1,
				Enabled:   true,
				Recipient: user.Email,
			})
		}
		if prefs.TelegramEnabled && user.TelegramID != "" && prefs.BillingAlerts {
			channels = append(channels, FallbackChannel{
				Channel:   "telegram",
				Priority:  2,
				Enabled:   true,
				Recipient: user.TelegramID,
			})
		}
		if prefs.SMSEnabled && user.Phone != "" && prefs.BillingAlerts {
			channels = append(channels, FallbackChannel{
				Channel:   "sms",
				Priority:  3,
				Enabled:   true,
				Recipient: user.Phone,
			})
		}

	case "stock_alert", "warranty_alert", "maintenance_alert":
		// Для складских уведомлений приоритет: Telegram -> Email -> SMS
		if prefs.TelegramEnabled && user.TelegramID != "" && prefs.WarehouseAlerts {
			channels = append(channels, FallbackChannel{
				Channel:   "telegram",
				Priority:  1,
				Enabled:   true,
				Recipient: user.TelegramID,
			})
		}
		if prefs.EmailEnabled && user.Email != "" && prefs.WarehouseAlerts {
			channels = append(channels, FallbackChannel{
				Channel:   "email",
				Priority:  2,
				Enabled:   true,
				Recipient: user.Email,
			})
		}
		if prefs.SMSEnabled && user.Phone != "" && prefs.WarehouseAlerts {
			channels = append(channels, FallbackChannel{
				Channel:   "sms",
				Priority:  3,
				Enabled:   true,
				Recipient: user.Phone,
			})
		}

	default:
		// По умолчанию: Telegram -> Email -> SMS
		if prefs.TelegramEnabled && user.TelegramID != "" && prefs.SystemNotifications {
			channels = append(channels, FallbackChannel{
				Channel:   "telegram",
				Priority:  1,
				Enabled:   true,
				Recipient: user.TelegramID,
			})
		}
		if prefs.EmailEnabled && user.Email != "" && prefs.SystemNotifications {
			channels = append(channels, FallbackChannel{
				Channel:   "email",
				Priority:  2,
				Enabled:   true,
				Recipient: user.Email,
			})
		}
		if prefs.SMSEnabled && user.Phone != "" && prefs.SystemNotifications {
			channels = append(channels, FallbackChannel{
				Channel:   "sms",
				Priority:  3,
				Enabled:   true,
				Recipient: user.Phone,
			})
		}
	}

	return channels
}

// sendDirectFallback отправляет уведомление напрямую без учета предпочтений пользователя
func (s *NotificationFallbackService) sendDirectFallback(notificationType string, recipient string, templateData map[string]interface{}, companyID, relatedID uint, relatedType string) error {
	// Определяем тип получателя по формату
	var channels []string

	if s.isTelegramID(recipient) {
		channels = []string{"telegram"}
	} else if s.isPhoneNumber(recipient) {
		channels = []string{"sms", "telegram"} // Пробуем SMS, потом Telegram
	} else if s.isEmail(recipient) {
		channels = []string{"email", "telegram"} // Пробуем Email, потом Telegram
	} else {
		// Неопределенный формат, пробуем все каналы
		channels = []string{"telegram", "email", "sms"}
	}

	var lastError error
	for _, channel := range channels {
		err := s.notificationService.SendNotification(notificationType, channel, recipient, templateData, companyID, relatedID, relatedType)
		if err == nil {
			return nil
		}
		lastError = err
		log.Printf("Ошибка прямой отправки через %s: %v", channel, err)
	}

	return fmt.Errorf("не удалось отправить уведомление: %w", lastError)
}

// isUrgentNotification проверяет, является ли уведомление срочным (не учитывает тихие часы)
func (s *NotificationFallbackService) isUrgentNotification(notificationType string) bool {
	urgentTypes := map[string]bool{
		"emergency_alert":     true,
		"security_alert":      true,
		"system_critical":     true,
		"installation_urgent": true,
		"payment_critical":    true,
	}
	return urgentTypes[notificationType]
}

// scheduleAfterQuietHours планирует отправку уведомления после тихих часов
func (s *NotificationFallbackService) scheduleAfterQuietHours(prefs *models.UserNotificationPreferences, notificationType string, templateData map[string]interface{}, companyID, relatedID uint, relatedType string) error {
	// Вычисляем время окончания тихих часов
	now := time.Now()
	endTime, err := time.Parse("15:04", prefs.QuietHoursEnd)
	if err != nil {
		return fmt.Errorf("неверный формат времени окончания тихих часов: %w", err)
	}

	// Определяем дату отправки
	var scheduleTime time.Time
	if now.Hour() < endTime.Hour() || (now.Hour() == endTime.Hour() && now.Minute() < endTime.Minute()) {
		// Сегодня
		scheduleTime = time.Date(now.Year(), now.Month(), now.Day(), endTime.Hour(), endTime.Minute(), 0, 0, now.Location())
	} else {
		// Завтра
		tomorrow := now.AddDate(0, 0, 1)
		scheduleTime = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), endTime.Hour(), endTime.Minute(), 0, 0, now.Location())
	}

	// Создаем отложенное уведомление
	delayedNotification := models.NotificationLog{
		Type:        notificationType,
		Channel:     "scheduled",
		Recipient:   fmt.Sprintf("user_%d", prefs.UserID),
		Subject:     "Отложенное уведомление",
		Message:     fmt.Sprintf("Уведомление запланировано на %s", scheduleTime.Format("02.01.2006 15:04")),
		Status:      "pending",
		RelatedID:   &relatedID,
		RelatedType: relatedType,
		CompanyID:   companyID,
		UserID:      &prefs.UserID,
		NextRetryAt: &scheduleTime,
		CreatedAt:   time.Now(),
	}

	return s.DB.Create(&delayedNotification).Error
}

// ProcessScheduledNotifications обрабатывает отложенные уведомления
func (s *NotificationFallbackService) ProcessScheduledNotifications() error {
	var notifications []models.NotificationLog
	err := s.DB.Where("channel = 'scheduled' AND status = 'pending' AND next_retry_at <= ?", time.Now()).
		Preload("User").Find(&notifications).Error
	if err != nil {
		return fmt.Errorf("ошибка получения отложенных уведомлений: %w", err)
	}

	for _, notification := range notifications {
		if notification.User == nil {
			log.Printf("Пользователь не найден для уведомления ID %d", notification.ID)
			continue
		}

		// Создаем template data (восстанавливаем из сохраненных данных)
		templateData := map[string]interface{}{
			"User":        notification.User,
			"Delayed":     true,
			"ScheduledAt": notification.NextRetryAt,
		}

		// Отправляем с fallback
		err := s.SendWithFallback(notification.Type, "", templateData,
			notification.CompanyID, *notification.RelatedID, notification.RelatedType)

		// Обновляем статус
		if err != nil {
			notification.Status = "failed"
			notification.ErrorMessage = err.Error()
		} else {
			notification.Status = "sent"
			now := time.Now()
			notification.SentAt = &now
		}

		s.DB.Save(&notification)
	}

	return nil
}

// Вспомогательные функции для определения типа получателя

func (s *NotificationFallbackService) isTelegramID(recipient string) bool {
	// Telegram ID обычно состоит только из цифр
	if len(recipient) < 6 || len(recipient) > 12 {
		return false
	}
	for _, r := range recipient {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s *NotificationFallbackService) isPhoneNumber(recipient string) bool {
	// Простая проверка номера телефона
	if len(recipient) < 10 {
		return false
	}
	// Проверяем, что начинается с + или цифры
	if recipient[0] != '+' && (recipient[0] < '0' || recipient[0] > '9') {
		return false
	}
	return true
}

func (s *NotificationFallbackService) isEmail(recipient string) bool {
	// Простая проверка email
	return len(recipient) > 5 &&
		(recipient[0] != '@') &&
		(recipient[len(recipient)-1] != '@') &&
		(contains(recipient, "@")) &&
		(contains(recipient, "."))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// SendBulkWithFallback отправляет уведомления группе пользователей с fallback
func (s *NotificationFallbackService) SendBulkWithFallback(notificationType string, userIDs []uint, templateData map[string]interface{}, companyID, relatedID uint, relatedType string) error {
	for _, userID := range userIDs {
		// Получаем пользователя
		var user models.User
		err := s.DB.Where("id = ? AND company_id = ?", userID, companyID).First(&user).Error
		if err != nil {
			log.Printf("Пользователь ID %d не найден: %v", userID, err)
			continue
		}

		// Отправляем с fallback
		go func(u models.User) {
			err := s.SendWithFallback(notificationType, u.TelegramID, templateData, companyID, relatedID, relatedType)
			if err != nil {
				log.Printf("Ошибка отправки уведомления пользователю %s: %v", u.Username, err)
			}
		}(user)
	}

	return nil
}

// GetFailedNotifications возвращает список неудачных уведомлений для повторной отправки
func (s *NotificationFallbackService) GetFailedNotifications(companyID uint, hours int) ([]models.NotificationLog, error) {
	var notifications []models.NotificationLog
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	err := s.DB.Where("company_id = ? AND status = 'failed' AND created_at >= ?", companyID, since).
		Preload("User").Preload("Template").Find(&notifications).Error

	return notifications, err
}

// RetryFailedNotification повторно отправляет неудачное уведомление
func (s *NotificationFallbackService) RetryFailedNotification(notificationID uint) error {
	var notification models.NotificationLog
	err := s.DB.Preload("User").Preload("Template").First(&notification, notificationID).Error
	if err != nil {
		return fmt.Errorf("уведомление не найдено: %w", err)
	}

	if notification.Status != "failed" {
		return fmt.Errorf("уведомление не в статусе 'failed'")
	}

	// Создаем template data
	templateData := map[string]interface{}{
		"Retry":         true,
		"OriginalError": notification.ErrorMessage,
	}

	// Отправляем с fallback
	recipient := notification.Recipient
	if notification.User != nil {
		// Используем данные пользователя для fallback
		recipient = notification.User.TelegramID
		if recipient == "" {
			recipient = notification.User.Phone
		}
		if recipient == "" {
			recipient = notification.User.Email
		}
	}

	return s.SendWithFallback(notification.Type, recipient, templateData,
		notification.CompanyID, *notification.RelatedID, notification.RelatedType)
}
