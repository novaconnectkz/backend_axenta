package api

import (
	"backend_axenta/audit"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuditAPI предоставляет endpoints для работы с аудит-логами
type AuditAPI struct {
	db *gorm.DB
}

// NewAuditAPI создает новый экземпляр AuditAPI
func NewAuditAPI(db *gorm.DB) *AuditAPI {
	return &AuditAPI{db: db}
}

// GetAuditLogsRequest запрос для получения аудит-логов
type GetAuditLogsRequest struct {
	Page      int       `form:"page" json:"page"`
	PerPage   int       `form:"per_page" json:"per_page"`
	UserID    string    `form:"user_id" json:"user_id"`
	Action    string    `form:"action" json:"action"`
	Level     string    `form:"level" json:"level"`
	Success   *bool     `form:"success" json:"success"`
	StartDate time.Time `form:"start_date" json:"start_date"`
	EndDate   time.Time `form:"end_date" json:"end_date"`
	Search    string    `form:"search" json:"search"`
	SortBy    string    `form:"sort_by" json:"sort_by"`
	SortOrder string    `form:"sort_order" json:"sort_order"`
}

// GetAuditLogs возвращает список аудит-логов с фильтрацией и пагинацией
func (api *AuditAPI) GetAuditLogs(c *gin.Context) {
	var req GetAuditLogsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Некорректные параметры запроса: " + err.Error(),
		})
		return
	}

	// Устанавливаем значения по умолчанию
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PerPage <= 0 {
		req.PerPage = 50
	}
	if req.PerPage > 1000 {
		req.PerPage = 1000
	}

	// Строим запрос
	query := api.db.Model(&audit.AuditLog{})

	// Применяем фильтры
	if req.UserID != "" {
		query = query.Where("user_id = ?", req.UserID)
	}
	if req.Action != "" {
		query = query.Where("action ILIKE ?", "%"+req.Action+"%")
	}
	if req.Level != "" {
		query = query.Where("level = ?", req.Level)
	}
	if req.Success != nil {
		query = query.Where("success = ?", *req.Success)
	}
	if !req.StartDate.IsZero() {
		query = query.Where("timestamp >= ?", req.StartDate)
	}
	if !req.EndDate.IsZero() {
		query = query.Where("timestamp <= ?", req.EndDate)
	}
	if req.Search != "" {
		query = query.Where("action ILIKE ? OR username ILIKE ? OR error ILIKE ?",
			"%"+req.Search+"%", "%"+req.Search+"%", "%"+req.Search+"%")
	}

	// Подсчет общего количества
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при подсчете логов: " + err.Error(),
		})
		return
	}

	// Сортировка
	sortBy := "timestamp"
	if req.SortBy != "" {
		sortBy = req.SortBy
	}
	sortOrder := "DESC"
	if req.SortOrder == "asc" || req.SortOrder == "ASC" {
		sortOrder = "ASC"
	}
	query = query.Order(sortBy + " " + sortOrder)

	// Пагинация
	offset := (req.Page - 1) * req.PerPage
	query = query.Offset(offset).Limit(req.PerPage)

	// Получаем данные
	var logs []audit.AuditLog
	if err := query.Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при получении логов: " + err.Error(),
		})
		return
	}

	// Логируем просмотр аудит-логов
	audit.LogSuccess(c, "audit_logs.view", gin.H{
		"filters": req,
		"count":   len(logs),
	})

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"items":       logs,
			"total":       total,
			"page":        req.Page,
			"per_page":    req.PerPage,
			"total_pages": (total + int64(req.PerPage) - 1) / int64(req.PerPage),
		},
	})
}

// GetAuditLogStats возвращает статистику по аудит-логам
func (api *AuditAPI) GetAuditLogStats(c *gin.Context) {
	// Параметры для фильтрации по периоду
	daysStr := c.DefaultQuery("days", "7")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		days = 7
	}

	startDate := time.Now().AddDate(0, 0, -days)

	// Общая статистика
	var stats struct {
		TotalLogs     int64 `json:"total_logs"`
		SuccessCount  int64 `json:"success_count"`
		ErrorCount    int64 `json:"error_count"`
		UniqueUsers   int64 `json:"unique_users"`
		UniqueActions int64 `json:"unique_actions"`
	}

	api.db.Model(&audit.AuditLog{}).
		Where("timestamp >= ?", startDate).
		Count(&stats.TotalLogs)

	api.db.Model(&audit.AuditLog{}).
		Where("timestamp >= ? AND success = ?", startDate, true).
		Count(&stats.SuccessCount)

	api.db.Model(&audit.AuditLog{}).
		Where("timestamp >= ? AND success = ?", startDate, false).
		Count(&stats.ErrorCount)

	api.db.Model(&audit.AuditLog{}).
		Where("timestamp >= ?", startDate).
		Distinct("user_id").
		Count(&stats.UniqueUsers)

	api.db.Model(&audit.AuditLog{}).
		Where("timestamp >= ?", startDate).
		Distinct("action").
		Count(&stats.UniqueActions)

	// Топ действий
	var topActions []struct {
		Action string `json:"action"`
		Count  int64  `json:"count"`
	}
	api.db.Model(&audit.AuditLog{}).
		Select("action, COUNT(*) as count").
		Where("timestamp >= ?", startDate).
		Group("action").
		Order("count DESC").
		Limit(10).
		Scan(&topActions)

	// Активность по уровням
	var levelStats []struct {
		Level string `json:"level"`
		Count int64  `json:"count"`
	}
	api.db.Model(&audit.AuditLog{}).
		Select("level, COUNT(*) as count").
		Where("timestamp >= ?", startDate).
		Group("level").
		Scan(&levelStats)

	// Активность по дням
	var dailyActivity []struct {
		Date  string `json:"date"`
		Count int64  `json:"count"`
	}
	api.db.Model(&audit.AuditLog{}).
		Select("DATE(timestamp) as date, COUNT(*) as count").
		Where("timestamp >= ?", startDate).
		Group("DATE(timestamp)").
		Order("date DESC").
		Scan(&dailyActivity)

	// Логируем просмотр статистики
	audit.LogSuccess(c, "audit_logs.stats", gin.H{
		"period_days": days,
	})

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"period":         days,
			"start_date":     startDate,
			"stats":          stats,
			"top_actions":    topActions,
			"level_stats":    levelStats,
			"daily_activity": dailyActivity,
		},
	})
}

// GetAuditLog возвращает детальную информацию об одном аудит-логе
func (api *AuditAPI) GetAuditLog(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Некорректный ID лога",
		})
		return
	}

	var log audit.AuditLog
	if err := api.db.First(&log, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Лог не найден",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при получении лога: " + err.Error(),
		})
		return
	}

	// Логируем просмотр конкретного лога
	audit.LogSuccess(c, "audit_logs.view_detail", gin.H{
		"log_id": id,
	})

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   log,
	})
}

// ExportAuditLogs экспортирует аудит-логи в JSON формате
func (api *AuditAPI) ExportAuditLogs(c *gin.Context) {
	var req GetAuditLogsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Некорректные параметры запроса: " + err.Error(),
		})
		return
	}

	// Строим запрос без пагинации для экспорта
	query := api.db.Model(&audit.AuditLog{})

	// Применяем фильтры
	if req.UserID != "" {
		query = query.Where("user_id = ?", req.UserID)
	}
	if req.Action != "" {
		query = query.Where("action ILIKE ?", "%"+req.Action+"%")
	}
	if req.Level != "" {
		query = query.Where("level = ?", req.Level)
	}
	if req.Success != nil {
		query = query.Where("success = ?", *req.Success)
	}
	if !req.StartDate.IsZero() {
		query = query.Where("timestamp >= ?", req.StartDate)
	}
	if !req.EndDate.IsZero() {
		query = query.Where("timestamp <= ?", req.EndDate)
	}

	// Ограничиваем максимальное количество для экспорта
	query = query.Limit(10000).Order("timestamp DESC")

	var logs []audit.AuditLog
	if err := query.Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при экспорте логов: " + err.Error(),
		})
		return
	}

	// Логируем экспорт
	audit.LogSuccess(c, "audit_logs.export", gin.H{
		"count":   len(logs),
		"filters": req,
	})

	// Возвращаем как JSON для скачивания
	c.Header("Content-Disposition", "attachment; filename=audit_logs_"+time.Now().Format("2006-01-02")+".json")
	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"data":        logs,
		"exported_at": time.Now(),
	})
}

// RegisterRoutes регистрирует маршруты для API аудит-логов
func (api *AuditAPI) RegisterRoutes(r *gin.RouterGroup) {
	audit := r.Group("/audit")
	{
		audit.GET("/logs", api.GetAuditLogs)
		audit.GET("/logs/:id", api.GetAuditLog)
		audit.GET("/stats", api.GetAuditLogStats)
		audit.GET("/export", api.ExportAuditLogs)
	}
}
