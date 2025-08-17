package api

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
)

// NotificationAPI представляет API для управления уведомлениями
type NotificationAPI struct {
	service *services.NotificationService
}

// NewNotificationAPI создает новый экземпляр NotificationAPI
func NewNotificationAPI(service *services.NotificationService) *NotificationAPI {
	return &NotificationAPI{
		service: service,
	}
}

// GetNotificationLogs возвращает логи уведомлений
// GET /api/notifications/logs
func (api *NotificationAPI) GetNotificationLogs(c *gin.Context) {
	companyID := getCompanyIDFromContext(c)

	// Получаем параметры пагинации
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	// Получаем фильтры
	filters := make(map[string]interface{})
	if notificationType := c.Query("type"); notificationType != "" {
		filters["type"] = notificationType
	}
	if channel := c.Query("channel"); channel != "" {
		filters["channel"] = channel
	}
	if status := c.Query("status"); status != "" {
		filters["status"] = status
	}
	if relatedType := c.Query("related_type"); relatedType != "" {
		filters["related_type"] = relatedType
	}

	logs, total, err := api.service.GetNotificationLogs(limit, offset, filters, companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения логов уведомлений: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"logs":   logs,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		},
	})
}

// GetNotificationStatistics возвращает статистику по уведомлениям
// GET /api/notifications/statistics
func (api *NotificationAPI) GetNotificationStatistics(c *gin.Context) {
	companyID := getCompanyIDFromContext(c)

	stats, err := api.service.GetNotificationStatistics(companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения статистики: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// GetNotificationTemplates возвращает шаблоны уведомлений
// GET /api/notifications/templates
func (api *NotificationAPI) GetNotificationTemplates(c *gin.Context) {
	companyID := getCompanyIDFromContext(c)
	db := database.GetTenantDB(c)

	var templates []models.NotificationTemplate
	query := db.Where("company_id = ? OR company_id = 0", companyID)

	// Фильтры
	if notificationType := c.Query("type"); notificationType != "" {
		query = query.Where("type = ?", notificationType)
	}
	if channel := c.Query("channel"); channel != "" {
		query = query.Where("channel = ?", channel)
	}
	if isActive := c.Query("is_active"); isActive != "" {
		query = query.Where("is_active = ?", isActive == "true")
	}

	err := query.Order("name").Find(&templates).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения шаблонов: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   templates,
	})
}

// CreateNotificationTemplate создает новый шаблон уведомления
// POST /api/notifications/templates
func (api *NotificationAPI) CreateNotificationTemplate(c *gin.Context) {
	companyID := getCompanyIDFromContext(c)
	db := database.GetTenantDB(c)

	var template models.NotificationTemplate
	if err := c.ShouldBindJSON(&template); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	template.CompanyID = companyID

	// Проверяем уникальность имени
	var existing models.NotificationTemplate
	err := db.Where("name = ? AND company_id = ?", template.Name, companyID).First(&existing).Error
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Шаблон с таким именем уже существует"})
		return
	}

	if err := db.Create(&template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания шаблона: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   template,
	})
}

// UpdateNotificationTemplate обновляет шаблон уведомления
// PUT /api/notifications/templates/:id
func (api *NotificationAPI) UpdateNotificationTemplate(c *gin.Context) {
	companyID := getCompanyIDFromContext(c)
	db := database.GetTenantDB(c)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID шаблона"})
		return
	}

	var template models.NotificationTemplate
	err = db.Where("id = ? AND company_id = ?", uint(id), companyID).First(&template).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Шаблон не найден"})
		return
	}

	var updateData models.NotificationTemplate
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Обновляем поля
	template.Name = updateData.Name
	template.Subject = updateData.Subject
	template.Template = updateData.Template
	template.Description = updateData.Description
	template.IsActive = updateData.IsActive
	template.Priority = updateData.Priority
	template.RetryAttempts = updateData.RetryAttempts
	template.DelaySeconds = updateData.DelaySeconds

	if err := db.Save(&template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления шаблона: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   template,
	})
}

// DeleteNotificationTemplate удаляет шаблон уведомления
// DELETE /api/notifications/templates/:id
func (api *NotificationAPI) DeleteNotificationTemplate(c *gin.Context) {
	companyID := getCompanyIDFromContext(c)
	db := database.GetTenantDB(c)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID шаблона"})
		return
	}

	var template models.NotificationTemplate
	err = db.Where("id = ? AND company_id = ?", uint(id), companyID).First(&template).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Шаблон не найден"})
		return
	}

	if err := db.Delete(&template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления шаблона: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Шаблон успешно удален",
	})
}

// GetNotificationSettings возвращает настройки уведомлений компании
// GET /api/notifications/settings
func (api *NotificationAPI) GetNotificationSettings(c *gin.Context) {
	companyID := getCompanyIDFromContext(c)
	db := database.GetTenantDB(c)

	var settings models.NotificationSettings
	err := db.Where("company_id = ?", companyID).First(&settings).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Создаем настройки по умолчанию
			settings = models.NotificationSettings{
				CompanyID:         companyID,
				TelegramEnabled:   false,
				EmailEnabled:      false,
				SMSEnabled:        false,
				DefaultLanguage:   "ru",
				MaxRetryAttempts:  3,
				RetryDelayMinutes: 5,
			}
			db.Create(&settings)
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения настроек: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   settings,
	})
}

// UpdateNotificationSettings обновляет настройки уведомлений компании
// PUT /api/notifications/settings
func (api *NotificationAPI) UpdateNotificationSettings(c *gin.Context) {
	companyID := getCompanyIDFromContext(c)
	db := database.GetTenantDB(c)

	var settings models.NotificationSettings
	err := db.Where("company_id = ?", companyID).First(&settings).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Создаем новые настройки
			settings.CompanyID = companyID
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения настроек: " + err.Error()})
			return
		}
	}

	var updateData models.NotificationSettings
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Обновляем настройки
	settings.TelegramBotToken = updateData.TelegramBotToken
	settings.TelegramWebhookURL = updateData.TelegramWebhookURL
	settings.TelegramEnabled = updateData.TelegramEnabled
	settings.SMTPHost = updateData.SMTPHost
	settings.SMTPPort = updateData.SMTPPort
	settings.SMTPUsername = updateData.SMTPUsername
	settings.SMTPPassword = updateData.SMTPPassword
	settings.SMTPFromEmail = updateData.SMTPFromEmail
	settings.SMTPFromName = updateData.SMTPFromName
	settings.SMTPUseTLS = updateData.SMTPUseTLS
	settings.EmailEnabled = updateData.EmailEnabled
	settings.SMSProvider = updateData.SMSProvider
	settings.SMSApiKey = updateData.SMSApiKey
	settings.SMSApiSecret = updateData.SMSApiSecret
	settings.SMSFromNumber = updateData.SMSFromNumber
	settings.SMSEnabled = updateData.SMSEnabled
	settings.DefaultLanguage = updateData.DefaultLanguage
	settings.MaxRetryAttempts = updateData.MaxRetryAttempts
	settings.RetryDelayMinutes = updateData.RetryDelayMinutes

	if err := db.Save(&settings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения настроек: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   settings,
	})
}

// TestNotification отправляет тестовое уведомление
// POST /api/notifications/test
func (api *NotificationAPI) TestNotification(c *gin.Context) {
	companyID := getCompanyIDFromContext(c)

	var request struct {
		Channel   string `json:"channel" binding:"required"`
		Recipient string `json:"recipient" binding:"required"`
		Message   string `json:"message" binding:"required"`
		Subject   string `json:"subject"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	templateData := map[string]interface{}{
		"Message":  request.Message,
		"TestMode": true,
	}

	err := api.service.SendNotification("test_notification", request.Channel, request.Recipient,
		templateData, companyID, 0, "test")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка отправки тестового уведомления: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Тестовое уведомление отправлено",
	})
}

// GetUserNotificationPreferences возвращает настройки уведомлений пользователя
// GET /api/notifications/preferences
func (api *NotificationAPI) GetUserNotificationPreferences(c *gin.Context) {
	userID := getUserIDFromContext(c)
	companyID := getCompanyIDFromContext(c)
	db := database.GetTenantDB(c)

	var prefs models.UserNotificationPreferences
	err := db.Where("user_id = ?", userID).First(&prefs).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Создаем настройки по умолчанию
			prefs = models.UserNotificationPreferences{
				UserID:                userID,
				CompanyID:             companyID,
				TelegramEnabled:       true,
				EmailEnabled:          true,
				SMSEnabled:            false,
				InstallationReminders: true,
				InstallationUpdates:   true,
				BillingAlerts:         true,
				WarehouseAlerts:       true,
				SystemNotifications:   true,
				QuietHoursStart:       "22:00",
				QuietHoursEnd:         "08:00",
				Timezone:              "Europe/Moscow",
			}
			db.Create(&prefs)
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения настроек: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   prefs,
	})
}

// UpdateUserNotificationPreferences обновляет настройки уведомлений пользователя
// PUT /api/notifications/preferences
func (api *NotificationAPI) UpdateUserNotificationPreferences(c *gin.Context) {
	userID := getUserIDFromContext(c)
	companyID := getCompanyIDFromContext(c)
	db := database.GetTenantDB(c)

	var prefs models.UserNotificationPreferences
	err := db.Where("user_id = ?", userID).First(&prefs).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			prefs.UserID = userID
			prefs.CompanyID = companyID
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения настроек: " + err.Error()})
			return
		}
	}

	var updateData models.UserNotificationPreferences
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Обновляем настройки
	prefs.TelegramEnabled = updateData.TelegramEnabled
	prefs.EmailEnabled = updateData.EmailEnabled
	prefs.SMSEnabled = updateData.SMSEnabled
	prefs.InstallationReminders = updateData.InstallationReminders
	prefs.InstallationUpdates = updateData.InstallationUpdates
	prefs.BillingAlerts = updateData.BillingAlerts
	prefs.WarehouseAlerts = updateData.WarehouseAlerts
	prefs.SystemNotifications = updateData.SystemNotifications
	prefs.QuietHoursStart = updateData.QuietHoursStart
	prefs.QuietHoursEnd = updateData.QuietHoursEnd
	prefs.Timezone = updateData.Timezone

	if err := db.Save(&prefs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения настроек: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   prefs,
	})
}

// ProcessTelegramWebhook обрабатывает webhook от Telegram
// POST /api/notifications/telegram/webhook/:company_id
func (api *NotificationAPI) ProcessTelegramWebhook(c *gin.Context) {
	companyIDStr := c.Param("company_id")
	companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID компании"})
		return
	}

	// TODO: Реализовать обработку webhook от Telegram
	// Пока что просто логируем получение webhook
	log.Printf("Получен Telegram webhook для компании %d", companyID)

	// Парсим обновление от Telegram
	var update map[string]interface{}
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// TODO: Обработать обновление через TelegramClient
	// client.ProcessUpdate(update)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// CreateDefaultTemplates создает шаблоны по умолчанию для компании
// POST /api/notifications/templates/defaults
func (api *NotificationAPI) CreateDefaultTemplates(c *gin.Context) {
	companyID := getCompanyIDFromContext(c)

	err := api.service.CreateDefaultTemplates(companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания шаблонов по умолчанию: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Шаблоны по умолчанию созданы",
	})
}

// Вспомогательные функции

func getCompanyIDFromContext(c *gin.Context) uint {
	if companyID, exists := c.Get("company_id"); exists {
		return companyID.(uint)
	}
	return 0
}

func getUserIDFromContext(c *gin.Context) uint {
	if userID, exists := c.Get("user_id"); exists {
		return userID.(uint)
	}
	return 0
}
