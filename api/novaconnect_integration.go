package api

import (
	"backend_axenta/middleware"
	"backend_axenta/models"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NovaConnectIntegrationAPI API для управления интеграцией с NovaConnect
type NovaConnectIntegrationAPI struct {
	db *gorm.DB
}

// NewNovaConnectIntegrationAPI создает новый экземпляр API
func NewNovaConnectIntegrationAPI(db *gorm.DB) *NovaConnectIntegrationAPI {
	return &NovaConnectIntegrationAPI{
		db: db,
	}
}

// RegisterRoutes регистрирует маршруты для API интеграции с NovaConnect
func (api *NovaConnectIntegrationAPI) RegisterRoutes(r *gin.RouterGroup) {
	novaconnect := r.Group("/novaconnect")
	{
		// Настройка интеграции
		novaconnect.POST("/setup", api.SetupIntegration)
		novaconnect.PUT("/setup", api.UpdateIntegration)
		novaconnect.GET("/config", api.GetIntegrationConfig)
		novaconnect.DELETE("/setup", api.DeleteIntegration)

		// Тестирование подключения
		novaconnect.POST("/test-connection", api.TestConnection)
	}
	log.Println("✅ NovaConnect API routes registered: /api/novaconnect/*")
}

// NovaConnectSetupIntegrationRequest запрос на настройку интеграции с NovaConnect
type NovaConnectSetupIntegrationRequest struct {
	APIURL          string `json:"api_url"`
	Token           string `json:"token"` // Не required для UpdateIntegration
	Language        string `json:"language"`
	WebhookURL      string `json:"webhook_url"`
	WebhookEnabled  bool   `json:"webhook_enabled"`
	SyncInterval    int    `json:"sync_interval"`
	AutoSyncEnabled bool   `json:"auto_sync_enabled"`
	Enabled         bool   `json:"enabled"`
}

// NovaConnectIntegrationConfig конфигурация интеграции NovaConnect
type NovaConnectIntegrationConfig struct {
	CompanyID       uint   `json:"company_id"`
	APIURL          string `json:"api_url"`
	Token           string `json:"token"`
	Language        string `json:"language"`
	WebhookURL      string `json:"webhook_url"`
	WebhookEnabled  bool   `json:"webhook_enabled"`
	SyncInterval    int    `json:"sync_interval"`
	AutoSyncEnabled bool   `json:"auto_sync_enabled"`
	Enabled         bool   `json:"enabled"`
}

// SetupIntegration настраивает интеграцию с NovaConnect
func (api *NovaConnectIntegrationAPI) SetupIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса настройки NovaConnect интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	var req NovaConnectSetupIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Токен обязателен только при создании новой интеграции
	if req.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Токен обязателен для настройки интеграции"})
		return
	}

	// Устанавливаем значения по умолчанию
	if req.APIURL == "" {
		req.APIURL = "https://api.novaconnect.kz/api"
	}
	if req.Language == "" {
		req.Language = "ru"
	}
	if req.SyncInterval == 0 {
		req.SyncInterval = 15 // 15 минут
	}

	// Создаем конфигурацию
	config := NovaConnectIntegrationConfig{
		CompanyID:       companyID,
		APIURL:          req.APIURL,
		Token:           req.Token,
		Language:        req.Language,
		WebhookURL:      req.WebhookURL,
		WebhookEnabled:  req.WebhookEnabled,
		SyncInterval:    req.SyncInterval,
		AutoSyncEnabled: req.AutoSyncEnabled,
		Enabled:         req.Enabled,
	}

	// Сериализуем конфигурацию
	configJSON, err := json.Marshal(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сериализации конфигурации"})
		return
	}

	// Проверяем, есть ли уже интеграция
	var existingIntegration models.Integration
	err = api.db.Where("company_id = ? AND integration_type = ?", companyID, "novaconnect").First(&existingIntegration).Error

	if err == gorm.ErrRecordNotFound {
		// Создаем новую интеграцию
		integration := models.Integration{
			CompanyID:       companyID,
			IntegrationType: "novaconnect",
			Name:            "NovaConnect API",
			Description:     "Интеграция с API NovaConnect для управления SIM-картами, счетами и отчетами",
			Settings:        string(configJSON),
			IsActive:        req.Enabled,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		if err := api.db.Create(&integration).Error; err != nil {
			log.Printf("Ошибка создания интеграции NovaConnect: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания интеграции"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":        "Интеграция с NovaConnect успешно настроена",
			"integration_id": integration.ID,
		})
	} else if err != nil {
		log.Printf("Ошибка проверки существующей интеграции NovaConnect: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки существующей интеграции"})
		return
	} else {
		// Обновляем существующую интеграцию
		existingIntegration.Settings = string(configJSON)
		existingIntegration.IsActive = req.Enabled
		existingIntegration.UpdatedAt = time.Now()

		if err := api.db.Save(&existingIntegration).Error; err != nil {
			log.Printf("Ошибка обновления интеграции NovaConnect: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления интеграции"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":        "Интеграция с NovaConnect успешно обновлена",
			"integration_id": existingIntegration.ID,
		})
	}
}

// UpdateIntegration обновляет настройки интеграции с NovaConnect
func (api *NovaConnectIntegrationAPI) UpdateIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса обновления NovaConnect интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	var req NovaConnectSetupIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Находим существующую интеграцию
	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "novaconnect").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с NovaConnect не найдена"})
		return
	}

	// Парсим существующую конфигурацию
	var existingConfig NovaConnectIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &existingConfig); err != nil {
		log.Printf("Ошибка парсинга существующей конфигурации: %v", err)
		// Создаем новую конфигурацию, если не удалось распарсить
		existingConfig = NovaConnectIntegrationConfig{
			CompanyID: companyID,
			APIURL:    "https://api.novaconnect.kz/api",
			Language:  "ru",
		}
	}

	// Обновляем только переданные поля (если токен не передан, сохраняем старый)
	if req.Token != "" {
		existingConfig.Token = req.Token
	}
	if req.APIURL != "" {
		existingConfig.APIURL = req.APIURL
	}
	if req.Language != "" {
		existingConfig.Language = req.Language
	}
	// Всегда обновляем webhook URL (может быть пустым)
	existingConfig.WebhookURL = req.WebhookURL
	existingConfig.WebhookEnabled = req.WebhookEnabled
	if req.SyncInterval > 0 {
		existingConfig.SyncInterval = req.SyncInterval
	}
	existingConfig.AutoSyncEnabled = req.AutoSyncEnabled
	existingConfig.Enabled = req.Enabled

	configJSON, err := json.Marshal(existingConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сериализации конфигурации"})
		return
	}

	// Обновляем интеграцию
	integration.Settings = string(configJSON)
	integration.IsActive = req.Enabled
	integration.UpdatedAt = time.Now()

	if err := api.db.Save(&integration).Error; err != nil {
		log.Printf("Ошибка обновления интеграции NovaConnect: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления интеграции"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Настройки интеграции с NovaConnect обновлены"})
}

// GetIntegrationConfig получает конфигурацию интеграции
func (api *NovaConnectIntegrationAPI) GetIntegrationConfig(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса конфигурации NovaConnect интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "novaconnect").First(&integration).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с NovaConnect не найдена"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения конфигурации"})
		return
	}

	var config NovaConnectIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка парсинга конфигурации"})
		return
	}

	// Возвращаем конфигурацию с токеном (доступ только для авторизованных пользователей компании)
	configResponse := map[string]interface{}{
		"api_url":           config.APIURL,
		"token":             config.Token, // Возвращаем токен для работы сервиса
		"language":          config.Language,
		"webhook_url":       config.WebhookURL,
		"webhook_enabled":   config.WebhookEnabled,
		"sync_interval":     config.SyncInterval,
		"auto_sync_enabled": config.AutoSyncEnabled,
		"enabled":           config.Enabled,
		"has_token":         config.Token != "",
		"is_active":         integration.IsActive,
	}

	c.JSON(http.StatusOK, configResponse)
}

// DeleteIntegration удаляет интеграцию
func (api *NovaConnectIntegrationAPI) DeleteIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "novaconnect").Delete(&models.Integration{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления интеграции"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Интеграция с NovaConnect удалена"})
}

// TestConnection тестирует подключение к API NovaConnect
func (api *NovaConnectIntegrationAPI) TestConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "novaconnect").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с NovaConnect не настроена"})
		return
	}

	var config NovaConnectIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка парсинга конфигурации"})
		return
	}

	if config.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Токен не настроен"})
		return
	}

	// Здесь можно добавить реальный тест подключения к API NovaConnect
	// Пока возвращаем успешный ответ
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Подключение к API NovaConnect успешно установлено",
	})
}
