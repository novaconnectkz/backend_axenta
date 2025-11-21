package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SimpleTelegramSender - простой отправитель сообщений в Telegram
type SimpleTelegramSender struct{}

// SendMessage отправляет сообщение через Telegram Bot API
func (s *SimpleTelegramSender) SendMessage(botToken, chatID, message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	
	body := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "HTML",
	}
	
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("ошибка формирования запроса: %w", err)
	}
	
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка отправки запроса: %w", err)
	}
	defer resp.Body.Close()
	
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ошибка чтения ответа: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ошибка Telegram API (код %d): %s", resp.StatusCode, string(respBody))
	}
	
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("ошибка парсинга ответа: %w", err)
	}
	
	if ok, exists := result["ok"].(bool); exists && !ok {
		return fmt.Errorf("ошибка Telegram API: %v", result["description"])
	}
	
	return nil
}

// SimpleMaxSender - простой отправитель сообщений в MAX
type SimpleMaxSender struct{}

// SendMessage отправляет сообщение через MAX Bot API
func (s *SimpleMaxSender) SendMessage(botToken, chatID, message string) error {
	url := "https://botapi.max.ru/sendMessage"
	
	body := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "HTML",
	}
	
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("ошибка формирования запроса: %w", err)
	}
	
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", botToken))
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка отправки запроса: %w", err)
	}
	defer resp.Body.Close()
	
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ошибка чтения ответа: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ошибка MAX API (код %d): %s", resp.StatusCode, string(respBody))
	}
	
	return nil
}

