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
// Поддерживаемые каналы: email (SMTP), telegram, max (MAX мессенджер).
type NotificationService struct {
	DB       *gorm.DB
	Cache    *CacheService
	Telegram *TelegramIntegrationService
	Max      *MaxIntegrationService
	Logger   *log.Logger
}

// NewNotificationService создаёт сервис уведомлений с зависимостями
// от существующих интеграционных сервисов. Если telegram/max == nil,
// соответствующие каналы вернут ошибку при отправке.
func NewNotificationService(db *gorm.DB, cache *CacheService, telegram *TelegramIntegrationService, max *MaxIntegrationService) *NotificationService {
	return &NotificationService{
		DB:       db,
		Cache:    cache,
		Telegram: telegram,
		Max:      max,
		Logger:   log.Default(),
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

	case "telegram":
		if !settings.TelegramEnabled {
			return fmt.Errorf("telegram канал отключён для компании %d", companyID)
		}
		if err := s.sendTelegram(context.Background(), companyID, recipient, body); err != nil {
			logEntry.status = "failed"
			logEntry.errorMessage = err.Error()
			writeNotificationLog(s.DB, logEntry)
			return err
		}
		logEntry.status = "sent"
		writeNotificationLog(s.DB, logEntry)
		return nil

	case "max":
		if !settings.MaxEnabled {
			return fmt.Errorf("max канал отключён для компании %d", companyID)
		}
		if err := s.sendMax(context.Background(), companyID, recipient, body); err != nil {
			logEntry.status = "failed"
			logEntry.errorMessage = err.Error()
			writeNotificationLog(s.DB, logEntry)
			return err
		}
		logEntry.status = "sent"
		writeNotificationLog(s.DB, logEntry)
		return nil

	case "sms":
		// SMS — отдельный провайдер (settings.SMSProvider). Phase 4.
		s.Logger.Printf("⚠️ NotificationService: SMS канал не реализован. Пропускаем %s для company=%d", notificationType, companyID)
		logEntry.status = "pending"
		logEntry.errorMessage = "sms channel not implemented yet"
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

	// Fallback на встроенный builtin-шаблон (без БД).
	if builtin := findBuiltinTemplate(notifType, channel); builtin != nil {
		return builtin, nil
	}

	return nil, fmt.Errorf("шаблон '%s' для канала '%s' не найден (ни для company_id=%d, ни global, ни builtin)", notifType, channel, companyID)
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
// Convenience methods для Installation*. Phase 2: реальная отправка
// монтажнику на email + telegram (если включены и заполнены контакты).
//
// companyID нужен для определения tenant-настроек (NotificationSettings
// в public schema). Caller обязан передать актуальный companyID.
// =====================================================================

// sendToInstaller отправляет одно уведомление монтажнику по всем
// активным каналам компании, для которых у монтажника заполнены контакты.
func (s *NotificationService) sendToInstaller(installation *models.Installation, companyID uint, notifType string, extraData map[string]interface{}) error {
	if installation == nil {
		return errors.New("nil installation")
	}

	installer := installation.Installer
	if installer == nil {
		var loaded models.Installer
		if err := s.DB.First(&loaded, installation.InstallerID).Error; err != nil {
			return fmt.Errorf("монтажник %d не найден: %w", installation.InstallerID, err)
		}
		installer = &loaded
	}

	settings, err := s.GetNotificationSettings(companyID)
	if err != nil {
		return fmt.Errorf("настройки уведомлений: %w", err)
	}

	data := buildInstallationData(installation, installer)
	for k, v := range extraData {
		data[k] = v
	}

	var errs []string

	if settings.EmailEnabled && installer.Email != "" {
		if err := s.SendNotification(notifType, "email", installer.Email, data, companyID, installation.ID, "installation"); err != nil {
			errs = append(errs, fmt.Sprintf("email: %v", err))
		}
	}

	if settings.TelegramEnabled && installer.TelegramID != "" {
		if err := s.SendNotification(notifType, "telegram", installer.TelegramID, data, companyID, installation.ID, "installation"); err != nil {
			errs = append(errs, fmt.Sprintf("telegram: %v", err))
		}
	}

	if len(errs) > 0 {
		return errors.New("частичные ошибки отправки: " + strings.Join(errs, "; "))
	}
	return nil
}

// buildInstallationData собирает переменные шаблона из объекта монтажа.
func buildInstallationData(i *models.Installation, installer *models.Installer) map[string]interface{} {
	data := map[string]interface{}{
		"installation_id":    i.ID,
		"installation_type":  i.Type,
		"status":             i.Status,
		"priority":           i.Priority,
		"description":        i.Description,
		"scheduled_at":       i.ScheduledAt.Format("2006-01-02 15:04"),
		"estimated_duration": i.EstimatedDuration,
		"address":            i.Address,
		"client_contact":     i.ClientContact,
		"notes":              i.Notes,
	}
	if installer != nil {
		data["installer_first_name"] = installer.FirstName
		data["installer_last_name"] = installer.LastName
		data["installer_full_name"] = strings.TrimSpace(installer.FirstName + " " + installer.LastName)
		data["installer_phone"] = installer.Phone
	}
	return data
}

func (s *NotificationService) SendInstallationReminder(installation *models.Installation, companyID uint) error {
	return s.sendToInstaller(installation, companyID, "installation_reminder", nil)
}

func (s *NotificationService) SendInstallationCreated(installation *models.Installation, companyID uint) error {
	return s.sendToInstaller(installation, companyID, "installation_created", nil)
}

func (s *NotificationService) SendInstallationUpdated(installation *models.Installation, companyID uint) error {
	return s.sendToInstaller(installation, companyID, "installation_updated", nil)
}

func (s *NotificationService) SendInstallationCompleted(installation *models.Installation, companyID uint) error {
	return s.sendToInstaller(installation, companyID, "installation_completed", nil)
}

func (s *NotificationService) SendInstallationCancelled(installation *models.Installation, companyID uint) error {
	return s.sendToInstaller(installation, companyID, "installation_cancelled", nil)
}

func (s *NotificationService) SendInstallationRescheduled(installation *models.Installation, companyID uint, oldScheduledAt time.Time) error {
	return s.sendToInstaller(installation, companyID, "installation_rescheduled", map[string]interface{}{
		"old_scheduled_at": oldScheduledAt.Format("2006-01-02 15:04"),
	})
}

// =====================================================================
// Прочие convenience-методы. Резолв recipient (admin компании,
// ответственный за биллинг/склад) — Phase 3.
// =====================================================================

func (s *NotificationService) SendBillingAlert(companyID uint, alertType string, message string) error {
	s.Logger.Printf("ℹ️ BillingAlert (Phase 3): company=%d type=%s msg=%q", companyID, alertType, message)
	return nil
}

func (s *NotificationService) SendWarehouseAlert(companyID uint, alertType string, message string) error {
	s.Logger.Printf("ℹ️ WarehouseAlert (Phase 3): company=%d type=%s msg=%q", companyID, alertType, message)
	return nil
}

func (s *NotificationService) SendStockAlert(alert models.StockAlert) error {
	s.Logger.Printf("ℹ️ StockAlert (Phase 3): id=%d", alert.ID)
	return nil
}

func (s *NotificationService) SendWarrantyAlert(alert models.StockAlert) error {
	s.Logger.Printf("ℹ️ WarrantyAlert (Phase 3): id=%d", alert.ID)
	return nil
}

func (s *NotificationService) SendMaintenanceAlert(alert models.StockAlert) error {
	s.Logger.Printf("ℹ️ MaintenanceAlert (Phase 3): id=%d", alert.ID)
	return nil
}

func (s *NotificationService) SendEquipmentMovementNotification(operation models.WarehouseOperation) error {
	s.Logger.Printf("ℹ️ EquipmentMovement (Phase 3): operation=%d", operation.ID)
	return nil
}

// ProcessRetryNotifications выполняет одну итерацию retry-цикла —
// выбирает пакет failed/retry/pending записей чей next_retry_at прошёл
// и пытается их переотправить. Используется внутри StartRetryWorker и
// может быть вызвана вручную (например, из admin endpoint).
func (s *NotificationService) ProcessRetryNotifications() error {
	return s.processRetryBatch()
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

// CreateDefaultTemplates записывает в БД набор стандартных builtin-шаблонов
// для компании (installation_*, и т.д.). Идемпотентно — пропускает уже
// существующие записи по Name+CompanyID.
func (s *NotificationService) CreateDefaultTemplates(companyID uint) error {
	return s.seedBuiltinTemplates(companyID)
}

