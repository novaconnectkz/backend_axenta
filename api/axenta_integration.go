package api

import (
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"backend_axenta/types"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AxentaIntegrationAPI API для управления интеграцией с Axenta Cloud
type AxentaIntegrationAPI struct {
	db            *gorm.DB
	axentaService *services.AxentaIntegrationService
}

// NewAxentaIntegrationAPI создает новый экземпляр API
func NewAxentaIntegrationAPI(db *gorm.DB) *AxentaIntegrationAPI {
	return &AxentaIntegrationAPI{
		db:            db,
		axentaService: services.NewAxentaIntegrationService(db),
	}
}

// RegisterRoutes регистрирует маршруты для API интеграции с Axenta
func (api *AxentaIntegrationAPI) RegisterRoutes(r *gin.RouterGroup) {
	log.Println("🔧 Registering Axenta integration routes...")
	axenta := r.Group("/axenta")
	{
		// Настройка интеграции
		axenta.POST("/setup", api.SetupIntegration)
		axenta.PUT("/setup", api.UpdateIntegration)
		axenta.GET("/config", api.GetIntegrationConfig)
		axenta.DELETE("/setup", api.DeleteIntegration)

		// Тестирование подключения
		axenta.POST("/test-connection", api.TestConnection)

		// Синхронизация объектов
		axenta.POST("/sync/objects", api.SyncObjects)
		axenta.POST("/sync/objects/auto", api.ScheduleAutoSync)

		// Мониторинг и ошибки
		axenta.GET("/errors", api.GetIntegrationErrors)
		axenta.PUT("/errors/:id/resolve", api.ResolveError)
		axenta.GET("/status", api.GetIntegrationStatus)
	}
	log.Println("✅ Axenta integration routes registered: /api/axenta/*")
}

// AxentaSetupIntegrationRequest запрос на настройку интеграции с Axenta
type AxentaSetupIntegrationRequest struct {
	APIURL          string `json:"api_url" binding:"required"`
	Username        string `json:"username" binding:"required"`
	Password        string `json:"password" binding:"required"`
	SyncInterval    int    `json:"sync_interval"`
	AutoSyncEnabled bool   `json:"auto_sync_enabled"`
	RetryAttempts   int    `json:"retry_attempts"`
	Timeout         int    `json:"timeout"`
}

// SetupIntegration настраивает интеграцию с Axenta Cloud
func (api *AxentaIntegrationAPI) SetupIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	
	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса настройки Axenta интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	var req AxentaSetupIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Устанавливаем значения по умолчанию
	if req.SyncInterval == 0 {
		req.SyncInterval = 15 // 15 минут
	}
	if req.RetryAttempts == 0 {
		req.RetryAttempts = 3
	}
	if req.Timeout == 0 {
		req.Timeout = 30 // 30 секунд
	}

	// Создаем конфигурацию
	config := services.AxentaIntegrationConfig{
		CompanyID:       companyID,
		APIURL:          req.APIURL,
		Username:        req.Username,
		Password:        req.Password,
		SyncInterval:    req.SyncInterval,
		AutoSyncEnabled: req.AutoSyncEnabled,
		RetryAttempts:   req.RetryAttempts,
		Timeout:         req.Timeout,
	}

	// Сериализуем конфигурацию
	configJSON, err := json.Marshal(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сериализации конфигурации"})
		return
	}

	// Проверяем, есть ли уже интеграция
	var existingIntegration models.Integration
	err = api.db.Where("company_id = ? AND integration_type = ?", companyID, "axenta_cloud").First(&existingIntegration).Error

	if err == gorm.ErrRecordNotFound {
		// Создаем новую интеграцию
		integration := models.Integration{
			CompanyID:       companyID,
			IntegrationType: "axenta_cloud",
			Name:            "Axenta Cloud API",
			Description:     "Основная интеграция с облачным сервисом Axenta для синхронизации объектов мониторинга",
			Settings:        string(configJSON),
			IsActive:        true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		if err := api.db.Create(&integration).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания интеграции"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":        "Интеграция с Axenta Cloud успешно настроена",
			"integration_id": integration.ID,
		})
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки существующей интеграции"})
		return
	} else {
		c.JSON(http.StatusConflict, gin.H{"error": "Интеграция с Axenta Cloud уже настроена"})
	}
}

// UpdateIntegration обновляет настройки интеграции с Axenta Cloud
func (api *AxentaIntegrationAPI) UpdateIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	
	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса обновления Axenta интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	var req AxentaSetupIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Находим существующую интеграцию
	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "axenta_cloud").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с Axenta Cloud не найдена"})
		return
	}

	// Обновляем конфигурацию
	config := services.AxentaIntegrationConfig{
		CompanyID:       companyID,
		APIURL:          req.APIURL,
		Username:        req.Username,
		Password:        req.Password,
		SyncInterval:    req.SyncInterval,
		AutoSyncEnabled: req.AutoSyncEnabled,
		RetryAttempts:   req.RetryAttempts,
		Timeout:         req.Timeout,
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сериализации конфигурации"})
		return
	}

	// Обновляем интеграцию
	integration.Settings = string(configJSON)
	integration.UpdatedAt = time.Now()

	if err := api.db.Save(&integration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления интеграции"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Настройки интеграции с Axenta Cloud обновлены"})
}

// GetIntegrationConfig получает конфигурацию интеграции
func (api *AxentaIntegrationAPI) GetIntegrationConfig(c *gin.Context) {
	log.Printf("🚀 GetIntegrationConfig вызвана!")
	companyID := middleware.GetCompanyID(c)
	
	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса конфигурации Axenta интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "axenta_cloud").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с Axenta Cloud не настроена"})
		return
	}

	var config services.AxentaIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка парсинга конфигурации"})
		return
	}

	// Получаем данные текущего пользователя из Axenta Cloud
	log.Printf("🔍 Попытка получить данные текущего пользователя...")
	currentUser, err := api.getCurrentAxentaUser(c)
	if err != nil {
		// Если не удалось получить данные пользователя, используем сохраненные
		log.Printf("⚠️ Не удалось получить данные текущего пользователя: %v", err)
	} else {
		// Обновляем настройки данными текущего пользователя
		log.Printf("✅ Получены данные пользователя: %s", currentUser.Username)
		config.Username = currentUser.Username
		// Пароль не обновляем, так как он не возвращается из API
	}

	// Скрываем пароль в ответе
	config.Password = "***"

	c.JSON(http.StatusOK, gin.H{
		"integration": integration,
		"config":      config,
	})
}

// DeleteIntegration удаляет интеграцию с Axenta Cloud
func (api *AxentaIntegrationAPI) DeleteIntegration(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	
	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса удаления Axenta интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "axenta_cloud").Delete(&models.Integration{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления интеграции"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Интеграция с Axenta Cloud удалена"})
}

// TestConnection тестирует подключение к Axenta Cloud
func (api *AxentaIntegrationAPI) TestConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	
	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса тестирования Axenta интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	if err := api.axentaService.TestConnection(c.Request.Context(), companyID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     "Тест подключения не пройден",
			"details":   err.Error(),
			"connected": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Подключение к Axenta Cloud успешно",
		"connected": true,
	})
}

// SyncObjects синхронизирует объекты с Axenta Cloud
func (api *AxentaIntegrationAPI) SyncObjects(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	
	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса синхронизации Axenta")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	if err := api.axentaService.SyncObjects(c.Request.Context(), companyID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Ошибка синхронизации объектов",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Синхронизация объектов завершена успешно",
	})
}

// ScheduleAutoSync планирует автоматическую синхронизацию
func (api *AxentaIntegrationAPI) ScheduleAutoSync(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	
	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса планирования синхронизации Axenta")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	if err := api.axentaService.ScheduleAutoSync(c.Request.Context(), companyID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Ошибка планирования автоматической синхронизации",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Автоматическая синхронизация запланирована",
	})
}

// GetIntegrationErrors получает список ошибок интеграции
func (api *AxentaIntegrationAPI) GetIntegrationErrors(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	
	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса ошибок Axenta интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	errors, err := api.axentaService.GetIntegrationErrors(c.Request.Context(), companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения списка ошибок"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"errors": errors,
	})
}

// ResolveError отмечает ошибку как решенную
func (api *AxentaIntegrationAPI) ResolveError(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	
	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса решения ошибки Axenta интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}
	errorID := c.Param("id")

	if err := api.axentaService.ResolveError(c.Request.Context(), companyID, errorID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Ошибка при решении проблемы",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Ошибка отмечена как решенная",
	})
}

// GetIntegrationStatus получает статус интеграции
func (api *AxentaIntegrationAPI) GetIntegrationStatus(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	
	// Проверяем, что компания определена (обязательно для безопасности)
	if companyID == 0 {
		log.Printf("❌ ОШИБКА БЕЗОПАСНОСТИ: GetCompanyID вернул 0 для запроса статуса Axenta интеграции")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Не удалось определить компанию. Обратитесь к администратору."})
		return
	}

	// Проверяем наличие интеграции
	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "axenta_cloud").First(&integration).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"configured":    false,
			"active":        false,
			"last_sync":     nil,
			"errors_count":  0,
			"connection_ok": false,
		})
		return
	}

	// Получаем детальный статус (используем для дополнительной информации)
	_, err := api.axentaService.GetIntegrationStatus(c.Request.Context(), companyID)
	if err != nil {
		// Если не удалось получить детальный статус, продолжаем с базовой информацией
		log.Printf("⚠️ Не удалось получить детальный статус интеграции: %v", err)
	}

	// Тестируем подключение
	connectionOK := false
	if err := api.axentaService.TestConnection(c.Request.Context(), companyID); err == nil {
		connectionOK = true
	}

	// Получаем количество нерешенных ошибок
	errors, _ := api.axentaService.GetIntegrationErrors(c.Request.Context(), companyID)
	errorsCount := len(errors)

	c.JSON(http.StatusOK, gin.H{
		"configured":    true,
		"active":        integration.IsActive,
		"last_sync":     integration.LastSyncAt,
		"errors_count":  errorsCount,
		"connection_ok": connectionOK,
		"created_at":    integration.CreatedAt,
		"updated_at":    integration.UpdatedAt,
		"error_message": integration.ErrorMessage,
	})
}

// getCurrentAxentaUser получает данные текущего пользователя из Axenta Cloud
func (api *AxentaIntegrationAPI) getCurrentAxentaUser(c *gin.Context) (*types.AxentaUserResponse, error) {
	// Получаем токен из заголовка Authorization
	authHeader := c.GetHeader("Authorization")
	headerPreview := authHeader
	if len(authHeader) > 20 {
		headerPreview = authHeader[:20] + "..."
	}
	log.Printf("🔑 Получен заголовок Authorization: %s", headerPreview)
	if authHeader == "" {
		return nil, fmt.Errorf("заголовок Authorization не найден")
	}

	// Создаем HTTP клиент
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Делаем запрос к Axenta Cloud для получения данных пользователя
	userURL := "https://axenta.cloud/api/current_user/"
	log.Printf("🌐 Делаем запрос к: %s", userURL)
	req, err := http.NewRequest("GET", userURL, nil)
	if err != nil {
		log.Printf("❌ Ошибка создания запроса: %v", err)
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")

	log.Printf("📤 Отправляем запрос с заголовками: Authorization=%s", headerPreview)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Ошибка выполнения запроса: %v", err)
		return nil, fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("📥 Получен ответ со статусом: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ Ошибка получения данных пользователя (статус: %d)", resp.StatusCode)
		return nil, fmt.Errorf("ошибка получения данных пользователя (статус: %d)", resp.StatusCode)
	}

	// Парсим ответ
	var userResponse types.AxentaUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&userResponse); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	return &userResponse, nil
}
