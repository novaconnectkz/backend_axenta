package api

import (
	"log"
	"net/http"
	"os"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var telegramIntegrationService *services.TelegramIntegrationService

// InitTelegramService инициализирует сервис интеграции с Telegram
func InitTelegramService() {
	logger := log.New(os.Stdout, "[Telegram_API] ", log.LstdFlags|log.Lshortfile)
	telegramIntegrationService = services.NewTelegramIntegrationService(database.DB, logger)
}

// TelegramIntegrationAPI API для работы с интеграцией Telegram
type TelegramIntegrationAPI struct {
	db                        *gorm.DB
	telegramIntegrationService *services.TelegramIntegrationService
}

// NewTelegramIntegrationAPI создает новый API для интеграции с Telegram
func NewTelegramIntegrationAPI() *TelegramIntegrationAPI {
	return &TelegramIntegrationAPI{
		db:                        database.DB,
		telegramIntegrationService: telegramIntegrationService,
	}
}

// RegisterRoutes регистрирует маршруты для API интеграции с Telegram
func (api *TelegramIntegrationAPI) RegisterRoutes(r *gin.RouterGroup) {
	telegram := r.Group("/telegram")
	{
		// Настройка интеграции
		telegram.POST("/setup", api.SetupIntegration)
		telegram.PUT("/setup", api.UpdateIntegration)
		telegram.GET("/config", api.GetIntegrationConfig)
		telegram.DELETE("/setup", api.DeleteIntegration)

		// Тестирование подключения
		telegram.POST("/test-connection", api.TestConnection)

		// Отправка сообщений
		telegram.POST("/send-message", api.SendMessage)

		// Статус интеграции
		telegram.GET("/status", api.GetIntegrationStatus)
	}
}

// TelegramSetupIntegrationRequest запрос на настройку интеграции Telegram
type TelegramSetupIntegrationRequest struct {
	BotToken             string `json:"bot_token" binding:"required"`
	DefaultChatID        string `json:"default_chat_id"`
	ParseMode            string `json:"parse_mode"`              // HTML, Markdown, MarkdownV2
	DisableNotifications bool   `json:"disable_notifications"`
	QuietHoursStart      string `json:"quiet_hours_start"`       // HH:mm
	QuietHoursEnd        string `json:"quiet_hours_end"`         // HH:mm
	QuietHoursEnabled    bool   `json:"quiet_hours_enabled"`
}

// SetupIntegration настраивает интеграцию с Telegram
func (api *TelegramIntegrationAPI) SetupIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var req TelegramSetupIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Устанавливаем значения по умолчанию
	if req.ParseMode == "" {
		req.ParseMode = "HTML"
	}

	// Создаем конфигурацию
	config := services.TelegramIntegrationConfig{
		CompanyID:            companyID,
		BotToken:             req.BotToken,
		DefaultChatID:        req.DefaultChatID,
		ParseMode:            req.ParseMode,
		DisableNotifications: req.DisableNotifications,
		QuietHoursStart:      req.QuietHoursStart,
		QuietHoursEnd:        req.QuietHoursEnd,
		QuietHoursEnabled:    req.QuietHoursEnabled,
	}

	// Сохраняем конфигурацию
	if err := api.telegramIntegrationService.SaveConfig(c.Request.Context(), &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения конфигурации: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Интеграция с Telegram успешно настроена",
	})
}

// UpdateIntegration обновляет настройки интеграции с Telegram
func (api *TelegramIntegrationAPI) UpdateIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var req TelegramSetupIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Получаем текущую конфигурацию
	config, err := api.telegramIntegrationService.GetConfig(c.Request.Context(), companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с Telegram не найдена"})
		return
	}

	// Обновляем поля
	if req.BotToken != "" {
		config.BotToken = req.BotToken
	}
	if req.DefaultChatID != "" {
		config.DefaultChatID = req.DefaultChatID
	}
	if req.ParseMode != "" {
		config.ParseMode = req.ParseMode
	}
	config.DisableNotifications = req.DisableNotifications
	config.QuietHoursStart = req.QuietHoursStart
	config.QuietHoursEnd = req.QuietHoursEnd
	config.QuietHoursEnabled = req.QuietHoursEnabled

	// Сохраняем обновленную конфигурацию
	if err := api.telegramIntegrationService.SaveConfig(c.Request.Context(), config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления конфигурации: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Настройки интеграции с Telegram обновлены"})
}

// GetIntegrationConfig получает конфигурацию интеграции
func (api *TelegramIntegrationAPI) GetIntegrationConfig(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	config, err := api.telegramIntegrationService.GetConfig(c.Request.Context(), companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с Telegram не настроена"})
		return
	}

	// Скрываем токен в ответе
	config.BotToken = "***"

	// Получаем информацию об интеграции
	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "telegram").First(&integration).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"config": config,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"integration": integration,
		"config":      config,
	})
}

// DeleteIntegration удаляет интеграцию с Telegram
func (api *TelegramIntegrationAPI) DeleteIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "telegram").Delete(&models.Integration{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления интеграции"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Интеграция с Telegram удалена"})
}

// TestConnection тестирует подключение к Telegram Bot API
func (api *TelegramIntegrationAPI) TestConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	if err := api.telegramIntegrationService.TestConnection(c.Request.Context(), companyID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     "Тест подключения не пройден",
			"details":   err.Error(),
			"connected": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Подключение к Telegram Bot API успешно",
		"connected": true,
	})
}

// SendMessageRequest запрос на отправку сообщения
type SendMessageRequest struct {
	ChatID                string                 `json:"chat_id"`
	Text                  string                 `json:"text" binding:"required"`
	ParseMode             string                 `json:"parse_mode"`
	DisableNotification  bool                  `json:"disable_notification"`
	DisableWebPagePreview bool                  `json:"disable_web_page_preview"`
	Options               map[string]interface{} `json:"options"`
}

// SendMessage отправляет сообщение через Telegram Bot
func (api *TelegramIntegrationAPI) SendMessage(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Формируем опции
	options := make(map[string]interface{})
	if req.ParseMode != "" {
		options["parse_mode"] = req.ParseMode
	}
	if req.DisableNotification {
		options["disable_notification"] = true
	}
	if req.DisableWebPagePreview {
		options["disable_web_page_preview"] = true
	}
	if req.Options != nil {
		for k, v := range req.Options {
			options[k] = v
		}
	}

	// Отправляем сообщение
	if err := api.telegramIntegrationService.SendMessage(c.Request.Context(), companyID, req.ChatID, req.Text, options); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Ошибка отправки сообщения",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Сообщение успешно отправлено",
	})
}

// GetIntegrationStatus получает статус интеграции
func (api *TelegramIntegrationAPI) GetIntegrationStatus(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	status, err := api.telegramIntegrationService.GetIntegrationStatus(c.Request.Context(), companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Ошибка получения статуса",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

