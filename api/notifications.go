package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
)

// NotificationAPI представляет admin-API для управления уведомлениями.
type NotificationAPI struct {
	service *services.NotificationService
}

// NewNotificationAPI создаёт API.
func NewNotificationAPI(service *services.NotificationService) *NotificationAPI {
	return &NotificationAPI{service: service}
}

// notificationCompanyID извлекает company_id из контекста (положен tenant middleware).
// Возвращает 0 если не найден.
func notificationCompanyID(c *gin.Context) uint {
	if v, ok := c.Get("company_id"); ok {
		if id, ok2 := v.(uint); ok2 {
			return id
		}
	}
	return 0
}

// notificationDB возвращает БД для работы с шаблонами/логами.
// NotificationTemplate и NotificationLog лежат в public schema (multi-tenant
// фильтруется по company_id), поэтому используем глобальный database.DB.
func notificationDB() *gorm.DB {
	return database.DB
}

// =====================================================================
// Templates CRUD
// =====================================================================

// ListTemplates возвращает шаблоны для текущей компании.
// GET /api/auth/notifications/templates?type=installation_created&channel=email
func (api *NotificationAPI) ListTemplates(c *gin.Context) {
	companyID := notificationCompanyID(c)

	q := notificationDB().Model(&models.NotificationTemplate{}).
		Where("company_id = ? OR company_id = 0", companyID)

	if t := c.Query("type"); t != "" {
		q = q.Where("type = ?", t)
	}
	if ch := c.Query("channel"); ch != "" {
		q = q.Where("channel = ?", ch)
	}

	var templates []models.NotificationTemplate
	if err := q.Order("created_at DESC").Find(&templates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения шаблонов: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": templates, "total": len(templates)})
}

// GetTemplate возвращает один шаблон.
// GET /api/auth/notifications/templates/:id
func (api *NotificationAPI) GetTemplate(c *gin.Context) {
	companyID := notificationCompanyID(c)
	id := c.Param("id")

	var tmpl models.NotificationTemplate
	err := notificationDB().Where("id = ? AND (company_id = ? OR company_id = 0)", id, companyID).First(&tmpl).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Шаблон не найден"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tmpl})
}

// CreateTemplate создаёт новый шаблон для компании.
// POST /api/auth/notifications/templates
func (api *NotificationAPI) CreateTemplate(c *gin.Context) {
	companyID := notificationCompanyID(c)

	var tmpl models.NotificationTemplate
	if err := c.ShouldBindJSON(&tmpl); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	tmpl.CompanyID = companyID
	if tmpl.Language == "" {
		tmpl.Language = "ru"
	}
	if tmpl.Priority == "" {
		tmpl.Priority = "normal"
	}

	if err := notificationDB().Create(&tmpl).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания шаблона: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": tmpl, "message": "Шаблон создан"})
}

// UpdateTemplate обновляет шаблон компании. Глобальные (company_id=0)
// нельзя редактировать через этот endpoint.
// PUT /api/auth/notifications/templates/:id
func (api *NotificationAPI) UpdateTemplate(c *gin.Context) {
	companyID := notificationCompanyID(c)
	id := c.Param("id")

	var existing models.NotificationTemplate
	if err := notificationDB().Where("id = ? AND company_id = ?", id, companyID).First(&existing).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Шаблон не найден или не принадлежит компании"})
		return
	}

	var update models.NotificationTemplate
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	// Не разрешаем менять CompanyID через апдейт
	update.CompanyID = existing.CompanyID

	if err := notificationDB().Model(&existing).Updates(update).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления: " + err.Error()})
		return
	}

	notificationDB().First(&existing, existing.ID)
	c.JSON(http.StatusOK, gin.H{"data": existing, "message": "Шаблон обновлён"})
}

// DeleteTemplate удаляет шаблон компании.
// DELETE /api/auth/notifications/templates/:id
func (api *NotificationAPI) DeleteTemplate(c *gin.Context) {
	companyID := notificationCompanyID(c)
	id := c.Param("id")

	res := notificationDB().Where("id = ? AND company_id = ?", id, companyID).Delete(&models.NotificationTemplate{})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления: " + res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Шаблон не найден"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Шаблон удалён"})
}

// SeedDefaults записывает в БД builtin-шаблоны для текущей компании.
// Идемпотентно — пропускает уже существующие.
// POST /api/auth/notifications/templates/seed
func (api *NotificationAPI) SeedDefaults(c *gin.Context) {
	companyID := notificationCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id не определён"})
		return
	}
	if err := api.service.CreateDefaultTemplates(companyID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сидинга: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Стандартные шаблоны созданы"})
}

// =====================================================================
// Test send, Logs, Stats
// =====================================================================

// TestSend отправляет тестовое уведомление.
// POST /api/auth/notifications/test
// Body: { "type": "installation_created", "channel": "email", "recipient": "test@example.com", "data": {...} }
func (api *NotificationAPI) TestSend(c *gin.Context) {
	var req struct {
		Type      string                 `json:"type" binding:"required"`
		Channel   string                 `json:"channel" binding:"required"`
		Recipient string                 `json:"recipient" binding:"required"`
		Data      map[string]interface{} `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	companyID := notificationCompanyID(c)
	if req.Data == nil {
		req.Data = map[string]interface{}{}
	}

	if err := api.service.SendNotification(req.Type, req.Channel, req.Recipient, req.Data, companyID, 0, "test"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Ошибка отправки: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Тестовое уведомление отправлено"})
}

// GetLogs возвращает логи отправок с пагинацией и фильтрами.
// GET /api/auth/notifications/logs?limit=20&offset=0&type=...&channel=...&status=...
func (api *NotificationAPI) GetLogs(c *gin.Context) {
	companyID := notificationCompanyID(c)

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	filters := map[string]interface{}{}
	if t := c.Query("type"); t != "" {
		filters["type"] = t
	}
	if ch := c.Query("channel"); ch != "" {
		filters["channel"] = ch
	}
	if st := c.Query("status"); st != "" {
		filters["status"] = st
	}
	if rt := c.Query("related_type"); rt != "" {
		filters["related_type"] = rt
	}

	logs, total, err := api.service.GetNotificationLogs(limit, offset, filters, companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения логов: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetStats возвращает сводку по каналам и статусам.
// GET /api/auth/notifications/stats
func (api *NotificationAPI) GetStats(c *gin.Context) {
	companyID := notificationCompanyID(c)
	stats, err := api.service.GetNotificationStatistics(companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения статистики: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// RegisterRoutes регистрирует все маршруты Notification API.
// Регистрация под /api/auth/notifications/* с auth middleware применяется на уровне группы.
func (api *NotificationAPI) RegisterRoutes(g *gin.RouterGroup) {
	notif := g.Group("/notifications")
	{
		notif.GET("/templates", api.ListTemplates)
		notif.GET("/templates/", api.ListTemplates)
		notif.POST("/templates", api.CreateTemplate)
		notif.POST("/templates/", api.CreateTemplate)
		notif.GET("/templates/:id", api.GetTemplate)
		notif.PUT("/templates/:id", api.UpdateTemplate)
		notif.DELETE("/templates/:id", api.DeleteTemplate)
		notif.POST("/templates/seed", api.SeedDefaults)

		notif.POST("/test", api.TestSend)
		notif.POST("/test/", api.TestSend)
		notif.GET("/logs", api.GetLogs)
		notif.GET("/logs/", api.GetLogs)
		notif.GET("/stats", api.GetStats)
		notif.GET("/stats/", api.GetStats)
	}
}
