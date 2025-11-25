package api

import (
	"net/http"

	"backend_axenta/middleware"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// IntegrationsAPI API для управления списком интеграций
type IntegrationsAPI struct {
	db *gorm.DB
}

// NewIntegrationsAPI создает новый экземпляр API
func NewIntegrationsAPI(db *gorm.DB) *IntegrationsAPI {
	return &IntegrationsAPI{db: db}
}

// RegisterRoutes регистрирует маршруты
func (api *IntegrationsAPI) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/integrations", api.GetIntegrations)
}

// IntegrationInfo информация об интеграции
type IntegrationInfo struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
	Configured  bool   `json:"configured"`
}

// GetIntegrations возвращает список всех интеграций компании
func (api *IntegrationsAPI) GetIntegrations(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	// Проверяем, что компания определена
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию"})
		return
	}

	// Получаем все интеграции компании
	var integrations []models.Integration
	if err := api.db.Where("company_id = ?", companyID).Find(&integrations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения интеграций"})
		return
	}

	// Формируем map для быстрого поиска
	configuredTypes := make(map[string]bool)
	activeTypes := make(map[string]bool)
	for _, integration := range integrations {
		configuredTypes[integration.IntegrationType] = true
		if integration.IsActive {
			activeTypes[integration.IntegrationType] = true
		}
	}

	// Список всех доступных интеграций
	allIntegrations := []IntegrationInfo{
		{
			Type:        "axenta_cloud",
			Name:        "Axenta Cloud API",
			Description: "Основная интеграция с облачным сервисом Axenta для синхронизации объектов мониторинга",
			Configured:  configuredTypes["axenta_cloud"],
			IsActive:    activeTypes["axenta_cloud"],
		},
		{
			Type:        "novaconnect",
			Name:        "NovaConnect",
			Description: "Интеграция с NovaConnect для синхронизации данных мониторинга и управления SIM-картами",
			Configured:  configuredTypes["novaconnect"],
			IsActive:    activeTypes["novaconnect"],
		},
		{
			Type:        "1c",
			Name:        "1С:Предприятие",
			Description: "Интеграция с 1С для экспорта реестров платежей и синхронизации контрагентов",
			Configured:  configuredTypes["1c"],
			IsActive:    activeTypes["1c"],
		},
		{
			Type:        "telegram",
			Name:        "Telegram Bot",
			Description: "Отправка уведомлений пользователям через Telegram Bot API",
			Configured:  configuredTypes["telegram"],
			IsActive:    activeTypes["telegram"],
		},
		{
			Type:        "max",
			Name:        "MAX Messenger",
			Description: "Российский мессенджер для отправки уведомлений пользователям через MAX Bot API",
			Configured:  configuredTypes["max"],
			IsActive:    activeTypes["max"],
		},
		{
			Type:        "email",
			Name:        "Email SMTP",
			Description: "Отправка уведомлений по электронной почте через SMTP",
			Configured:  configuredTypes["email"],
			IsActive:    activeTypes["email"],
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"integrations": allIntegrations,
	})
}

