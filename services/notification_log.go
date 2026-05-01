package services

import (
	"time"

	"gorm.io/gorm"

	"backend_axenta/models"
)

// notificationLogEntry — параметры для создания записи в NotificationLog
type notificationLogEntry struct {
	companyID    uint
	notifType    string
	channel      string
	recipient    string
	subject      string
	message      string
	relatedID    uint
	relatedType  string
	templateID   uint
	externalID   string
	status       string // "sent", "failed", "pending"
	errorMessage string
	attemptCount int
}

// writeNotificationLog сохраняет запись об отправке в БД.
// Не блокирует caller-а ошибкой записи лога — только логирует в stdout.
func writeNotificationLog(db *gorm.DB, entry notificationLogEntry) {
	if db == nil {
		return
	}

	now := time.Now()

	logRec := models.NotificationLog{
		Type:         entry.notifType,
		Channel:      entry.channel,
		Recipient:    entry.recipient,
		Subject:      entry.subject,
		Message:      entry.message,
		Status:       entry.status,
		ErrorMessage: entry.errorMessage,
		AttemptCount: entry.attemptCount,
		CompanyID:    entry.companyID,
		RelatedType:  entry.relatedType,
		ExternalID:   entry.externalID,
	}

	if entry.relatedID != 0 {
		id := entry.relatedID
		logRec.RelatedID = &id
	}

	if entry.templateID != 0 {
		tid := entry.templateID
		logRec.TemplateID = &tid
	}

	if entry.status == "sent" {
		logRec.SentAt = &now
	}

	if err := db.Create(&logRec).Error; err != nil {
		// Не падаем — отправка уже произошла, лог только для аудита
		_ = err
	}
}
