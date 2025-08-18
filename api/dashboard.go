package api

import (
	"backend_axenta/middleware"
	"backend_axenta/models"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// DashboardStats структура для статистики dashboard
type DashboardStats struct {
	TotalObjects    int64     `json:"total_objects"`
	ActiveObjects   int64     `json:"active_objects"`
	InactiveObjects int64     `json:"inactive_objects"`
	TotalUsers      int64     `json:"total_users"`
	TotalContracts  int64     `json:"total_contracts"`
	ActiveContracts int64     `json:"active_contracts"`
	MonthlyRevenue  float64   `json:"monthly_revenue"`
	PendingPayments float64   `json:"pending_payments"`
	LastUpdated     time.Time `json:"last_updated"`
}

// ActivityItem структура для элемента активности
type ActivityItem struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	UserName    string    `json:"user_name"`
	ObjectName  string    `json:"object_name,omitempty"`
}

// NotificationItem структура для уведомления
type NotificationItem struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// GetDashboardStats получает общую статистику для dashboard
func GetDashboardStats(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	stats := DashboardStats{
		LastUpdated: time.Now(),
	}

	// Подсчет объектов
	tenantDB.Model(&models.Object{}).Count(&stats.TotalObjects)
	tenantDB.Model(&models.Object{}).Where("status = ?", "active").Count(&stats.ActiveObjects)
	tenantDB.Model(&models.Object{}).Where("status = ?", "inactive").Count(&stats.InactiveObjects)

	// Подсчет пользователей
	tenantDB.Model(&models.User{}).Count(&stats.TotalUsers)

	// Подсчет контрактов
	tenantDB.Model(&models.Contract{}).Count(&stats.TotalContracts)
	tenantDB.Model(&models.Contract{}).Where("status = ?", "active").Count(&stats.ActiveContracts)

	// Примерные финансовые данные (можно дополнить реальной логикой)
	stats.MonthlyRevenue = 150000.0
	stats.PendingPayments = 25000.0

	c.JSON(200, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// GetDashboardActivity получает последнюю активность
func GetDashboardActivity(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	var activities []ActivityItem

	// Получаем последние изменения объектов
	var objects []models.Object
	tenantDB.Model(&models.Object{}).
		Preload("Contract").
		Order("updated_at DESC").
		Limit(limit).
		Find(&objects)

	for _, obj := range objects {
		activities = append(activities, ActivityItem{
			ID:          strconv.FormatUint(uint64(obj.ID), 10),
			Type:        "object_update",
			Title:       "Обновление объекта",
			Description: "Объект " + obj.Name + " был обновлен",
			Timestamp:   obj.UpdatedAt,
			ObjectName:  obj.Name,
		})
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   activities,
	})
}

// GetDashboardNotifications получает уведомления для dashboard
func GetDashboardNotifications(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5"))
	unreadOnly := c.Query("unread_only") == "true"

	var notifications []NotificationItem

	// Примерные уведомления (можно дополнить реальной логикой)
	query := tenantDB.Model(&models.NotificationLog{}).Order("created_at DESC").Limit(limit)

	if unreadOnly {
		query = query.Where("is_read = ?", false)
	}

	var dbNotifications []models.NotificationLog
	query.Find(&dbNotifications)

	for _, notif := range dbNotifications {
		notifications = append(notifications, NotificationItem{
			ID:        strconv.FormatUint(uint64(notif.ID), 10),
			Type:      notif.Type,
			Title:     notif.Subject,
			Message:   notif.Message,
			IsRead:    notif.Status == "sent",
			CreatedAt: notif.CreatedAt,
		})
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   notifications,
	})
}

// GetDashboardLayouts получает макеты dashboard
func GetDashboardLayouts(c *gin.Context) {
	// Пока возвращаем пустой массив, так как макеты не реализованы
	c.JSON(200, gin.H{
		"status": "success",
		"data":   []interface{}{},
	})
}

// GetDefaultDashboardLayout получает макет по умолчанию
func GetDefaultDashboardLayout(c *gin.Context) {
	// Возвращаем базовый макет
	defaultLayout := map[string]interface{}{
		"id":      "default",
		"name":    "По умолчанию",
		"widgets": []interface{}{},
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   defaultLayout,
	})
}

// Упрощенные функции без мультитенантности для временного решения

// GetDashboardStatsSimple получает упрощенную статистику без tenant
func GetDashboardStatsSimple(c *gin.Context) {
	stats := DashboardStats{
		TotalObjects:    42,
		ActiveObjects:   38,
		InactiveObjects: 4,
		TotalUsers:      12,
		TotalContracts:  15,
		ActiveContracts: 13,
		MonthlyRevenue:  150000.0,
		PendingPayments: 25000.0,
		LastUpdated:     time.Now(),
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// GetDashboardActivitySimple получает упрощенную активность
func GetDashboardActivitySimple(c *gin.Context) {
	activities := []ActivityItem{
		{
			ID:          "1",
			Type:        "object_update",
			Title:       "Обновление объекта",
			Description: "Объект 'Офис центральный' был обновлен",
			Timestamp:   time.Now().Add(-1 * time.Hour),
			UserName:    "Drew",
			ObjectName:  "Офис центральный",
		},
		{
			ID:          "2",
			Type:        "user_login",
			Title:       "Вход в систему",
			Description: "Пользователь Drew вошел в систему",
			Timestamp:   time.Now().Add(-2 * time.Hour),
			UserName:    "Drew",
		},
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   activities,
	})
}

// GetDashboardNotificationsSimple получает упрощенные уведомления
func GetDashboardNotificationsSimple(c *gin.Context) {
	notifications := []NotificationItem{
		{
			ID:        "1",
			Type:      "info",
			Title:     "Добро пожаловать",
			Message:   "Добро пожаловать в систему Axenta CRM",
			IsRead:    false,
			CreatedAt: time.Now().Add(-30 * time.Minute),
		},
		{
			ID:        "2",
			Type:      "success",
			Title:     "Система готова",
			Message:   "Все сервисы работают нормально",
			IsRead:    true,
			CreatedAt: time.Now().Add(-1 * time.Hour),
		},
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   notifications,
	})
}
