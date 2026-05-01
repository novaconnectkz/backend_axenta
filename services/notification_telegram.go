package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// sendTelegram отправляет уведомление через TelegramIntegrationService.
// chatID — Telegram chat_id или username. Текст рендерится без HTML-эскейпа,
// caller сам отвечает за безопасность контента.
func (s *NotificationService) sendTelegram(ctx context.Context, companyID uint, chatID, body string) error {
	if s.Telegram == nil {
		return errors.New("TelegramIntegrationService не инициализирован")
	}
	if chatID == "" {
		return errors.New("пустой chat_id")
	}

	// Telegram плохо принимает HTML с CSS. Простой fallback для уведомлений
	// из email-шаблонов: вырезаем простые теги, оставляем читаемый plain.
	plain := stripBasicHTML(body)

	if err := s.Telegram.SendMessage(ctx, companyID, chatID, plain, nil); err != nil {
		return fmt.Errorf("telegram send: %w", err)
	}
	return nil
}

// stripBasicHTML удаляет минимальные HTML-теги для отправки в plain-каналы.
func stripBasicHTML(html string) string {
	replacer := strings.NewReplacer(
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
		"</p>", "\n\n",
		"<p>", "",
		"</div>", "\n",
		"<div>", "",
		"<strong>", "*",
		"</strong>", "*",
		"<b>", "*",
		"</b>", "*",
		"&nbsp;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
	)
	return strings.TrimSpace(replacer.Replace(html))
}
