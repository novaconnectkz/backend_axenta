package api

import (
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WialonIntegrationAPI API для управления интеграцией с Wialon
type WialonIntegrationAPI struct {
	db      *gorm.DB
	service *services.WialonService
}

// NewWialonIntegrationAPI создает новый экземпляр API
func NewWialonIntegrationAPI(db *gorm.DB) *WialonIntegrationAPI {
	return &WialonIntegrationAPI{
		db:      db,
		service: services.NewWialonService(),
	}
}

// RegisterRoutes регистрирует маршруты для API интеграции с Wialon
func (api *WialonIntegrationAPI) RegisterRoutes(r *gin.RouterGroup) {
	wialon := r.Group("/wialon")
	{
		// Настройка интеграции
		wialon.POST("/setup", api.SetupIntegration)
		wialon.PUT("/setup", api.UpdateIntegration)
		wialon.GET("/config", api.GetIntegrationConfig)
		wialon.DELETE("/setup", api.DeleteIntegration)

		// Тестирование подключения
		wialon.POST("/test-connection", api.TestConnection)

		// Синхронизация данных
		wialon.POST("/sync", api.SyncData)
		wialon.GET("/sync-status", api.GetSyncStatus)

		// Получение данных из Wialon
		wialon.GET("/units", api.GetUnits)
		wialon.GET("/accounts", api.GetAccounts)
	}
	log.Println("✅ Wialon API routes registered: /api/wialon/*")
}

// RegisterTestRoutes регистрирует тестовые маршруты без авторизации (для отладки)
func (api *WialonIntegrationAPI) RegisterTestRoutes(r *gin.RouterGroup) {
	wialon := r.Group("/wialon")
	{
		wialon.POST("/test-connection", api.TestConnectionPublic)
	}
	log.Println("⚠️ Wialon TEST API routes registered: /api/test/wialon/* (без авторизации)")
}

// TestConnectionPublic публичный тест подключения (для отладки)
func (api *WialonIntegrationAPI) TestConnectionPublic(c *gin.Context) {
	var req struct {
		Token      string `json:"token" binding:"required"`
		DataCenter string `json:"data_center"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	if req.DataCenter == "" {
		req.DataCenter = "com"
	}

	result, err := api.service.TestConnection(req.Token, req.DataCenter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// WialonSetupIntegrationRequest запрос на настройку интеграции с Wialon
type WialonSetupIntegrationRequest struct {
	Token           string `json:"token" binding:"required"`
	DataCenter      string `json:"data_center"` // com, us, eu, org
	SyncInterval    int    `json:"sync_interval"`
	AutoSyncEnabled bool   `json:"auto_sync_enabled"`
	SyncVehicles    bool   `json:"sync_vehicles"`
	SyncSensors     bool   `json:"sync_sensors"`
	SyncMaintenance bool   `json:"sync_maintenance"`
	SyncDrivers     bool   `json:"sync_drivers"`
	SyncGeozones    bool   `json:"sync_geozones"`
	Enabled         bool   `json:"enabled"`
}

// WialonIntegrationConfig конфигурация интеграции Wialon
type WialonIntegrationConfig struct {
	CompanyID       uint   `json:"company_id"`
	Token           string `json:"token"`
	DataCenter      string `json:"data_center"`
	UserName        string `json:"user_name"` // Имя пользователя Wialon
	SyncInterval    int    `json:"sync_interval"`
	AutoSyncEnabled bool   `json:"auto_sync_enabled"`
	SyncVehicles    bool   `json:"sync_vehicles"`
	SyncSensors     bool   `json:"sync_sensors"`
	SyncMaintenance bool   `json:"sync_maintenance"`
	SyncDrivers     bool   `json:"sync_drivers"`
	SyncGeozones    bool   `json:"sync_geozones"`
	Enabled         bool   `json:"enabled"`
}

// SetupIntegration настраивает интеграцию с Wialon
func (api *WialonIntegrationAPI) SetupIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	// Проверяем, что компания определена
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса настройки Wialon интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	var req WialonSetupIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Проверяем токен
	if len(req.Token) != 72 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат токена. Токен должен состоять из 72 символов."})
		return
	}

	// Устанавливаем значения по умолчанию
	if req.DataCenter == "" {
		req.DataCenter = "com"
	}
	if req.SyncInterval == 0 {
		req.SyncInterval = 5 // 5 минут
	}

	// Тестируем подключение перед сохранением
	result, err := api.service.TestConnection(req.Token, req.DataCenter)
	if err != nil || !result.Success {
		errorMsg := "Не удалось подключиться к Wialon API"
		if result != nil && result.Message != "" {
			errorMsg = result.Message
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": errorMsg})
		return
	}

	// Создаем конфигурацию
	config := WialonIntegrationConfig{
		CompanyID:       companyID,
		Token:           req.Token,
		DataCenter:      req.DataCenter,
		UserName:        result.UserName, // Имя пользователя из результата авторизации
		SyncInterval:    req.SyncInterval,
		AutoSyncEnabled: req.AutoSyncEnabled,
		SyncVehicles:    req.SyncVehicles,
		SyncSensors:     req.SyncSensors,
		SyncMaintenance: req.SyncMaintenance,
		SyncDrivers:     req.SyncDrivers,
		SyncGeozones:    req.SyncGeozones,
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
	err = api.db.Where("company_id = ? AND integration_type = ?", companyID, "wialon").First(&existingIntegration).Error

	if err == gorm.ErrRecordNotFound {
		// Создаем новую интеграцию
		integration := models.Integration{
			CompanyID:       companyID,
			IntegrationType: "wialon",
			Name:            "Wialon Hosting",
			Description:     "Интеграция с системой GPS-мониторинга Wialon для получения данных о транспорте",
			Settings:        string(configJSON),
			IsActive:        req.Enabled,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		if err := api.db.Create(&integration).Error; err != nil {
			log.Printf("Ошибка создания интеграции Wialon: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания интеграции"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":        "Интеграция с Wialon успешно настроена",
			"integration_id": integration.ID,
			"user_name":      result.UserName,
		})
	} else if err != nil {
		log.Printf("Ошибка проверки существующей интеграции Wialon: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки существующей интеграции"})
		return
	} else {
		// Обновляем существующую интеграцию
		existingIntegration.Settings = string(configJSON)
		existingIntegration.IsActive = req.Enabled
		existingIntegration.UpdatedAt = time.Now()

		if err := api.db.Save(&existingIntegration).Error; err != nil {
			log.Printf("Ошибка обновления интеграции Wialon: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления интеграции"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":        "Интеграция с Wialon успешно обновлена",
			"integration_id": existingIntegration.ID,
			"user_name":      result.UserName,
		})
	}
}

// UpdateIntegration обновляет настройки интеграции с Wialon
func (api *WialonIntegrationAPI) UpdateIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса обновления Wialon интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	var req WialonSetupIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Находим существующую интеграцию
	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "wialon").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с Wialon не найдена"})
		return
	}

	// Парсим существующую конфигурацию
	var existingConfig WialonIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &existingConfig); err != nil {
		log.Printf("Ошибка парсинга существующей конфигурации: %v", err)
		existingConfig = WialonIntegrationConfig{
			CompanyID:  companyID,
			DataCenter: "com",
		}
	}

	// Обновляем только переданные поля
	if req.Token != "" {
		if len(req.Token) != 72 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат токена. Токен должен состоять из 72 символов."})
			return
		}
		existingConfig.Token = req.Token

		// Получаем user_name через TestConnection
		dataCenter := req.DataCenter
		if dataCenter == "" {
			dataCenter = existingConfig.DataCenter
		}
		result, err := api.service.TestConnection(req.Token, dataCenter)
		if err == nil && result.Success {
			existingConfig.UserName = result.UserName
		}
	}
	if req.DataCenter != "" {
		existingConfig.DataCenter = req.DataCenter
	}
	if req.SyncInterval > 0 {
		existingConfig.SyncInterval = req.SyncInterval
	}
	existingConfig.AutoSyncEnabled = req.AutoSyncEnabled
	existingConfig.SyncVehicles = req.SyncVehicles
	existingConfig.SyncSensors = req.SyncSensors
	existingConfig.SyncMaintenance = req.SyncMaintenance
	existingConfig.SyncDrivers = req.SyncDrivers
	existingConfig.SyncGeozones = req.SyncGeozones
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
		log.Printf("Ошибка обновления интеграции Wialon: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления интеграции"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Настройки интеграции с Wialon обновлены"})
}

// GetIntegrationConfig получает конфигурацию интеграции
func (api *WialonIntegrationAPI) GetIntegrationConfig(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса конфигурации Wialon интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "wialon").First(&integration).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с Wialon не найдена"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения конфигурации"})
		return
	}

	var config WialonIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка парсинга конфигурации"})
		return
	}

	// Маскируем токен для безопасности (показываем только первые и последние 4 символа)
	maskedToken := ""
	if len(config.Token) >= 8 {
		maskedToken = config.Token[:4] + "..." + config.Token[len(config.Token)-4:]
	}

	configResponse := map[string]interface{}{
		"data_center":       config.DataCenter,
		"token":             config.Token, // Полный токен для работы сервиса
		"token_masked":      maskedToken,
		"user_name":         config.UserName,
		"sync_interval":     config.SyncInterval,
		"auto_sync_enabled": config.AutoSyncEnabled,
		"sync_vehicles":     config.SyncVehicles,
		"sync_sensors":      config.SyncSensors,
		"sync_maintenance":  config.SyncMaintenance,
		"sync_drivers":      config.SyncDrivers,
		"sync_geozones":     config.SyncGeozones,
		"enabled":           config.Enabled,
		"has_token":         config.Token != "",
		"is_active":         integration.IsActive,
		"last_sync_at":      integration.LastSyncAt,
		"last_error_at":     integration.LastErrorAt,
		"error_message":     integration.ErrorMessage,
	}

	c.JSON(http.StatusOK, configResponse)
}

// DeleteIntegration удаляет интеграцию
func (api *WialonIntegrationAPI) DeleteIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "wialon").Delete(&models.Integration{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления интеграции"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Интеграция с Wialon удалена"})
}

// TestConnection тестирует подключение к API Wialon
func (api *WialonIntegrationAPI) TestConnection(c *gin.Context) {
	var req struct {
		Token      string `json:"token"`
		DataCenter string `json:"data_center"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных"})
		return
	}

	// Если токен не передан, пытаемся получить из сохраненной конфигурации
	if req.Token == "" {
		companyID := middleware.GetCompanyID(c)

		var integration models.Integration
		if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "wialon").First(&integration).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Токен не указан и интеграция не настроена"})
			return
		}

		var config WialonIntegrationConfig
		if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка парсинга конфигурации"})
			return
		}

		req.Token = config.Token
		if req.DataCenter == "" {
			req.DataCenter = config.DataCenter
		}
	}

	if req.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Токен не настроен"})
		return
	}

	if req.DataCenter == "" {
		req.DataCenter = "com"
	}

	result, err := api.service.TestConnection(req.Token, req.DataCenter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// SyncData запускает синхронизацию данных из Wialon
func (api *WialonIntegrationAPI) SyncData(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "wialon").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с Wialon не настроена"})
		return
	}

	var config WialonIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка парсинга конфигурации"})
		return
	}

	if config.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Токен не настроен"})
		return
	}

	// Авторизуемся
	loginResp, err := api.service.Login(config.Token, config.DataCenter)
	if err != nil {
		integration.UpdateStats(false, err.Error())
		api.db.Save(&integration)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка авторизации в Wialon: " + err.Error()})
		return
	}

	// Получаем объекты
	units, err := api.service.SearchUnits(loginResp.Eid, config.DataCenter)
	if err != nil {
		integration.UpdateStats(false, err.Error())
		api.db.Save(&integration)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения объектов: " + err.Error()})
		return
	}

	// Закрываем сессию
	go func() { _ = api.service.Logout(loginResp.Eid, config.DataCenter) }()

	// Обновляем статистику интеграции
	integration.UpdateStats(true, "")
	api.db.Save(&integration)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Синхронизация завершена успешно",
		"data": gin.H{
			"units_count": len(units),
			"units":       units,
		},
	})
}

// GetSyncStatus получает статус синхронизации
func (api *WialonIntegrationAPI) GetSyncStatus(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "wialon").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с Wialon не настроена"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"is_active":     integration.IsActive,
		"last_sync_at":  integration.LastSyncAt,
		"last_error_at": integration.LastErrorAt,
		"error_message": integration.ErrorMessage,
		"sync_count":    integration.SyncCount,
		"success_count": integration.SuccessCount,
		"error_count":   integration.ErrorCount,
		"success_rate":  integration.GetSuccessRate(),
		"is_healthy":    integration.IsHealthy(),
	})
}

// GetUnits получает объекты из Wialon
func (api *WialonIntegrationAPI) GetUnits(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "wialon").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с Wialon не настроена"})
		return
	}

	var config WialonIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка парсинга конфигурации"})
		return
	}

	if config.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Токен не настроен"})
		return
	}

	// Авторизуемся
	loginResp, err := api.service.Login(config.Token, config.DataCenter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка авторизации в Wialon: " + err.Error()})
		return
	}

	// Получаем объекты
	units, err := api.service.SearchUnits(loginResp.Eid, config.DataCenter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения объектов: " + err.Error()})
		return
	}

	// Получаем типы оборудования для подстановки названий
	hwTypes, err := api.service.GetHardwareTypes(loginResp.Eid, config.DataCenter)
	if err == nil && hwTypes != nil {
		// Подставляем названия типов оборудования
		for i := range units {
			if name, ok := hwTypes[units[i].HardwareType]; ok {
				units[i].HardwareTypeName = name
			}
		}
	}

	// Закрываем сессию
	go func() { _ = api.service.Logout(loginResp.Eid, config.DataCenter) }()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": units,
			"total": len(units),
		},
	})
}

// GetAccounts получает список аккаунтов (пользователей) Wialon
func (api *WialonIntegrationAPI) GetAccounts(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса Wialon аккаунтов")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	// Проверяем, есть ли интеграция с Wialon
	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "wialon").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с Wialon не настроена"})
		return
	}

	// Парсим конфигурацию
	var config WialonIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка чтения конфигурации"})
		return
	}

	if config.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Токен не настроен"})
		return
	}

	// Получаем аккаунты с объектами
	accounts, err := api.service.GetAccountsWithObjects(config.Token, config.DataCenter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения аккаунтов: " + err.Error()})
		return
	}

	// Подсчитываем статистику
	totalAccounts := len(accounts)
	activeAccounts := 0
	totalObjects := 0
	for _, acc := range accounts {
		if acc.IsActive {
			activeAccounts++
		}
		totalObjects += acc.ObjectsTotal
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": accounts,
			"total": totalAccounts,
			"stats": gin.H{
				"total":         totalAccounts,
				"active":        activeAccounts,
				"blocked":       totalAccounts - activeAccounts,
				"objects_total": totalObjects,
			},
		},
	})
}
