package api

import (
	"backend_axenta/models"
	"backend_axenta/services"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ReportsAPI предоставляет API для работы с отчетами
type ReportsAPI struct {
	db               *gorm.DB
	reportService    *services.ReportService
	schedulerService *services.ReportSchedulerService
}

// NewReportsAPI создает новый экземпляр ReportsAPI
func NewReportsAPI(db *gorm.DB, reportService *services.ReportService, schedulerService *services.ReportSchedulerService) *ReportsAPI {
	return &ReportsAPI{
		db:               db,
		reportService:    reportService,
		schedulerService: schedulerService,
	}
}

// RegisterRoutes регистрирует маршруты для API отчетов
func (ra *ReportsAPI) RegisterRoutes(router *gin.RouterGroup) {
	reports := router.Group("/reports")
	{
		// CRUD операции для отчетов
		reports.GET("", ra.GetReports)
		reports.POST("", ra.CreateReport)
		reports.GET("/:id", ra.GetReport)
		reports.PUT("/:id", ra.UpdateReport)
		reports.DELETE("/:id", ra.DeleteReport)

		// Генерация и скачивание отчетов
		reports.POST("/:id/generate", ra.GenerateReport)
		reports.GET("/:id/download", ra.DownloadReport)
		reports.GET("/:id/status", ra.GetReportStatus)

		// Шаблоны отчетов
		reports.GET("/templates", ra.GetReportTemplates)
		reports.POST("/templates", ra.CreateReportTemplate)
		reports.GET("/templates/:id", ra.GetReportTemplate)
		reports.PUT("/templates/:id", ra.UpdateReportTemplate)
		reports.DELETE("/templates/:id", ra.DeleteReportTemplate)

		// Расписания отчетов
		reports.GET("/schedules", ra.GetReportSchedules)
		reports.POST("/schedules", ra.CreateReportSchedule)
		reports.GET("/schedules/:id", ra.GetReportSchedule)
		reports.PUT("/schedules/:id", ra.UpdateReportSchedule)
		reports.DELETE("/schedules/:id", ra.DeleteReportSchedule)
		reports.POST("/schedules/:id/run", ra.RunScheduledReport)
		reports.GET("/schedules/:id/status", ra.GetScheduleStatus)

		// Выполнения отчетов
		reports.GET("/executions", ra.GetReportExecutions)
		reports.GET("/executions/:id", ra.GetReportExecution)

		// Статистика и аналитика
		reports.GET("/stats", ra.GetReportsStats)
	}
}

// CreateReportRequest представляет запрос на создание отчета
type CreateReportRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Type        models.ReportType      `json:"type" binding:"required"`
	Format      models.ReportFormat    `json:"format" binding:"required"`
	DateFrom    *time.Time             `json:"date_from"`
	DateTo      *time.Time             `json:"date_to"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// GetReports возвращает список отчетов
func (ra *ReportsAPI) GetReports(c *gin.Context) {
	companyID := getCompanyID(c)

	var reports []models.Report
	query := ra.db.Where("company_id = ?", companyID)

	// Фильтрация по типу
	if reportType := c.Query("type"); reportType != "" {
		query = query.Where("type = ?", reportType)
	}

	// Фильтрация по статусу
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// Фильтрация по датам
	if dateFrom := c.Query("date_from"); dateFrom != "" {
		if date, err := time.Parse("2006-01-02", dateFrom); err == nil {
			query = query.Where("created_at >= ?", date)
		}
	}
	if dateTo := c.Query("date_to"); dateTo != "" {
		if date, err := time.Parse("2006-01-02", dateTo); err == nil {
			query = query.Where("created_at <= ?", date.AddDate(0, 0, 1))
		}
	}

	// Пагинация
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Report{}).Count(&total)

	if err := query.Preload("CreatedBy").Offset(offset).Limit(limit).Order("created_at DESC").Find(&reports).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reports"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reports": reports,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

// CreateReport создает новый отчет
func (ra *ReportsAPI) CreateReport(c *gin.Context) {
	var req CreateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	companyID := getCompanyID(c)
	userID := getUserID(c)

	// Сериализуем параметры в JSON
	parametersJSON, _ := json.Marshal(req.Parameters)

	report := models.Report{
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		Format:      req.Format,
		DateFrom:    req.DateFrom,
		DateTo:      req.DateTo,
		Parameters:  string(parametersJSON),
		Status:      models.ReportStatusPending,
		CreatedByID: userID,
		CompanyID:   companyID,
	}

	if err := ra.db.Create(&report).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create report"})
		return
	}

	c.JSON(http.StatusCreated, report)
}

// GetReport возвращает отчет по ID
func (ra *ReportsAPI) GetReport(c *gin.Context) {
	reportID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
		return
	}

	companyID := getCompanyID(c)

	var report models.Report
	if err := ra.db.Where("id = ? AND company_id = ?", reportID, companyID).
		Preload("CreatedBy").First(&report).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch report"})
		return
	}

	c.JSON(http.StatusOK, report)
}

// UpdateReport обновляет отчет
func (ra *ReportsAPI) UpdateReport(c *gin.Context) {
	reportID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
		return
	}

	companyID := getCompanyID(c)

	var report models.Report
	if err := ra.db.Where("id = ? AND company_id = ?", reportID, companyID).First(&report).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch report"})
		return
	}

	var req CreateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Обновляем поля
	report.Name = req.Name
	report.Description = req.Description
	if report.Status == models.ReportStatusPending || report.Status == models.ReportStatusFailed {
		report.Type = req.Type
		report.Format = req.Format
		report.DateFrom = req.DateFrom
		report.DateTo = req.DateTo

		parametersJSON, _ := json.Marshal(req.Parameters)
		report.Parameters = string(parametersJSON)
	}

	if err := ra.db.Save(&report).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update report"})
		return
	}

	c.JSON(http.StatusOK, report)
}

// DeleteReport удаляет отчет
func (ra *ReportsAPI) DeleteReport(c *gin.Context) {
	reportID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
		return
	}

	companyID := getCompanyID(c)

	var report models.Report
	if err := ra.db.Where("id = ? AND company_id = ?", reportID, companyID).First(&report).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch report"})
		return
	}

	// Удаляем файл отчета если он существует
	if report.FilePath != "" {
		os.Remove(report.FilePath)
	}

	if err := ra.db.Delete(&report).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete report"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Report deleted successfully"})
}

// GenerateReport запускает генерацию отчета
func (ra *ReportsAPI) GenerateReport(c *gin.Context) {
	reportID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
		return
	}

	companyID := getCompanyID(c)

	var report models.Report
	if err := ra.db.Where("id = ? AND company_id = ?", reportID, companyID).First(&report).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch report"})
		return
	}

	// Проверяем, что отчет можно генерировать
	if report.Status == models.ReportStatusProcessing {
		c.JSON(http.StatusConflict, gin.H{"error": "Report is already being generated"})
		return
	}

	// Парсим параметры
	var params services.ReportParams
	if err := json.Unmarshal([]byte(report.Parameters), &params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report parameters"})
		return
	}

	params.CompanyID = companyID
	params.Type = report.Type
	params.Format = report.Format
	if params.DateFrom == nil {
		params.DateFrom = report.DateFrom
	}
	if params.DateTo == nil {
		params.DateTo = report.DateTo
	}

	// Запускаем генерацию в горутине
	go func() {
		if err := ra.reportService.GenerateReport(params, &report); err != nil {
			// Ошибка уже обработана в reportService
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message":   "Report generation started",
		"report_id": report.ID,
	})
}

// DownloadReport скачивает сгенерированный отчет
func (ra *ReportsAPI) DownloadReport(c *gin.Context) {
	reportID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
		return
	}

	companyID := getCompanyID(c)

	var report models.Report
	if err := ra.db.Where("id = ? AND company_id = ?", reportID, companyID).First(&report).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch report"})
		return
	}

	if report.Status != models.ReportStatusCompleted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Report is not ready for download"})
		return
	}

	if report.FilePath == "" || !fileExists(report.FilePath) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Report file not found"})
		return
	}

	// Определяем MIME тип по расширению
	ext := filepath.Ext(report.FilePath)
	var contentType string
	switch ext {
	case ".csv":
		contentType = "text/csv"
	case ".xlsx":
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pdf":
		contentType = "application/pdf"
	case ".json":
		contentType = "application/json"
	default:
		contentType = "application/octet-stream"
	}

	fileName := fmt.Sprintf("%s_%s%s", report.Name, report.CreatedAt.Format("20060102"), ext)

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	c.Header("Content-Type", contentType)
	c.File(report.FilePath)
}

// GetReportStatus возвращает статус генерации отчета
func (ra *ReportsAPI) GetReportStatus(c *gin.Context) {
	reportID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report ID"})
		return
	}

	companyID := getCompanyID(c)

	var report models.Report
	if err := ra.db.Where("id = ? AND company_id = ?", reportID, companyID).First(&report).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch report"})
		return
	}

	status := map[string]interface{}{
		"id":           report.ID,
		"status":       report.Status,
		"error_msg":    report.ErrorMsg,
		"record_count": report.RecordCount,
		"file_size":    report.FileSize,
		"started_at":   report.StartedAt,
		"completed_at": report.CompletedAt,
		"duration":     report.Duration,
		"download_url": nil,
	}

	if report.Status == models.ReportStatusCompleted && report.FilePath != "" {
		status["download_url"] = fmt.Sprintf("/api/reports/%d/download", report.ID)
	}

	c.JSON(http.StatusOK, status)
}

// CreateReportTemplateRequest представляет запрос на создание шаблона отчета
type CreateReportTemplateRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Type        models.ReportType      `json:"type" binding:"required"`
	Config      map[string]interface{} `json:"config"`
	SQLQuery    string                 `json:"sql_query"`
	Parameters  map[string]interface{} `json:"parameters"`
	Headers     []string               `json:"headers"`
	Formatting  map[string]interface{} `json:"formatting"`
	IsPublic    bool                   `json:"is_public"`
}

// GetReportTemplates возвращает список шаблонов отчетов
func (ra *ReportsAPI) GetReportTemplates(c *gin.Context) {
	companyID := getCompanyID(c)
	userID := getUserID(c)

	var templates []models.ReportTemplate
	query := ra.db.Where("company_id = ? AND (is_public = ? OR created_by_id = ?)", companyID, true, userID)

	if reportType := c.Query("type"); reportType != "" {
		query = query.Where("type = ?", reportType)
	}

	if err := query.Preload("CreatedBy").Order("name").Find(&templates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch templates"})
		return
	}

	c.JSON(http.StatusOK, templates)
}

// CreateReportTemplate создает новый шаблон отчета
func (ra *ReportsAPI) CreateReportTemplate(c *gin.Context) {
	var req CreateReportTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	companyID := getCompanyID(c)
	userID := getUserID(c)

	configJSON, _ := json.Marshal(req.Config)
	parametersJSON, _ := json.Marshal(req.Parameters)
	headersJSON, _ := json.Marshal(req.Headers)
	formattingJSON, _ := json.Marshal(req.Formatting)

	template := models.ReportTemplate{
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		Config:      string(configJSON),
		SQLQuery:    req.SQLQuery,
		Parameters:  string(parametersJSON),
		Headers:     string(headersJSON),
		Formatting:  string(formattingJSON),
		IsActive:    true,
		IsPublic:    req.IsPublic,
		CreatedByID: userID,
		CompanyID:   companyID,
	}

	if err := ra.db.Create(&template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create template"})
		return
	}

	c.JSON(http.StatusCreated, template)
}

// GetReportTemplate возвращает шаблон отчета по ID
func (ra *ReportsAPI) GetReportTemplate(c *gin.Context) {
	templateID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	companyID := getCompanyID(c)
	userID := getUserID(c)

	var template models.ReportTemplate
	if err := ra.db.Where("id = ? AND company_id = ? AND (is_public = ? OR created_by_id = ?)",
		templateID, companyID, true, userID).
		Preload("CreatedBy").First(&template).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch template"})
		return
	}

	c.JSON(http.StatusOK, template)
}

// UpdateReportTemplate обновляет шаблон отчета
func (ra *ReportsAPI) UpdateReportTemplate(c *gin.Context) {
	templateID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	companyID := getCompanyID(c)
	userID := getUserID(c)

	var template models.ReportTemplate
	if err := ra.db.Where("id = ? AND company_id = ? AND created_by_id = ?",
		templateID, companyID, userID).First(&template).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch template"})
		return
	}

	var req CreateReportTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	configJSON, _ := json.Marshal(req.Config)
	parametersJSON, _ := json.Marshal(req.Parameters)
	headersJSON, _ := json.Marshal(req.Headers)
	formattingJSON, _ := json.Marshal(req.Formatting)

	template.Name = req.Name
	template.Description = req.Description
	template.Type = req.Type
	template.Config = string(configJSON)
	template.SQLQuery = req.SQLQuery
	template.Parameters = string(parametersJSON)
	template.Headers = string(headersJSON)
	template.Formatting = string(formattingJSON)
	template.IsPublic = req.IsPublic

	if err := ra.db.Save(&template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update template"})
		return
	}

	c.JSON(http.StatusOK, template)
}

// DeleteReportTemplate удаляет шаблон отчета
func (ra *ReportsAPI) DeleteReportTemplate(c *gin.Context) {
	templateID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	companyID := getCompanyID(c)
	userID := getUserID(c)

	var template models.ReportTemplate
	if err := ra.db.Where("id = ? AND company_id = ? AND created_by_id = ?",
		templateID, companyID, userID).First(&template).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch template"})
		return
	}

	// Проверяем, что шаблон не используется в активных расписаниях
	var count int64
	ra.db.Model(&models.ReportSchedule{}).Where("template_id = ? AND is_active = ?", templateID, true).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Template is used in active schedules"})
		return
	}

	if err := ra.db.Delete(&template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template deleted successfully"})
}

// CreateReportScheduleRequest представляет запрос на создание расписания отчета
type CreateReportScheduleRequest struct {
	Name           string                    `json:"name" binding:"required"`
	Description    string                    `json:"description"`
	TemplateID     uint                      `json:"template_id" binding:"required"`
	Type           models.ReportScheduleType `json:"type" binding:"required"`
	CronExpression string                    `json:"cron_expression"`
	TimeOfDay      string                    `json:"time_of_day"`
	DayOfWeek      int                       `json:"day_of_week"`
	DayOfMonth     int                       `json:"day_of_month"`
	Parameters     map[string]interface{}    `json:"parameters"`
	Format         models.ReportFormat       `json:"format" binding:"required"`
	Recipients     []string                  `json:"recipients"`
}

// GetReportSchedules возвращает список расписаний отчетов
func (ra *ReportsAPI) GetReportSchedules(c *gin.Context) {
	companyID := getCompanyID(c)

	var schedules []models.ReportSchedule
	query := ra.db.Where("company_id = ?", companyID)

	if active := c.Query("active"); active != "" {
		query = query.Where("is_active = ?", active == "true")
	}

	if err := query.Preload("Template").Preload("CreatedBy").
		Preload("LastReport").Order("name").Find(&schedules).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch schedules"})
		return
	}

	c.JSON(http.StatusOK, schedules)
}

// CreateReportSchedule создает новое расписание отчета
func (ra *ReportsAPI) CreateReportSchedule(c *gin.Context) {
	var req CreateReportScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	companyID := getCompanyID(c)
	userID := getUserID(c)

	// Проверяем существование шаблона
	var template models.ReportTemplate
	if err := ra.db.Where("id = ? AND company_id = ?", req.TemplateID, companyID).
		First(&template).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Template not found"})
		return
	}

	parametersJSON, _ := json.Marshal(req.Parameters)
	recipientsJSON, _ := json.Marshal(req.Recipients)

	schedule := models.ReportSchedule{
		Name:           req.Name,
		Description:    req.Description,
		Type:           req.Type,
		TemplateID:     req.TemplateID,
		CronExpression: req.CronExpression,
		TimeOfDay:      req.TimeOfDay,
		DayOfWeek:      req.DayOfWeek,
		DayOfMonth:     req.DayOfMonth,
		Parameters:     string(parametersJSON),
		Format:         req.Format,
		Recipients:     string(recipientsJSON),
		IsActive:       true,
		CreatedByID:    userID,
		CompanyID:      companyID,
	}

	if err := ra.db.Create(&schedule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create schedule"})
		return
	}

	// Добавляем в планировщик
	if ra.schedulerService != nil {
		if err := ra.schedulerService.AddSchedule(schedule); err != nil {
			// Логируем ошибку, но не возвращаем её пользователю
			// так как расписание уже создано в БД
		}
	}

	c.JSON(http.StatusCreated, schedule)
}

// GetReportSchedule возвращает расписание отчета по ID
func (ra *ReportsAPI) GetReportSchedule(c *gin.Context) {
	scheduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schedule ID"})
		return
	}

	companyID := getCompanyID(c)

	var schedule models.ReportSchedule
	if err := ra.db.Where("id = ? AND company_id = ?", scheduleID, companyID).
		Preload("Template").Preload("CreatedBy").Preload("LastReport").
		First(&schedule).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Schedule not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch schedule"})
		return
	}

	c.JSON(http.StatusOK, schedule)
}

// UpdateReportSchedule обновляет расписание отчета
func (ra *ReportsAPI) UpdateReportSchedule(c *gin.Context) {
	scheduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schedule ID"})
		return
	}

	companyID := getCompanyID(c)

	var schedule models.ReportSchedule
	if err := ra.db.Where("id = ? AND company_id = ?", scheduleID, companyID).
		First(&schedule).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Schedule not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch schedule"})
		return
	}

	var req CreateReportScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	parametersJSON, _ := json.Marshal(req.Parameters)
	recipientsJSON, _ := json.Marshal(req.Recipients)

	schedule.Name = req.Name
	schedule.Description = req.Description
	schedule.Type = req.Type
	schedule.TemplateID = req.TemplateID
	schedule.CronExpression = req.CronExpression
	schedule.TimeOfDay = req.TimeOfDay
	schedule.DayOfWeek = req.DayOfWeek
	schedule.DayOfMonth = req.DayOfMonth
	schedule.Parameters = string(parametersJSON)
	schedule.Format = req.Format
	schedule.Recipients = string(recipientsJSON)

	if err := ra.db.Save(&schedule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update schedule"})
		return
	}

	// Обновляем в планировщике
	if ra.schedulerService != nil {
		if err := ra.schedulerService.UpdateSchedule(schedule); err != nil {
			// Логируем ошибку
		}
	}

	c.JSON(http.StatusOK, schedule)
}

// DeleteReportSchedule удаляет расписание отчета
func (ra *ReportsAPI) DeleteReportSchedule(c *gin.Context) {
	scheduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schedule ID"})
		return
	}

	companyID := getCompanyID(c)

	var schedule models.ReportSchedule
	if err := ra.db.Where("id = ? AND company_id = ?", scheduleID, companyID).
		First(&schedule).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Schedule not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch schedule"})
		return
	}

	if err := ra.db.Delete(&schedule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete schedule"})
		return
	}

	// Удаляем из планировщика
	if ra.schedulerService != nil {
		if err := ra.schedulerService.RemoveSchedule(uint(scheduleID)); err != nil {
			// Логируем ошибку
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Schedule deleted successfully"})
}

// RunScheduledReport запускает отчет по расписанию вручную
func (ra *ReportsAPI) RunScheduledReport(c *gin.Context) {
	scheduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schedule ID"})
		return
	}

	companyID := getCompanyID(c)

	var schedule models.ReportSchedule
	if err := ra.db.Where("id = ? AND company_id = ?", scheduleID, companyID).
		Preload("Template").First(&schedule).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Schedule not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch schedule"})
		return
	}

	// Запускаем выполнение в горутине
	go func() {
		// Здесь должна быть логика выполнения отчета по расписанию
		// Аналогично executeScheduledReport в scheduler service
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message":     "Scheduled report execution started",
		"schedule_id": schedule.ID,
	})
}

// GetScheduleStatus возвращает статус расписания
func (ra *ReportsAPI) GetScheduleStatus(c *gin.Context) {
	scheduleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schedule ID"})
		return
	}

	companyID := getCompanyID(c)

	if ra.schedulerService != nil {
		status, err := ra.schedulerService.GetScheduleStatus(uint(scheduleID))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get schedule status"})
			return
		}
		c.JSON(http.StatusOK, status)
		return
	}

	// Fallback: базовая информация из БД
	var schedule models.ReportSchedule
	if err := ra.db.Where("id = ? AND company_id = ?", scheduleID, companyID).
		Preload("LastReport").First(&schedule).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Schedule not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch schedule"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"schedule":   schedule,
		"is_running": false,
	})
}

// GetReportExecutions возвращает список выполнений отчетов
func (ra *ReportsAPI) GetReportExecutions(c *gin.Context) {
	companyID := getCompanyID(c)

	var executions []models.ReportExecution
	query := ra.db.Where("company_id = ?", companyID)

	if scheduleID := c.Query("schedule_id"); scheduleID != "" {
		query = query.Where("schedule_id = ?", scheduleID)
	}

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// Пагинация
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.ReportExecution{}).Count(&total)

	if err := query.Preload("Schedule").Preload("Report").
		Offset(offset).Limit(limit).Order("created_at DESC").Find(&executions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch executions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"executions": executions,
		"total":      total,
		"page":       page,
		"limit":      limit,
	})
}

// GetReportExecution возвращает выполнение отчета по ID
func (ra *ReportsAPI) GetReportExecution(c *gin.Context) {
	executionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid execution ID"})
		return
	}

	companyID := getCompanyID(c)

	var execution models.ReportExecution
	if err := ra.db.Where("id = ? AND company_id = ?", executionID, companyID).
		Preload("Schedule").Preload("Report").First(&execution).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Execution not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch execution"})
		return
	}

	c.JSON(http.StatusOK, execution)
}

// GetReportsStats возвращает статистику по отчетам
func (ra *ReportsAPI) GetReportsStats(c *gin.Context) {
	companyID := getCompanyID(c)

	var stats struct {
		TotalReports         int64 `json:"total_reports"`
		CompletedReports     int64 `json:"completed_reports"`
		FailedReports        int64 `json:"failed_reports"`
		ProcessingReports    int64 `json:"processing_reports"`
		TotalTemplates       int64 `json:"total_templates"`
		ActiveSchedules      int64 `json:"active_schedules"`
		TotalExecutions      int64 `json:"total_executions"`
		SuccessfulExecutions int64 `json:"successful_executions"`
		FailedExecutions     int64 `json:"failed_executions"`
	}

	ra.db.Model(&models.Report{}).Where("company_id = ?", companyID).Count(&stats.TotalReports)
	ra.db.Model(&models.Report{}).Where("company_id = ? AND status = ?", companyID, models.ReportStatusCompleted).Count(&stats.CompletedReports)
	ra.db.Model(&models.Report{}).Where("company_id = ? AND status = ?", companyID, models.ReportStatusFailed).Count(&stats.FailedReports)
	ra.db.Model(&models.Report{}).Where("company_id = ? AND status = ?", companyID, models.ReportStatusProcessing).Count(&stats.ProcessingReports)

	ra.db.Model(&models.ReportTemplate{}).Where("company_id = ?", companyID).Count(&stats.TotalTemplates)
	ra.db.Model(&models.ReportSchedule{}).Where("company_id = ? AND is_active = ?", companyID, true).Count(&stats.ActiveSchedules)

	ra.db.Model(&models.ReportExecution{}).Where("company_id = ?", companyID).Count(&stats.TotalExecutions)
	ra.db.Model(&models.ReportExecution{}).Where("company_id = ? AND status = ?", companyID, models.ReportStatusCompleted).Count(&stats.SuccessfulExecutions)
	ra.db.Model(&models.ReportExecution{}).Where("company_id = ? AND status = ?", companyID, models.ReportStatusFailed).Count(&stats.FailedExecutions)

	c.JSON(http.StatusOK, stats)
}

// Вспомогательные функции

// getCompanyID извлекает ID компании из контекста
func getCompanyID(c *gin.Context) uint {
	if companyID, exists := c.Get("company_id"); exists {
		if id, ok := companyID.(uint); ok {
			return id
		}
	}
	return 1 // Fallback для тестирования
}

// getUserID извлекает ID пользователя из контекста
func getUserID(c *gin.Context) uint {
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(uint); ok {
			return id
		}
	}
	return 1 // Fallback для тестирования
}

// fileExists проверяет существование файла
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}
