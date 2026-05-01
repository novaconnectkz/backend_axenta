package services

import (
	"context"
	"errors"
	"fmt"
)

// sendMax отправляет уведомление через MaxIntegrationService (российский мессенджер).
// chatID — chat_id MAX-бота.
func (s *NotificationService) sendMax(ctx context.Context, companyID uint, chatID, body string) error {
	if s.Max == nil {
		return errors.New("MaxIntegrationService не инициализирован")
	}
	if chatID == "" {
		return errors.New("пустой chat_id")
	}

	plain := stripBasicHTML(body)

	if err := s.Max.SendMessage(ctx, companyID, chatID, plain, nil); err != nil {
		return fmt.Errorf("max send: %w", err)
	}
	return nil
}
