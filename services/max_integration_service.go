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

// MaxIntegrationService сервис для интеграции с MAX Messenger Bot
type MaxIntegrationService struct {
	db     *gorm.DB
	logger *log.Logger
}

// MaxIntegrationConfig конфигурация интеграции с MAX
type MaxIntegrationConfig struct {
	CompanyID  uint   `json:"company_id"`
	BotToken   string `json:"bot_token"`   // Токен бота от @MasterBot в MAX
	ParseMode  string `json:"parse_mode"`  // HTML или Markdown
	WebhookURL string `json:"webhook_url"` // URL для webhook
	UsePolling bool   `json:"use_polling"` // Использовать polling вместо webhook
}

// MaxMessage структура для отправки сообщения через MAX
type MaxMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

// MaxAPIResponse ответ от MAX API
type MaxAPIResponse struct {
	OK          bool                   `json:"ok"`
	Result      map[string]interface{} `json:"result,omitempty"`
	ErrorCode   int                    `json:"error_code,omitempty"`
	Description string                 `json:"description,omitempty"`
}

// NewMaxIntegrationService создает новый сервис интеграции с MAX
func NewMaxIntegrationService(db *gorm.DB, logger *log.Logger) *MaxIntegrationService {
	if logger == nil {
		logger = log.New(log.Writer(), "[MAX] ", log.LstdFlags|log.Lshortfile)
	}
	return &MaxIntegrationService{
		db:     db,
		logger: logger,
	}
}

// GetConfig получает конфигурацию интеграции
func (s *MaxIntegrationService) GetConfig(ctx context.Context, companyID uint) (*MaxIntegrationConfig, error) {
	var integration models.Integration
	if err := s.db.WithContext(ctx).Where("company_id = ? AND integration_type = ?", companyID, "max").First(&integration).Error; err != nil {
		return nil, fmt.Errorf("интеграция с MAX не настроена: %w", err)
	}

	var config MaxIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		return nil, fmt.Errorf("ошибка парсинга настроек MAX: %w", err)
	}

	return &config, nil
}

// SaveConfig сохраняет конфигурацию интеграции
func (s *MaxIntegrationService) SaveConfig(ctx context.Context, config *MaxIntegrationConfig) error {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("ошибка сериализации конфигурации: %w", err)
	}

	var integration models.Integration
	err = s.db.WithContext(ctx).Where("company_id = ? AND integration_type = ?", config.CompanyID, "max").First(&integration).Error

	if err == gorm.ErrRecordNotFound {
		// Создаем новую интеграцию
		integration = models.Integration{
			CompanyID:       config.CompanyID,
			IntegrationType: "max",
			Name:            "MAX Messenger",
			Description:     "Российский мессенджер для отправки уведомлений пользователям через MAX Bot API",
			Settings:        string(configJSON),
			IsActive:        true,
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

// SendMessage отправляет сообщение через MAX Bot API
func (s *MaxIntegrationService) SendMessage(ctx context.Context, companyID uint, chatID string, text string, options map[string]interface{}) error {
	config, err := s.GetConfig(ctx, companyID)
	if err != nil {
		return err
	}

	if config.BotToken == "" {
		return fmt.Errorf("токен бота не настроен")
	}

	if chatID == "" {
		return fmt.Errorf("chat_id не указан")
	}

	// Формируем сообщение
	message := MaxMessage{
		ChatID:    chatID,
		Text:      text,
		ParseMode: config.ParseMode,
	}

	// Применяем дополнительные опции
	if parseMode, ok := options["parse_mode"].(string); ok && parseMode != "" {
		message.ParseMode = parseMode
	}

	// Отправляем через MAX Bot API
	// Примечание: URL API для MAX может отличаться от Telegram
	apiURL := fmt.Sprintf("https://api.max.ru/bot%s/sendMessage", config.BotToken)

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
		return fmt.Errorf("ошибка отправки запроса к MAX API: %w", err)
	}
	defer resp.Body.Close()

	var apiResp MaxAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("ошибка парсинга ответа от MAX API: %w", err)
	}

	if !apiResp.OK {
		return fmt.Errorf("ошибка MAX API: %s (код: %d)", apiResp.Description, apiResp.ErrorCode)
	}

	// Обновляем статистику интеграции
	var integration models.Integration
	if err := s.db.WithContext(ctx).Where("company_id = ? AND integration_type = ?", companyID, "max").First(&integration).Error; err == nil {
		integration.UpdateStats(true, "")
		s.db.WithContext(ctx).Save(&integration)
	}

	s.logger.Printf("Сообщение успешно отправлено в MAX (компания: %d, chat_id: %s)", companyID, chatID)
	return nil
}

// TestConnection тестирует подключение к MAX Bot API
func (s *MaxIntegrationService) TestConnection(ctx context.Context, companyID uint) error {
	config, err := s.GetConfig(ctx, companyID)
	if err != nil {
		return err
	}

	if config.BotToken == "" {
		return fmt.Errorf("токен бота не настроен")
	}

	// Проверяем токен через метод getMe
	// Примечание: URL API для MAX может отличаться от Telegram
	apiURL := fmt.Sprintf("https://api.max.ru/bot%s/getMe", config.BotToken)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка отправки запроса к MAX API: %w", err)
	}
	defer resp.Body.Close()

	var apiResp MaxAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("ошибка парсинга ответа от MAX API: %w", err)
	}

	if !apiResp.OK {
		return fmt.Errorf("неверный токен бота: %s (код: %d)", apiResp.Description, apiResp.ErrorCode)
	}

	s.logger.Printf("Тест подключения к MAX успешно пройден (компания: %d)", companyID)
	return nil
}

// GetIntegrationStatus получает статус интеграции
func (s *MaxIntegrationService) GetIntegrationStatus(ctx context.Context, companyID uint) (map[string]interface{}, error) {
	var integration models.Integration
	if err := s.db.WithContext(ctx).Where("company_id = ? AND integration_type = ?", companyID, "max").First(&integration).Error; err != nil {
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
