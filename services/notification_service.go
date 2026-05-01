package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"backend_axenta/models"
)

// NotificationService сервис отправки уведомлений по нескольким каналам.
// Phase 1: email канал работает реально, остальные методы — заглушки до Phase 2.
type NotificationService struct {
	DB     *gorm.DB
	Cache  *CacheService
	Logger *log.Logger
}

// NewNotificationService создаёт сервис уведомлений.
// Сигнатура совместима с текущим вызовом из main.go (db, cache).
func NewNotificationService(db *gorm.DB, cache *CacheService) *NotificationService {
	return &NotificationService{
		DB:     db,
		Cache:  cache,
		Logger: log.Default(),
	}
}

// SendNotification — generic точка входа: рендерит шаблон по type+channel и отправляет.
//   - notificationType: ключ шаблона (например "installation_created", "billing_alert").
//   - channel: "email" / "telegram" / "max" / "sms".
//   - recipient: email-адрес / chat_id / номер.
//   - templateData: переменные для рендеринга шаблона.
//   - companyID: компания (для tenant-настроек и логов).
//   - relatedID/relatedType: ссылка на связанную сущность (для аудита).
//
// Phase 1: реально отправляется только email. Остальные каналы — лог + nil.
func (s *NotificationService) SendNotification(notificationType, channel, recipient string, templateData map[string]interface{}, companyID, relatedID uint, relatedType string) error {
	if recipient == "" {
		return errors.New("пустой recipient")
	}

	settings, err := s.GetNotificationSettings(companyID)
	if err != nil {
		return fmt.Errorf("настройки уведомлений недоступны: %w", err)
	}

	tmpl, err := s.findTemplate(notificationType, channel, companyID)
	if err != nil {
		return fmt.Errorf("шаблон не найден: %w", err)
	}

	subject, body, err := renderTemplate(tmpl, templateData)
	if err != nil {
		return fmt.Errorf("ошибка рендеринга: %w", err)
	}

	tmplID := uint(0)
	if tmpl != nil {
		tmplID = tmpl.ID
	}

	logEntry := notificationLogEntry{
		companyID:    companyID,
		notifType:    notificationType,
		channel:      channel,
		recipient:    recipient,
		subject:      subject,
		message:      body,
		relatedID:    relatedID,
		relatedType:  relatedType,
		templateID:   tmplID,
		attemptCount: 1,
	}

	switch strings.ToLower(channel) {
	case "email":
		if err := sendEmail(settings, recipient, subject, body); err != nil {
			logEntry.status = "failed"
			logEntry.errorMessage = err.Error()
			writeNotificationLog(s.DB, logEntry)
			return err
		}
		logEntry.status = "sent"
		writeNotificationLog(s.DB, logEntry)
		return nil

	case "telegram", "max", "sms":
		// Phase 2: реальная отправка через TelegramIntegrationService / MaxIntegrationService
		s.Logger.Printf("⚠️ NotificationService: канал %s не реализован (Phase 2). Пропускаем отправку %s для company=%d", channel, notificationType, companyID)
		logEntry.status = "pending"
		logEntry.errorMessage = "channel not implemented yet"
		writeNotificationLog(s.DB, logEntry)
		return nil

	default:
		return fmt.Errorf("неизвестный канал: %s", channel)
	}
}

// GetNotificationSettings возвращает настройки уведомлений компании.
// Phase 1: прямое чтение из БД, без cache layer.
func (s *NotificationService) GetNotificationSettings(companyID uint) (*models.NotificationSettings, error) {
	if s.DB == nil {
		return nil, errors.New("БД не подключена")
	}

	var settings models.NotificationSettings
	err := s.DB.Where("company_id = ?", companyID).First(&settings).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Дефолтные настройки — всё отключено
		return &models.NotificationSettings{
			CompanyID:        companyID,
			EmailEnabled:     false,
			TelegramEnabled:  false,
			SMSEnabled:       false,
			MaxEnabled:       false,
			SMTPPort:         587,
			SMTPUseTLS:       true,
			DefaultLanguage:  "ru",
			MaxRetryAttempts: 3,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

// findTemplate ищет NotificationTemplate в БД по type+channel.
// Сначала ищет company-specific (company_id = companyID), потом дефолтный (company_id = 0).
func (s *NotificationService) findTemplate(notifType, channel string, companyID uint) (*models.NotificationTemplate, error) {
	if s.DB == nil {
		return nil, errors.New("БД не подключена")
	}

	var tmpl models.NotificationTemplate
	err := s.DB.Where("type = ? AND channel = ? AND is_active = ? AND company_id = ?", notifType, channel, true, companyID).First(&tmpl).Error
	if err == nil {
		return &tmpl, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Fallback на глобальный (company_id = 0)
	err = s.DB.Where("type = ? AND channel = ? AND is_active = ? AND company_id = ?", notifType, channel, true, 0).First(&tmpl).Error
	if err == nil {
		return &tmpl, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	return nil, fmt.Errorf("шаблон '%s' для канала '%s' не найден (ни для company_id=%d ни как default)", notifType, channel, companyID)
}

// renderTemplate рендерит subject и body шаблона через text/html template.
func renderTemplate(tmpl *models.NotificationTemplate, data map[string]interface{}) (subject, body string, err error) {
	if tmpl == nil {
		return "", "", errors.New("nil template")
	}

	if tmpl.Subject != "" {
		subject, err = renderString(tmpl.Subject, data)
		if err != nil {
			return "", "", fmt.Errorf("subject render: %w", err)
		}
	}

	body, err = renderString(tmpl.Template, data)
	if err != nil {
		return "", "", fmt.Errorf("body render: %w", err)
	}

	return subject, body, nil
}

func renderString(tmpl string, data map[string]interface{}) (string, error) {
	t, err := template.New("notif").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// =====================================================================
// Convenience methods — Phase 1 stub-уровень. Phase 2 заполнит реальной
// логикой (resolve recipient → channel(s) → SendNotification).
// =====================================================================

func (s *NotificationService) SendInstallationReminder(installation *models.Installation) error {
	return nil
}

func (s *NotificationService) SendInstallationCreated(installation *models.Installation) error {
	return nil
}

func (s *NotificationService) SendInstallationUpdated(installation *models.Installation) error {
	return nil
}

func (s *NotificationService) SendInstallationCompleted(installation *models.Installation) error {
	return nil
}

func (s *NotificationService) SendInstallationCancelled(installation *models.Installation) error {
	return nil
}

func (s *NotificationService) SendInstallationRescheduled(installation *models.Installation, oldScheduledAt time.Time) error {
	return nil
}

func (s *NotificationService) SendBillingAlert(companyID uint, alertType string, message string) error {
	return nil
}

func (s *NotificationService) SendWarehouseAlert(companyID uint, alertType string, message string) error {
	return nil
}

func (s *NotificationService) SendStockAlert(alert models.StockAlert) error {
	return nil
}

func (s *NotificationService) SendWarrantyAlert(alert models.StockAlert) error {
	return nil
}

func (s *NotificationService) SendMaintenanceAlert(alert models.StockAlert) error {
	return nil
}

func (s *NotificationService) SendEquipmentMovementNotification(operation models.WarehouseOperation) error {
	return nil
}

// ProcessRetryNotifications перезапустит отправку для NotificationLog со статусом
// "pending" / "retry" чей next_retry_at прошёл. Phase 4.
func (s *NotificationService) ProcessRetryNotifications() error {
	return nil
}

// GetNotificationLogs читает лог-записи для админ-API. Phase 1: реализовано,
// фильтры пока минимальны.
func (s *NotificationService) GetNotificationLogs(limit int, offset int, filters map[string]interface{}, companyID uint) ([]models.NotificationLog, int64, error) {
	if s.DB == nil {
		return nil, 0, errors.New("БД не подключена")
	}

	q := s.DB.Model(&models.NotificationLog{}).Where("company_id = ?", companyID)
	for k, v := range filters {
		// Безопасный whitelist полей фильтрации
		switch k {
		case "channel", "status", "type", "related_type":
			q = q.Where(k+" = ?", v)
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 50
	}
	var logs []models.NotificationLog
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// GetNotificationStatistics — сводка по каналам и статусам для дашборда. Phase 1: базовая.
func (s *NotificationService) GetNotificationStatistics(companyID uint) (map[string]interface{}, error) {
	if s.DB == nil {
		return nil, errors.New("БД не подключена")
	}

	type row struct {
		Channel string
		Status  string
		Count   int64
	}

	var rows []row
	err := s.DB.Model(&models.NotificationLog{}).
		Select("channel, status, COUNT(*) as count").
		Where("company_id = ?", companyID).
		Group("channel, status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"by_channel_status": rows,
	}
	return result, nil
}

// CreateDefaultTemplates создаёт стандартный набор шаблонов для компании.
// Phase 3 — добавит набор реальных шаблонов. Phase 1: пусто.
func (s *NotificationService) CreateDefaultTemplates(companyID uint) error {
	return nil
}

// _ ensures context import used (unused in Phase 1 but reserved for Phase 2 telegram/max calls)
var _ = context.Background
