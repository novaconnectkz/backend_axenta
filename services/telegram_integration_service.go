package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"backend_axenta/models"

	"gorm.io/gorm"
)

// TelegramIntegrationService сервис для интеграции с Telegram Bot
type TelegramIntegrationService struct {
	db     *gorm.DB
	logger *log.Logger
}

// TelegramIntegrationConfig конфигурация интеграции с Telegram
type TelegramIntegrationConfig struct {
	CompanyID            uint   `json:"company_id"`
	BotToken             string `json:"bot_token"`              // Токен бота от BotFather
	DefaultChatID        string `json:"default_chat_id"`         // ID чата по умолчанию (опционально)
	ParseMode            string `json:"parse_mode"`              // HTML, Markdown, MarkdownV2
	DisableNotifications bool   `json:"disable_notifications"`  // Отключить уведомления
	QuietHoursStart      string `json:"quiet_hours_start"`       // Начало тихих часов (HH:mm)
	QuietHoursEnd        string `json:"quiet_hours_end"`         // Конец тихих часов (HH:mm)
	QuietHoursEnabled    bool   `json:"quiet_hours_enabled"`     // Включены ли тихие часы
}

// TelegramMessage структура для отправки сообщения
type TelegramMessage struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview,omitempty"`
	DisableNotification   bool   `json:"disable_notification,omitempty"`
}

// TelegramAPIResponse ответ от Telegram API
type TelegramAPIResponse struct {
	OK          bool                   `json:"ok"`
	Result      map[string]interface{} `json:"result,omitempty"`
	ErrorCode   int                    `json:"error_code,omitempty"`
	Description string                 `json:"description,omitempty"`
}

// NewTelegramIntegrationService создает новый сервис интеграции с Telegram
func NewTelegramIntegrationService(db *gorm.DB, logger *log.Logger) *TelegramIntegrationService {
	if logger == nil {
		logger = log.New(log.Writer(), "[Telegram] ", log.LstdFlags|log.Lshortfile)
	}
	return &TelegramIntegrationService{
		db:     db,
		logger: logger,
	}
}

// GetConfig получает конфигурацию интеграции
func (s *TelegramIntegrationService) GetConfig(ctx context.Context, companyID uint) (*TelegramIntegrationConfig, error) {
	var integration models.Integration
	if err := s.db.WithContext(ctx).Where("company_id = ? AND integration_type = ?", companyID, "telegram").First(&integration).Error; err != nil {
		return nil, fmt.Errorf("интеграция с Telegram не настроена: %w", err)
	}

	var config TelegramIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		return nil, fmt.Errorf("ошибка парсинга настроек Telegram: %w", err)
	}

	return &config, nil
}

// SaveConfig сохраняет конфигурацию интеграции
func (s *TelegramIntegrationService) SaveConfig(ctx context.Context, config *TelegramIntegrationConfig) error {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("ошибка сериализации конфигурации: %w", err)
	}

	var integration models.Integration
	err = s.db.WithContext(ctx).Where("company_id = ? AND integration_type = ?", config.CompanyID, "telegram").First(&integration).Error

	if err == gorm.ErrRecordNotFound {
		// Создаем новую интеграцию
		integration = models.Integration{
			CompanyID:       config.CompanyID,
			IntegrationType: "telegram",
			Name:            "Telegram Bot",
			Description:     "Интеграция с Telegram для отправки уведомлений пользователям",
			Settings:        string(configJSON),
			IsActive:         true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		if err := s.db.WithContext(ctx).Create(&integration).Error; err != nil {
			return fmt.Errorf("ошибка создания интеграции: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("ошибка проверки существующей интеграции: %w", err)
	} else {
		// Обновляем существующую интеграцию
		integration.Settings = string(configJSON)
		integration.UpdatedAt = time.Now()
		if err := s.db.WithContext(ctx).Save(&integration).Error; err != nil {
			return fmt.Errorf("ошибка обновления интеграции: %w", err)
		}
	}

	return nil
}

// SendMessage отправляет сообщение через Telegram Bot API
func (s *TelegramIntegrationService) SendMessage(ctx context.Context, companyID uint, chatID string, text string, options map[string]interface{}) error {
	config, err := s.GetConfig(ctx, companyID)
	if err != nil {
		return err
	}

	if config.BotToken == "" {
		return fmt.Errorf("токен бота не настроен")
	}

	// Проверяем тихие часы
	if config.QuietHoursEnabled && s.isQuietHours(config) {
		s.logger.Printf("Пропуск отправки сообщения из-за тихих часов (компания: %d)", companyID)
		return nil // Не возвращаем ошибку, просто пропускаем
	}

	// Используем chatID из параметров или из конфигурации
	if chatID == "" {
		chatID = config.DefaultChatID
	}
	if chatID == "" {
		return fmt.Errorf("chat_id не указан")
	}

	// Формируем сообщение
	message := TelegramMessage{
		ChatID:                chatID,
		Text:                  text,
		ParseMode:             config.ParseMode,
		DisableWebPagePreview: false,
		DisableNotification:   config.DisableNotifications,
	}

	// Применяем дополнительные опции
	if parseMode, ok := options["parse_mode"].(string); ok && parseMode != "" {
		message.ParseMode = parseMode
	}
	if disableNotification, ok := options["disable_notification"].(bool); ok {
		message.DisableNotification = disableNotification
	}
	if disablePreview, ok := options["disable_web_page_preview"].(bool); ok {
		message.DisableWebPagePreview = disablePreview
	}

	// Отправляем через Telegram Bot API
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.BotToken)

	messageJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("ошибка сериализации сообщения: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(messageJSON))
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка отправки запроса к Telegram API: %w", err)
	}
	defer resp.Body.Close()

	var apiResp TelegramAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("ошибка парсинга ответа от Telegram API: %w", err)
	}

	if !apiResp.OK {
		return fmt.Errorf("ошибка Telegram API: %s (код: %d)", apiResp.Description, apiResp.ErrorCode)
	}

	// Обновляем статистику интеграции
	var integration models.Integration
	if err := s.db.WithContext(ctx).Where("company_id = ? AND integration_type = ?", companyID, "telegram").First(&integration).Error; err == nil {
		integration.UpdateStats(true, "")
		s.db.WithContext(ctx).Save(&integration)
	}

	s.logger.Printf("Сообщение успешно отправлено в Telegram (компания: %d, chat_id: %s)", companyID, chatID)
	return nil
}

// TestConnection тестирует подключение к Telegram Bot API
func (s *TelegramIntegrationService) TestConnection(ctx context.Context, companyID uint) error {
	config, err := s.GetConfig(ctx, companyID)
	if err != nil {
		return err
	}

	if config.BotToken == "" {
		return fmt.Errorf("токен бота не настроен")
	}

	// Проверяем токен через метод getMe
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", config.BotToken)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка отправки запроса к Telegram API: %w", err)
	}
	defer resp.Body.Close()

	var apiResp TelegramAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("ошибка парсинга ответа от Telegram API: %w", err)
	}

	if !apiResp.OK {
		return fmt.Errorf("неверный токен бота: %s (код: %d)", apiResp.Description, apiResp.ErrorCode)
	}

	s.logger.Printf("Тест подключения к Telegram успешно пройден (компания: %d)", companyID)
	return nil
}

// isQuietHours проверяет, находятся ли мы в тихих часах
func (s *TelegramIntegrationService) isQuietHours(config *TelegramIntegrationConfig) bool {
	if !config.QuietHoursEnabled || config.QuietHoursStart == "" || config.QuietHoursEnd == "" {
		return false
	}

	now := time.Now()
	loc := now.Location()

	// Парсим время начала и конца тихих часов
	startTime, err := time.ParseInLocation("15:04", config.QuietHoursStart, loc)
	if err != nil {
		return false
	}
	endTime, err := time.ParseInLocation("15:04", config.QuietHoursEnd, loc)
	if err != nil {
		return false
	}

	// Нормализуем время до сегодняшней даты
	startTime = time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), 0, 0, loc)
	endTime = time.Date(now.Year(), now.Month(), now.Day(), endTime.Hour(), endTime.Minute(), 0, 0, loc)

	// Если тихие часы переходят через полночь
	if endTime.Before(startTime) || endTime.Equal(startTime) {
		// Тихие часы переходят через полночь
		return now.After(startTime) || now.Before(endTime)
	}

	// Обычный случай: тихие часы в пределах одного дня
	return now.After(startTime) && now.Before(endTime)
}

// GetIntegrationStatus получает статус интеграции
func (s *TelegramIntegrationService) GetIntegrationStatus(ctx context.Context, companyID uint) (map[string]interface{}, error) {
	var integration models.Integration
	if err := s.db.WithContext(ctx).Where("company_id = ? AND integration_type = ?", companyID, "telegram").First(&integration).Error; err != nil {
		return map[string]interface{}{
			"configured":    false,
			"active":        false,
			"connection_ok": false,
		}, nil
	}

	// Тестируем подключение
	connectionOK := false
	if err := s.TestConnection(ctx, companyID); err == nil {
		connectionOK = true
	}

	return map[string]interface{}{
		"configured":    true,
		"active":        integration.IsActive,
		"last_sync":     integration.LastSyncAt,
		"errors_count":  integration.ErrorCount,
		"connection_ok": connectionOK,
		"created_at":    integration.CreatedAt,
		"updated_at":    integration.UpdatedAt,
	}, nil
}

