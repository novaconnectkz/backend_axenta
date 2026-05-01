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

var maxIntegrationService *services.MaxIntegrationService

// InitMaxService инициализирует сервис интеграции с MAX
func InitMaxService() {
	logger := log.New(os.Stdout, "[MAX_API] ", log.LstdFlags|log.Lshortfile)
	maxIntegrationService = services.NewMaxIntegrationService(database.DB, logger)
}

// GetMaxService возвращает singleton сервиса интеграции (nil если не инициализирован).
// Используется NotificationService после InitMaxService.
func GetMaxService() *services.MaxIntegrationService {
	return maxIntegrationService
}

// MaxIntegrationAPI API для работы с интеграцией MAX
type MaxIntegrationAPI struct {
	db                   *gorm.DB
	maxIntegrationService *services.MaxIntegrationService
}

// NewMaxIntegrationAPI создает новый API для интеграции с MAX
func NewMaxIntegrationAPI() *MaxIntegrationAPI {
	return &MaxIntegrationAPI{
		db:                   database.DB,
		maxIntegrationService: maxIntegrationService,
	}
}

// RegisterRoutes регистрирует маршруты для API интеграции с MAX
func (api *MaxIntegrationAPI) RegisterRoutes(r *gin.RouterGroup) {
	max := r.Group("/integrations/max")
	{
		// Настройка интеграции
		max.POST("/setup", api.SetupIntegration)
		max.PUT("/setup", api.UpdateIntegration)
		max.GET("/config", api.GetIntegrationConfig)
		max.DELETE("/setup", api.DeleteIntegration)

		// Тестирование подключения
		max.POST("/test-connection", api.TestConnection)

		// Отправка сообщений
		max.POST("/send-message", api.SendMessage)

		// Статус интеграции
		max.GET("/status", api.GetIntegrationStatus)
	}
}

// MaxSetupIntegrationRequest запрос на настройку интеграции MAX
type MaxSetupIntegrationRequest struct {
	BotToken   string `json:"bot_token" binding:"required"`
	ParseMode  string `json:"parse_mode"`  // HTML, Markdown
	WebhookURL string `json:"webhook_url"` // URL для webhook
	UsePolling bool   `json:"use_polling"` // Использовать polling вместо webhook
}

// SetupIntegration настраивает интеграцию с MAX
func (api *MaxIntegrationAPI) SetupIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var req MaxSetupIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Устанавливаем значения по умолчанию
	if req.ParseMode == "" {
		req.ParseMode = "HTML"
	}

	// Создаем конфигурацию
	config := services.MaxIntegrationConfig{
		CompanyID:  companyID,
		BotToken:   req.BotToken,
		ParseMode:  req.ParseMode,
		WebhookURL: req.WebhookURL,
		UsePolling: req.UsePolling,
	}

	// Сохраняем конфигурацию
	if err := api.maxIntegrationService.SaveConfig(c.Request.Context(), &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения конфигурации: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Интеграция с MAX успешно настроена",
	})
}

// UpdateIntegration обновляет настройки интеграции с MAX
func (api *MaxIntegrationAPI) UpdateIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var req MaxSetupIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Получаем текущую конфигурацию
	config, err := api.maxIntegrationService.GetConfig(c.Request.Context(), companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с MAX не найдена"})
		return
	}

	// Обновляем поля
	if req.BotToken != "" {
		config.BotToken = req.BotToken
	}
	if req.ParseMode != "" {
		config.ParseMode = req.ParseMode
	}
	config.WebhookURL = req.WebhookURL
	config.UsePolling = req.UsePolling

	// Сохраняем обновленную конфигурацию
	if err := api.maxIntegrationService.SaveConfig(c.Request.Context(), config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления конфигурации: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Настройки интеграции с MAX обновлены"})
}

// GetIntegrationConfig получает конфигурацию интеграции
func (api *MaxIntegrationAPI) GetIntegrationConfig(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	config, err := api.maxIntegrationService.GetConfig(c.Request.Context(), companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с MAX не настроена"})
		return
	}

	// Проверяем, нужно ли показать токен
	showToken := c.Query("show_token")
	if showToken != "true" {
		// Скрываем токен в ответе
		config.BotToken = "***"
	}

	// Получаем информацию об интеграции
	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "max").First(&integration).Error; err != nil {
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

// DeleteIntegration удаляет интеграцию с MAX
func (api *MaxIntegrationAPI) DeleteIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "max").Delete(&models.Integration{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления интеграции"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Интеграция с MAX удалена"})
}

// TestConnection тестирует подключение к MAX Bot API
func (api *MaxIntegrationAPI) TestConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	if err := api.maxIntegrationService.TestConnection(c.Request.Context(), companyID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     "Тест подключения не пройден",
			"details":   err.Error(),
			"connected": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Подключение к MAX Bot API успешно",
		"connected": true,
	})
}

// MaxSendMessageRequest запрос на отправку сообщения
type MaxSendMessageRequest struct {
	ChatID    string                 `json:"chat_id" binding:"required"`
	Text      string                 `json:"text" binding:"required"`
	ParseMode string                 `json:"parse_mode"`
	Options   map[string]interface{} `json:"options"`
}

// SendMessage отправляет сообщение через MAX Bot
func (api *MaxIntegrationAPI) SendMessage(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var req MaxSendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Формируем опции
	options := make(map[string]interface{})
	if req.ParseMode != "" {
		options["parse_mode"] = req.ParseMode
	}
	if req.Options != nil {
		for k, v := range req.Options {
			options[k] = v
		}
	}

	// Отправляем сообщение
	if err := api.maxIntegrationService.SendMessage(c.Request.Context(), companyID, req.ChatID, req.Text, options); err != nil {
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
func (api *MaxIntegrationAPI) GetIntegrationStatus(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	status, err := api.maxIntegrationService.GetIntegrationStatus(c.Request.Context(), companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Ошибка получения статуса",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

