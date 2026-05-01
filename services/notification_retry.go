package services

import (
	"context"
	"strings"
	"time"

	"backend_axenta/models"
)

const (
	// retryBatchSize — сколько записей берём за один тик.
	retryBatchSize = 50
	// retryDefaultInterval — интервал тика по умолчанию.
	retryDefaultInterval = 60 * time.Second
	// defaultMaxAttempts если в settings не задано.
	defaultMaxAttempts = 3
	// defaultRetryDelayMinutes если в settings не задано.
	defaultRetryDelayMinutes = 5
)

// StartRetryWorker запускает фоновый цикл, повторно отправляющий
// failed/pending записи NotificationLog. Останавливается при ctx.Done().
//
// interval=0 → используется retryDefaultInterval.
func (s *NotificationService) StartRetryWorker(ctx context.Context, interval time.Duration) {
	if interval == 0 {
		interval = retryDefaultInterval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		s.Logger.Printf("✅ NotificationService retry worker started (interval=%s)", interval)
		for {
			select {
			case <-ctx.Done():
				s.Logger.Printf("ℹ️ NotificationService retry worker stopped")
				return
			case <-ticker.C:
				if err := s.processRetryBatch(); err != nil {
					s.Logger.Printf("⚠️ retry batch: %v", err)
				}
			}
		}
	}()
}

// processRetryBatch — одна итерация: выбрать пакет ready-to-retry
// записей, попытаться отправить каждую.
func (s *NotificationService) processRetryBatch() error {
	if s.DB == nil {
		return nil
	}

	var records []models.NotificationLog
	err := s.DB.Where(`
		status IN ('failed', 'retry', 'pending')
		AND (next_retry_at IS NULL OR next_retry_at <= ?)
	`, time.Now()).
		Order("created_at ASC").
		Limit(retryBatchSize).
		Find(&records).Error
	if err != nil {
		return err
	}

	for i := range records {
		s.retryOne(&records[i])
	}
	return nil
}

// retryOne пытается переотправить одну запись. Обновляет status,
// attempt_count, next_retry_at, sent_at, error_message в БД.
func (s *NotificationService) retryOne(rec *models.NotificationLog) {
	settings, err := s.GetNotificationSettings(rec.CompanyID)
	if err != nil {
		s.Logger.Printf("⚠️ retry: settings для company=%d недоступны: %v", rec.CompanyID, err)
		return
	}

	maxAttempts := settings.MaxRetryAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}

	// Превышен лимит попыток — фиксируем final и выходим.
	if rec.AttemptCount >= maxAttempts {
		rec.Status = "failed_final"
		rec.NextRetryAt = nil
		s.DB.Save(rec)
		return
	}

	// Пытаемся отправить через исходный канал, повторно используя
	// сохранённые subject/message (повторная рендеринг шаблона не нужна).
	var sendErr error
	switch strings.ToLower(rec.Channel) {
	case "email":
		sendErr = sendEmail(settings, rec.Recipient, rec.Subject, rec.Message)
	case "telegram":
		sendErr = s.sendTelegram(context.Background(), rec.CompanyID, rec.Recipient, rec.Message)
	case "max":
		sendErr = s.sendMax(context.Background(), rec.CompanyID, rec.Recipient, rec.Message)
	default:
		// Неподдерживаемый канал — фиксируем как final, чтобы не крутить
		rec.Status = "failed_final"
		rec.ErrorMessage = "unsupported channel: " + rec.Channel
		rec.NextRetryAt = nil
		s.DB.Save(rec)
		return
	}

	rec.AttemptCount++

	if sendErr == nil {
		now := time.Now()
		rec.Status = "sent"
		rec.SentAt = &now
		rec.ErrorMessage = ""
		rec.NextRetryAt = nil
		s.DB.Save(rec)
		return
	}

	// Ошибка
	rec.ErrorMessage = sendErr.Error()
	if rec.AttemptCount >= maxAttempts {
		rec.Status = "failed_final"
		rec.NextRetryAt = nil
		s.DB.Save(rec)
		return
	}

	// Exponential backoff: base_delay * 2^(attempt-1)
	delayMin := settings.RetryDelayMinutes
	if delayMin <= 0 {
		delayMin = defaultRetryDelayMinutes
	}
	delay := time.Duration(delayMin) * time.Minute * time.Duration(1<<uint(rec.AttemptCount-1))
	next := time.Now().Add(delay)
	rec.Status = "retry"
	rec.NextRetryAt = &next
	s.DB.Save(rec)
}
