package services

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"time"

	"gorm.io/gorm"

	"backend_axenta/models"
)

// NotificationService представляет сервис для отправки уведомлений
type NotificationService struct {
	DB              *gorm.DB
	telegramClients map[uint]*TelegramClient // Карта клиентов Telegram по company_id
	cache           *CacheService
}

// NewNotificationService создает новый экземпляр NotificationService
func NewNotificationService(db *gorm.DB, cache *CacheService) *NotificationService {
	return &NotificationService{
		DB:              db,
		telegramClients: make(map[uint]*TelegramClient),
		cache:           cache,
	}
}

// getTelegramClient получает или создает Telegram клиент для компании
func (s *NotificationService) getTelegramClient(companyID uint) (*TelegramClient, error) {
	// Проверяем кэш
	if client, exists := s.telegramClients[companyID]; exists {
		if client.IsHealthy() {
			return client, nil
		}
		// Если клиент нездоров, удаляем его из кэша
		delete(s.telegramClients, companyID)
	}

	// Создаем новый клиент
	client, err := NewTelegramClient(s.DB, companyID)
	if err != nil {
		return nil, err
	}

	// Сохраняем в кэш
	s.telegramClients[companyID] = client
	return client, nil
}

// SendNotification отправляет уведомление используя шаблон
func (s *NotificationService) SendNotification(notificationType, channel, recipient string, templateData map[string]interface{}, companyID, relatedID uint, relatedType string) error {
	// Получаем шаблон уведомления
	template, err := s.getNotificationTemplate(notificationType, channel, companyID)
	if err != nil {
		return fmt.Errorf("шаблон не найден: %w", err)
	}

	// Рендерим шаблон
	subject, message, err := s.renderTemplate(template, templateData)
	if err != nil {
		return fmt.Errorf("ошибка рендеринга шаблона: %w", err)
	}

	// Отправляем уведомление
	return s.sendNotificationWithTemplate(template, recipient, subject, message, &relatedID, relatedType, companyID)
}

// getNotificationTemplate получает шаблон уведомления
func (s *NotificationService) getNotificationTemplate(notificationType, channel string, companyID uint) (*models.NotificationTemplate, error) {
	var template models.NotificationTemplate
	err := s.DB.Where("type = ? AND channel = ? AND company_id = ? AND is_active = true",
		notificationType, channel, companyID).First(&template).Error

	if err != nil {
		// Пробуем найти глобальный шаблон (company_id = 0)
		err = s.DB.Where("type = ? AND channel = ? AND company_id = 0 AND is_active = true",
			notificationType, channel).First(&template).Error
	}

	return &template, err
}

// renderTemplate рендерит шаблон с данными
func (s *NotificationService) renderTemplate(tmpl *models.NotificationTemplate, data map[string]interface{}) (string, string, error) {
	// Рендерим тему (subject)
	var subject string
	if tmpl.Subject != "" {
		subjectTmpl, err := template.New("subject").Parse(tmpl.Subject)
		if err != nil {
			return "", "", fmt.Errorf("ошибка парсинга темы: %w", err)
		}

		var subjectBuf bytes.Buffer
		if err := subjectTmpl.Execute(&subjectBuf, data); err != nil {
			return "", "", fmt.Errorf("ошибка рендеринга темы: %w", err)
		}
		subject = subjectBuf.String()
	}

	// Рендерим сообщение
	messageTmpl, err := template.New("message").Parse(tmpl.Template)
	if err != nil {
		return "", "", fmt.Errorf("ошибка парсинга шаблона: %w", err)
	}

	var messageBuf bytes.Buffer
	if err := messageTmpl.Execute(&messageBuf, data); err != nil {
		return "", "", fmt.Errorf("ошибка рендеринга сообщения: %w", err)
	}

	return subject, messageBuf.String(), nil
}

// sendNotificationWithTemplate отправляет уведомление с использованием шаблона
func (s *NotificationService) sendNotificationWithTemplate(template *models.NotificationTemplate, recipient, subject, message string, relatedID *uint, relatedType string, companyID uint) error {
	// Создаем запись в логе
	notificationLog := models.NotificationLog{
		Type:        template.Type,
		Channel:     template.Channel,
		Recipient:   recipient,
		Subject:     subject,
		Message:     message,
		Status:      "pending",
		RelatedID:   relatedID,
		RelatedType: relatedType,
		CompanyID:   companyID,
		TemplateID:  &template.ID,
		CreatedAt:   time.Now(),
	}

	// Пытаемся отправить уведомление
	var err error
	switch template.Channel {
	case "telegram":
		err = s.sendTelegramNotification(recipient, message, companyID)
	case "email":
		err = s.sendEmailNotification(recipient, subject, message, companyID)
	case "sms":
		err = s.sendSMSNotification(recipient, message, companyID)
	default:
		err = fmt.Errorf("неподдерживаемый канал уведомлений: %s", template.Channel)
	}

	// Обновляем статус в логе
	if err != nil {
		notificationLog.Status = "failed"
		notificationLog.ErrorMessage = err.Error()
		notificationLog.AttemptCount = 1

		// Планируем повторную попытку
		if template.RetryAttempts > 0 {
			notificationLog.Status = "retry"
			nextRetry := time.Now().Add(time.Duration(5) * time.Minute) // 5 минут по умолчанию
			notificationLog.NextRetryAt = &nextRetry
		}
	} else {
		notificationLog.Status = "sent"
		now := time.Now()
		notificationLog.SentAt = &now
	}

	// Сохраняем лог
	s.DB.Create(&notificationLog)

	return err
}

// sendTelegramNotification отправляет уведомление через Telegram
func (s *NotificationService) sendTelegramNotification(recipient, message string, companyID uint) error {
	client, err := s.getTelegramClient(companyID)
	if err != nil {
		return fmt.Errorf("ошибка получения Telegram клиента: %w", err)
	}

	_, err = client.SendMessage(recipient, message)
	return err
}

// sendEmailNotification отправляет email уведомление
func (s *NotificationService) sendEmailNotification(recipient, subject, message string, companyID uint) error {
	// Получаем настройки email для компании
	var settings models.NotificationSettings
	err := s.DB.Where("company_id = ?", companyID).First(&settings).Error
	if err != nil {
		return fmt.Errorf("настройки email не найдены: %w", err)
	}

	if !settings.EmailEnabled {
		return fmt.Errorf("email уведомления отключены для компании %d", companyID)
	}

	// Настраиваем SMTP
	auth := smtp.PlainAuth("", settings.SMTPUsername, settings.SMTPPassword, settings.SMTPHost)

	// Формируем сообщение
	msg := fmt.Sprintf("From: %s <%s>\r\n", settings.SMTPFromName, settings.SMTPFromEmail)
	msg += fmt.Sprintf("To: %s\r\n", recipient)
	msg += fmt.Sprintf("Subject: %s\r\n", subject)
	msg += "Content-Type: text/html; charset=UTF-8\r\n"
	msg += "\r\n"
	msg += message

	// Отправляем
	addr := fmt.Sprintf("%s:%d", settings.SMTPHost, settings.SMTPPort)

	if settings.SMTPUseTLS {
		// Используем TLS
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         settings.SMTPHost,
		}

		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("ошибка TLS подключения: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, settings.SMTPHost)
		if err != nil {
			return fmt.Errorf("ошибка создания SMTP клиента: %w", err)
		}
		defer client.Quit()

		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("ошибка аутентификации: %w", err)
		}

		if err = client.Mail(settings.SMTPFromEmail); err != nil {
			return fmt.Errorf("ошибка установки отправителя: %w", err)
		}

		if err = client.Rcpt(recipient); err != nil {
			return fmt.Errorf("ошибка установки получателя: %w", err)
		}

		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("ошибка получения writer: %w", err)
		}

		_, err = w.Write([]byte(msg))
		if err != nil {
			return fmt.Errorf("ошибка записи сообщения: %w", err)
		}

		err = w.Close()
		if err != nil {
			return fmt.Errorf("ошибка закрытия writer: %w", err)
		}
	} else {
		// Обычный SMTP без TLS
		err = smtp.SendMail(addr, auth, settings.SMTPFromEmail, []string{recipient}, []byte(msg))
		if err != nil {
			return fmt.Errorf("ошибка отправки email: %w", err)
		}
	}

	return nil
}

// sendSMSNotification отправляет SMS уведомление
func (s *NotificationService) sendSMSNotification(recipient, message string, companyID uint) error {
	// Получаем настройки SMS для компании
	var settings models.NotificationSettings
	err := s.DB.Where("company_id = ?", companyID).First(&settings).Error
	if err != nil {
		return fmt.Errorf("настройки SMS не найдены: %w", err)
	}

	if !settings.SMSEnabled {
		return fmt.Errorf("SMS уведомления отключены для компании %d", companyID)
	}

	// TODO: Интеграция с SMS провайдером (зависит от выбранного провайдера)
	log.Printf("SMS to %s: %s (provider: %s)", recipient, message, settings.SMSProvider)

	// Заглушка - в реальном проекте здесь будет интеграция с конкретным SMS провайдером
	return nil
}

// SendInstallationReminder отправляет напоминание о предстоящем монтаже
func (s *NotificationService) SendInstallationReminder(installation *models.Installation) error {
	templateData := map[string]interface{}{
		"Installation":  installation,
		"Object":        installation.Object,
		"Installer":     installation.Installer,
		"Time":          installation.ScheduledAt.Format("15:04"),
		"Date":          installation.ScheduledAt.Format("02.01.2006"),
		"Address":       installation.Address,
		"ClientContact": installation.ClientContact,
	}

	// Отправляем уведомление монтажнику через Telegram
	if installation.Installer != nil && installation.Installer.TelegramID != "" {
		err := s.SendNotification("installation_reminder", "telegram", installation.Installer.TelegramID,
			templateData, installation.CompanyID, installation.ID, "installation")
		if err != nil {
			log.Printf("Ошибка отправки Telegram напоминания монтажнику: %v", err)
		}
	}

	// Отправляем уведомление монтажнику через SMS
	if installation.Installer != nil && installation.Installer.Phone != "" {
		err := s.SendNotification("installation_reminder", "sms", installation.Installer.Phone,
			templateData, installation.CompanyID, installation.ID, "installation")
		if err != nil {
			log.Printf("Ошибка отправки SMS напоминания монтажнику: %v", err)
		}
	}

	// Отправляем уведомление клиенту через SMS (если указан контакт)
	if installation.ClientContact != "" {
		err := s.SendNotification("installation_reminder_client", "sms", installation.ClientContact,
			templateData, installation.CompanyID, installation.ID, "installation")
		if err != nil {
			log.Printf("Ошибка отправки SMS напоминания клиенту: %v", err)
		}
	}

	return nil
}

// SendInstallationCreated отправляет уведомление о создании нового монтажа
func (s *NotificationService) SendInstallationCreated(installation *models.Installation) error {
	templateData := map[string]interface{}{
		"Installation":  installation,
		"Object":        installation.Object,
		"Installer":     installation.Installer,
		"Time":          installation.ScheduledAt.Format("15:04"),
		"Date":          installation.ScheduledAt.Format("02.01.2006"),
		"Address":       installation.Address,
		"ClientContact": installation.ClientContact,
	}

	// Отправляем уведомление монтажнику через Telegram
	if installation.Installer != nil && installation.Installer.TelegramID != "" {
		err := s.SendNotification("installation_created", "telegram", installation.Installer.TelegramID,
			templateData, installation.CompanyID, installation.ID, "installation")
		if err != nil {
			log.Printf("Ошибка отправки Telegram уведомления о создании монтажа: %v", err)
		}
	}

	// Отправляем уведомление монтажнику через SMS
	if installation.Installer != nil && installation.Installer.Phone != "" {
		err := s.SendNotification("installation_created", "sms", installation.Installer.Phone,
			templateData, installation.CompanyID, installation.ID, "installation")
		if err != nil {
			log.Printf("Ошибка отправки SMS уведомления о создании монтажа: %v", err)
		}
	}

	return nil
}

// SendInstallationUpdated отправляет уведомление об обновлении монтажа
func (s *NotificationService) SendInstallationUpdated(installation *models.Installation) error {
	templateData := map[string]interface{}{
		"Installation": installation,
		"Object":       installation.Object,
		"Installer":    installation.Installer,
		"Time":         installation.ScheduledAt.Format("15:04"),
		"Date":         installation.ScheduledAt.Format("02.01.2006"),
		"Address":      installation.Address,
	}

	// Отправляем уведомление монтажнику
	if installation.Installer != nil {
		if installation.Installer.TelegramID != "" {
			err := s.SendNotification("installation_updated", "telegram", installation.Installer.TelegramID,
				templateData, installation.CompanyID, installation.ID, "installation")
			if err != nil {
				log.Printf("Ошибка отправки Telegram уведомления об обновлении монтажа: %v", err)
			}
		}

		if installation.Installer.Phone != "" {
			err := s.SendNotification("installation_updated", "sms", installation.Installer.Phone,
				templateData, installation.CompanyID, installation.ID, "installation")
			if err != nil {
				log.Printf("Ошибка отправки SMS уведомления об обновлении монтажа: %v", err)
			}
		}
	}

	return nil
}

// SendInstallationCompleted отправляет уведомление о завершении монтажа
func (s *NotificationService) SendInstallationCompleted(installation *models.Installation) error {
	templateData := map[string]interface{}{
		"Installation": installation,
		"Object":       installation.Object,
		"Installer":    installation.Installer,
	}

	// Уведомляем клиента о завершении
	if installation.ClientContact != "" {
		err := s.SendNotification("installation_completed", "sms", installation.ClientContact,
			templateData, installation.CompanyID, installation.ID, "installation")
		if err != nil {
			log.Printf("Ошибка отправки уведомления о завершении клиенту: %v", err)
		}
	}

	return nil
}

// SendInstallationCancelled отправляет уведомление об отмене монтажа
func (s *NotificationService) SendInstallationCancelled(installation *models.Installation) error {
	templateData := map[string]interface{}{
		"Installation": installation,
		"Object":       installation.Object,
		"Installer":    installation.Installer,
		"Time":         installation.ScheduledAt.Format("15:04"),
		"Date":         installation.ScheduledAt.Format("02.01.2006"),
	}

	// Уведомляем монтажника
	if installation.Installer != nil {
		if installation.Installer.TelegramID != "" {
			err := s.SendNotification("installation_cancelled", "telegram", installation.Installer.TelegramID,
				templateData, installation.CompanyID, installation.ID, "installation")
			if err != nil {
				log.Printf("Ошибка отправки Telegram уведомления об отмене монтажнику: %v", err)
			}
		}

		if installation.Installer.Phone != "" {
			err := s.SendNotification("installation_cancelled", "sms", installation.Installer.Phone,
				templateData, installation.CompanyID, installation.ID, "installation")
			if err != nil {
				log.Printf("Ошибка отправки SMS уведомления об отмене монтажнику: %v", err)
			}
		}
	}

	// Уведомляем клиента
	if installation.ClientContact != "" {
		err := s.SendNotification("installation_cancelled", "sms", installation.ClientContact,
			templateData, installation.CompanyID, installation.ID, "installation")
		if err != nil {
			log.Printf("Ошибка отправки уведомления об отмене клиенту: %v", err)
		}
	}

	return nil
}

// SendInstallationRescheduled отправляет уведомление о переносе монтажа
func (s *NotificationService) SendInstallationRescheduled(installation *models.Installation, oldScheduledAt time.Time) error {
	templateData := map[string]interface{}{
		"Installation": installation,
		"Object":       installation.Object,
		"Installer":    installation.Installer,
		"NewTime":      installation.ScheduledAt.Format("15:04"),
		"NewDate":      installation.ScheduledAt.Format("02.01.2006"),
		"OldTime":      oldScheduledAt.Format("15:04"),
		"OldDate":      oldScheduledAt.Format("02.01.2006"),
	}

	// Уведомляем монтажника
	if installation.Installer != nil {
		if installation.Installer.TelegramID != "" {
			err := s.SendNotification("installation_rescheduled", "telegram", installation.Installer.TelegramID,
				templateData, installation.CompanyID, installation.ID, "installation")
			if err != nil {
				log.Printf("Ошибка отправки Telegram уведомления о переносе монтажнику: %v", err)
			}
		}

		if installation.Installer.Phone != "" {
			err := s.SendNotification("installation_rescheduled", "sms", installation.Installer.Phone,
				templateData, installation.CompanyID, installation.ID, "installation")
			if err != nil {
				log.Printf("Ошибка отправки SMS уведомления о переносе монтажнику: %v", err)
			}
		}
	}

	// Уведомляем клиента
	if installation.ClientContact != "" {
		err := s.SendNotification("installation_rescheduled", "sms", installation.ClientContact,
			templateData, installation.CompanyID, installation.ID, "installation")
		if err != nil {
			log.Printf("Ошибка отправки уведомления о переносе клиенту: %v", err)
		}
	}

	return nil
}

// SendStockAlert отправляет уведомление о низком остатке
func (s *NotificationService) SendStockAlert(alert models.StockAlert) error {
	templateData := map[string]interface{}{
		"Alert":       alert,
		"Title":       alert.Title,
		"Description": alert.Description,
		"Severity":    alert.GetSeverityDisplayName(),
	}

	// Получаем список пользователей для уведомления (администраторы склада)
	var users []models.User
	if err := s.DB.Joins("JOIN roles ON users.role_id = roles.id").
		Where("roles.name IN ('admin', 'warehouse_manager') AND users.company_id = ?", alert.CompanyID).
		Find(&users).Error; err != nil {
		log.Printf("Ошибка при получении пользователей для уведомления: %v", err)
		return err
	}

	for _, user := range users {
		if user.TelegramID != "" {
			go s.SendNotification("stock_alert", "telegram", user.TelegramID,
				templateData, alert.CompanyID, alert.ID, "stock_alert")
		}
		if user.Email != "" {
			go s.SendNotification("stock_alert", "email", user.Email,
				templateData, alert.CompanyID, alert.ID, "stock_alert")
		}
	}

	return nil
}

// SendWarrantyAlert отправляет уведомление об истекшей гарантии
func (s *NotificationService) SendWarrantyAlert(alert models.StockAlert) error {
	templateData := map[string]interface{}{
		"Alert":       alert,
		"Title":       alert.Title,
		"Description": alert.Description,
	}

	// Получаем список пользователей для уведомления
	var users []models.User
	if err := s.DB.Joins("JOIN roles ON users.role_id = roles.id").
		Where("roles.name IN ('admin', 'warehouse_manager', 'technician') AND users.company_id = ?", alert.CompanyID).
		Find(&users).Error; err != nil {
		log.Printf("Ошибка при получении пользователей для уведомления: %v", err)
		return err
	}

	for _, user := range users {
		if user.TelegramID != "" {
			go s.SendNotification("warranty_alert", "telegram", user.TelegramID,
				templateData, alert.CompanyID, alert.ID, "warranty_alert")
		}
		if user.Email != "" {
			go s.SendNotification("warranty_alert", "email", user.Email,
				templateData, alert.CompanyID, alert.ID, "warranty_alert")
		}
	}

	return nil
}

// SendMaintenanceAlert отправляет уведомление о необходимости обслуживания
func (s *NotificationService) SendMaintenanceAlert(alert models.StockAlert) error {
	templateData := map[string]interface{}{
		"Alert":       alert,
		"Title":       alert.Title,
		"Description": alert.Description,
	}

	// Получаем список пользователей для уведомления
	var users []models.User
	if err := s.DB.Joins("JOIN roles ON users.role_id = roles.id").
		Where("roles.name IN ('admin', 'technician', 'maintenance_manager') AND users.company_id = ?", alert.CompanyID).
		Find(&users).Error; err != nil {
		log.Printf("Ошибка при получении пользователей для уведомления: %v", err)
		return err
	}

	for _, user := range users {
		if user.TelegramID != "" {
			go s.SendNotification("maintenance_alert", "telegram", user.TelegramID,
				templateData, alert.CompanyID, alert.ID, "maintenance_alert")
		}
		if user.Email != "" {
			go s.SendNotification("maintenance_alert", "email", user.Email,
				templateData, alert.CompanyID, alert.ID, "maintenance_alert")
		}
	}

	return nil
}

// SendEquipmentMovementNotification отправляет уведомление о движении оборудования
func (s *NotificationService) SendEquipmentMovementNotification(operation models.WarehouseOperation) error {
	var equipment models.Equipment
	if err := s.DB.First(&equipment, operation.EquipmentID).Error; err != nil {
		return fmt.Errorf("оборудование не найдено: %w", err)
	}

	var user models.User
	if err := s.DB.First(&user, operation.UserID).Error; err != nil {
		return fmt.Errorf("пользователь не найден: %w", err)
	}

	templateData := map[string]interface{}{
		"Operation":    operation,
		"Equipment":    equipment,
		"User":         user,
		"Type":         operation.GetTypeDisplayName(),
		"FromLocation": operation.FromLocation,
		"ToLocation":   operation.ToLocation,
	}

	// Отправляем уведомление администраторам склада
	var users []models.User
	if err := s.DB.Joins("JOIN roles ON users.role_id = roles.id").
		Where("roles.name IN ('admin', 'warehouse_manager') AND users.company_id = ?", operation.CompanyID).
		Find(&users).Error; err != nil {
		log.Printf("Ошибка при получении пользователей для уведомления: %v", err)
		return err
	}

	for _, u := range users {
		if u.TelegramID != "" {
			go s.SendNotification("equipment_movement", "telegram", u.TelegramID,
				templateData, operation.CompanyID, operation.ID, "equipment_movement")
		}
		if u.Email != "" {
			go s.SendNotification("equipment_movement", "email", u.Email,
				templateData, operation.CompanyID, operation.ID, "equipment_movement")
		}
	}

	return nil
}

// ProcessRetryNotifications обрабатывает уведомления, которые нужно повторить
func (s *NotificationService) ProcessRetryNotifications() error {
	var notifications []models.NotificationLog
	err := s.DB.Where("status = 'retry' AND next_retry_at <= ?", time.Now()).
		Preload("Template").Find(&notifications).Error
	if err != nil {
		return fmt.Errorf("ошибка получения уведомлений для повтора: %w", err)
	}

	for _, notification := range notifications {
		// Проверяем, не превышен ли лимит попыток
		if notification.AttemptCount >= notification.Template.RetryAttempts {
			notification.Status = "failed"
			notification.ErrorMessage = "Превышен лимит попыток отправки"
			s.DB.Save(&notification)
			continue
		}

		// Пытаемся отправить снова
		var err error
		switch notification.Channel {
		case "telegram":
			err = s.sendTelegramNotification(notification.Recipient, notification.Message, notification.CompanyID)
		case "email":
			err = s.sendEmailNotification(notification.Recipient, notification.Subject, notification.Message, notification.CompanyID)
		case "sms":
			err = s.sendSMSNotification(notification.Recipient, notification.Message, notification.CompanyID)
		}

		// Обновляем статус
		notification.AttemptCount++
		if err != nil {
			notification.ErrorMessage = err.Error()
			if notification.AttemptCount >= notification.Template.RetryAttempts {
				notification.Status = "failed"
			} else {
				// Планируем следующую попытку
				nextRetry := time.Now().Add(time.Duration(notification.AttemptCount*5) * time.Minute)
				notification.NextRetryAt = &nextRetry
			}
		} else {
			notification.Status = "sent"
			now := time.Now()
			notification.SentAt = &now
			notification.ErrorMessage = ""
		}

		s.DB.Save(&notification)
	}

	return nil
}

// GetNotificationLogs возвращает логи уведомлений
func (s *NotificationService) GetNotificationLogs(limit int, offset int, filters map[string]interface{}, companyID uint) ([]models.NotificationLog, int64, error) {
	query := s.DB.Model(&models.NotificationLog{}).Where("company_id = ?", companyID)

	// Применяем фильтры
	if notificationType, ok := filters["type"].(string); ok && notificationType != "" {
		query = query.Where("type = ?", notificationType)
	}
	if channel, ok := filters["channel"].(string); ok && channel != "" {
		query = query.Where("channel = ?", channel)
	}
	if status, ok := filters["status"].(string); ok && status != "" {
		query = query.Where("status = ?", status)
	}
	if relatedType, ok := filters["related_type"].(string); ok && relatedType != "" {
		query = query.Where("related_type = ?", relatedType)
	}

	// Подсчитываем общее количество
	var total int64
	query.Count(&total)

	// Получаем записи с пагинацией
	var logs []models.NotificationLog
	err := query.Preload("Template").Preload("User").
		Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error

	return logs, total, err
}

// GetNotificationStatistics возвращает статистику по уведомлениям
func (s *NotificationService) GetNotificationStatistics(companyID uint) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Общая статистика
	var total, sent, failed, pending int64
	s.DB.Model(&models.NotificationLog{}).Where("company_id = ?", companyID).Count(&total)
	s.DB.Model(&models.NotificationLog{}).Where("company_id = ? AND status = 'sent'", companyID).Count(&sent)
	s.DB.Model(&models.NotificationLog{}).Where("company_id = ? AND status = 'failed'", companyID).Count(&failed)
	s.DB.Model(&models.NotificationLog{}).Where("company_id = ? AND status = 'pending'", companyID).Count(&pending)

	stats["total"] = total
	stats["sent"] = sent
	stats["failed"] = failed
	stats["pending"] = pending

	// Статистика по каналам
	var channelStats []struct {
		Channel string `json:"channel"`
		Count   int64  `json:"count"`
	}
	s.DB.Model(&models.NotificationLog{}).Where("company_id = ?", companyID).
		Select("channel, COUNT(*) as count").
		Group("channel").
		Scan(&channelStats)

	stats["by_channel"] = channelStats

	// Статистика по типам
	var typeStats []struct {
		Type  string `json:"type"`
		Count int64  `json:"count"`
	}
	s.DB.Model(&models.NotificationLog{}).Where("company_id = ?", companyID).
		Select("type, COUNT(*) as count").
		Group("type").
		Scan(&typeStats)

	stats["by_type"] = typeStats

	return stats, nil
}

// CreateDefaultTemplates создает шаблоны по умолчанию для компании
func (s *NotificationService) CreateDefaultTemplates(companyID uint) error {
	templates := []models.NotificationTemplate{
		{
			Name:        "Напоминание о монтаже (Telegram)",
			Type:        "installation_reminder",
			Channel:     "telegram",
			Subject:     "",
			Template:    "🔧 <b>Напоминание о монтаже</b>\n\n📅 Дата: {{.Date}}\n⏰ Время: {{.Time}}\n📍 Адрес: {{.Address}}\n🏢 Объект: {{.Object.Name}}\n📞 Контакт клиента: {{.ClientContact}}",
			Description: "Напоминание монтажнику о предстоящем монтаже через Telegram",
			CompanyID:   companyID,
		},
		{
			Name:        "Напоминание о монтаже (SMS)",
			Type:        "installation_reminder",
			Channel:     "sms",
			Subject:     "",
			Template:    "Напоминание: {{.Date}} в {{.Time}} монтаж по адресу {{.Address}}. Объект: {{.Object.Name}}. Контакт: {{.ClientContact}}",
			Description: "Напоминание монтажнику о предстоящем монтаже через SMS",
			CompanyID:   companyID,
		},
		{
			Name:        "Напоминание клиенту о монтаже (SMS)",
			Type:        "installation_reminder_client",
			Channel:     "sms",
			Subject:     "",
			Template:    "Напоминание: {{.Date}} в {{.Time}} запланирован монтаж объекта \"{{.Object.Name}}\". Монтажник: {{.Installer.FirstName}} {{.Installer.LastName}}, тел. {{.Installer.Phone}}",
			Description: "Напоминание клиенту о предстоящем монтаже",
			CompanyID:   companyID,
		},
		{
			Name:        "Новый монтаж (Telegram)",
			Type:        "installation_created",
			Channel:     "telegram",
			Subject:     "",
			Template:    "🆕 <b>Новый монтаж</b>\n\n📅 Дата: {{.Date}}\n⏰ Время: {{.Time}}\n📍 Адрес: {{.Address}}\n🏢 Объект: {{.Object.Name}}\n📞 Контакт клиента: {{.ClientContact}}",
			Description: "Уведомление о создании нового монтажа через Telegram",
			CompanyID:   companyID,
		},
		{
			Name:        "Складское уведомление (Telegram)",
			Type:        "stock_alert",
			Channel:     "telegram",
			Subject:     "",
			Template:    "⚠️ <b>{{.Title}}</b>\n\n{{.Description}}\n\nУровень важности: {{.Severity}}",
			Description: "Уведомление о складских проблемах через Telegram",
			CompanyID:   companyID,
		},
		{
			Name:        "Складское уведомление (Email)",
			Type:        "stock_alert",
			Channel:     "email",
			Subject:     "Складское уведомление: {{.Title}}",
			Template:    "<h2>{{.Title}}</h2><p>{{.Description}}</p><p><b>Уровень важности:</b> {{.Severity}}</p>",
			Description: "Уведомление о складских проблемах через Email",
			CompanyID:   companyID,
		},
	}

	for _, tmpl := range templates {
		// Проверяем, не существует ли уже такой шаблон
		var existing models.NotificationTemplate
		err := s.DB.Where("type = ? AND channel = ? AND company_id = ?", tmpl.Type, tmpl.Channel, companyID).First(&existing).Error
		if err == nil {
			// Шаблон уже существует, пропускаем
			continue
		}

		// Создаем новый шаблон
		if err := s.DB.Create(&tmpl).Error; err != nil {
			log.Printf("Ошибка создания шаблона %s: %v", tmpl.Name, err)
		}
	}

	return nil
}
