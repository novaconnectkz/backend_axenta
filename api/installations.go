package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend_axenta/models"
)

// InstallationAPI представляет API для работы с монтажами
type InstallationAPI struct {
	DB *gorm.DB
}

// NewInstallationAPI создает новый экземпляр InstallationAPI
func NewInstallationAPI(db *gorm.DB) *InstallationAPI {
	return &InstallationAPI{DB: db}
}

// CreateInstallation создает новый монтаж
func (api *InstallationAPI) CreateInstallation(c *gin.Context) {
	var installation models.Installation
	if err := c.ShouldBindJSON(&installation); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	// Проверяем существование объекта
	var object models.Object
	if err := api.DB.First(&object, installation.ObjectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Объект не найден"})
		return
	}

	// Проверяем существование монтажника
	var installer models.Installer
	if err := api.DB.First(&installer, installation.InstallerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Монтажник не найден"})
		return
	}

	// Проверяем доступность монтажника на указанную дату
	if !installer.IsAvailableOnDate(installation.ScheduledAt) {
		c.JSON(http.StatusConflict, gin.H{"error": "Монтажник недоступен на указанную дату"})
		return
	}

	// Проверяем конфликты в расписании
	var conflictingInstallations []models.Installation
	startTime := installation.ScheduledAt
	endTime := startTime.Add(time.Duration(installation.EstimatedDuration) * time.Minute)

	err := api.DB.Where("installer_id = ? AND status IN ('planned', 'in_progress') AND scheduled_at BETWEEN ? AND ?",
		installation.InstallerID, startTime.Add(-2*time.Hour), endTime.Add(2*time.Hour)).
		Find(&conflictingInstallations).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при проверке расписания"})
		return
	}

	if len(conflictingInstallations) > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":     "У монтажника уже есть работы в это время",
			"conflicts": conflictingInstallations,
		})
		return
	}

	// Устанавливаем значения по умолчанию
	if installation.Status == "" {
		installation.Status = "planned"
	}
	if installation.Priority == "" {
		installation.Priority = "normal"
	}
	if installation.EstimatedDuration == 0 {
		installation.EstimatedDuration = 120 // 2 часа по умолчанию
	}

	// Получаем ID пользователя из контекста (добавлен middleware)
	if userID, exists := c.Get("user_id"); exists {
		if uid, ok := userID.(uint); ok {
			installation.CreatedByUserID = uid
		}
	}

	if err := api.DB.Create(&installation).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при создании монтажа: " + err.Error()})
		return
	}

	// Загружаем связанные данные
	api.DB.Preload("Object").Preload("Installer").Preload("Location").
		Preload("Equipment").Preload("CreatedByUser").First(&installation, installation.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "Монтаж успешно создан",
		"data":    installation,
	})
}

// GetInstallations возвращает список монтажей с фильтрацией
func (api *InstallationAPI) GetInstallations(c *gin.Context) {
	var installations []models.Installation
	query := api.DB.Preload("Object").Preload("Installer").Preload("Location").
		Preload("Equipment").Preload("CreatedByUser")

	// Фильтры
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if installerID := c.Query("installer_id"); installerID != "" {
		query = query.Where("installer_id = ?", installerID)
	}
	if objectID := c.Query("object_id"); objectID != "" {
		query = query.Where("object_id = ?", objectID)
	}
	if installationType := c.Query("type"); installationType != "" {
		query = query.Where("type = ?", installationType)
	}

	// Фильтр по дате
	if dateFrom := c.Query("date_from"); dateFrom != "" {
		if parsedDate, err := time.Parse("2006-01-02", dateFrom); err == nil {
			query = query.Where("scheduled_at >= ?", parsedDate)
		}
	}
	if dateTo := c.Query("date_to"); dateTo != "" {
		if parsedDate, err := time.Parse("2006-01-02", dateTo); err == nil {
			query = query.Where("scheduled_at <= ?", parsedDate.Add(24*time.Hour))
		}
	}

	// Сортировка
	sortBy := c.DefaultQuery("sort_by", "scheduled_at")
	sortOrder := c.DefaultQuery("sort_order", "asc")
	query = query.Order(sortBy + " " + sortOrder)

	// Пагинация
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset := (page - 1) * limit

	// Подсчет общего количества
	var total int64
	countQuery := api.DB.Model(&models.Installation{})
	if status := c.Query("status"); status != "" {
		countQuery = countQuery.Where("status = ?", status)
	}
	if installerID := c.Query("installer_id"); installerID != "" {
		countQuery = countQuery.Where("installer_id = ?", installerID)
	}
	if objectID := c.Query("object_id"); objectID != "" {
		countQuery = countQuery.Where("object_id = ?", objectID)
	}
	countQuery.Count(&total)

	if err := query.Limit(limit).Offset(offset).Find(&installations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении списка монтажей"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": installations,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
			"pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetInstallation возвращает информацию о конкретном монтаже
func (api *InstallationAPI) GetInstallation(c *gin.Context) {
	id := c.Param("id")
	var installation models.Installation

	if err := api.DB.Preload("Object").Preload("Installer").Preload("Location").
		Preload("Equipment").Preload("CreatedByUser").First(&installation, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Монтаж не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении монтажа"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": installation})
}

// UpdateInstallation обновляет информацию о монтаже
func (api *InstallationAPI) UpdateInstallation(c *gin.Context) {
	id := c.Param("id")
	var installation models.Installation

	if err := api.DB.First(&installation, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Монтаж не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске монтажа"})
		}
		return
	}

	var updateData models.Installation
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	// Проверяем изменение монтажника или времени
	if updateData.InstallerID != 0 && updateData.InstallerID != installation.InstallerID {
		var installer models.Installer
		if err := api.DB.First(&installer, updateData.InstallerID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Монтажник не найден"})
			return
		}
	}

	if err := api.DB.Model(&installation).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при обновлении монтажа"})
		return
	}

	// Загружаем обновленные данные
	api.DB.Preload("Object").Preload("Installer").Preload("Location").
		Preload("Equipment").Preload("CreatedByUser").First(&installation, installation.ID)

	c.JSON(http.StatusOK, gin.H{
		"message": "Монтаж успешно обновлен",
		"data":    installation,
	})
}

// DeleteInstallation удаляет монтаж (мягкое удаление)
func (api *InstallationAPI) DeleteInstallation(c *gin.Context) {
	id := c.Param("id")
	var installation models.Installation

	if err := api.DB.First(&installation, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Монтаж не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске монтажа"})
		}
		return
	}

	if err := api.DB.Delete(&installation).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при удалении монтажа"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Монтаж успешно удален"})
}

// StartInstallation начинает выполнение монтажа
func (api *InstallationAPI) StartInstallation(c *gin.Context) {
	id := c.Param("id")
	var installation models.Installation

	if err := api.DB.First(&installation, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Монтаж не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске монтажа"})
		}
		return
	}

	if installation.Status != "planned" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Монтаж не может быть начат в текущем статусе"})
		return
	}

	now := time.Now()
	installation.Status = "in_progress"
	installation.StartedAt = &now

	if err := api.DB.Save(&installation).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при обновлении статуса монтажа"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Монтаж начат",
		"data":    installation,
	})
}

// CompleteInstallation завершает монтаж
func (api *InstallationAPI) CompleteInstallation(c *gin.Context) {
	id := c.Param("id")
	var installation models.Installation

	if err := api.DB.First(&installation, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Монтаж не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске монтажа"})
		}
		return
	}

	if installation.Status != "in_progress" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Монтаж не может быть завершен в текущем статусе"})
		return
	}

	var completeData struct {
		Result         string  `json:"result"`
		Notes          string  `json:"notes"`
		ActualDuration int     `json:"actual_duration"`
		MaterialsCost  float64 `json:"materials_cost"`
		LaborCost      float64 `json:"labor_cost"`
	}

	if err := c.ShouldBindJSON(&completeData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	now := time.Now()
	installation.Status = "completed"
	installation.CompletedAt = &now
	installation.Result = completeData.Result
	installation.Notes = completeData.Notes
	installation.ActualDuration = completeData.ActualDuration
	installation.MaterialsCost = completeData.MaterialsCost
	installation.LaborCost = completeData.LaborCost

	// Если не указана фактическая продолжительность, рассчитываем ее
	if installation.ActualDuration == 0 && installation.StartedAt != nil {
		duration := now.Sub(*installation.StartedAt)
		installation.ActualDuration = int(duration.Minutes())
	}

	if err := api.DB.Save(&installation).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при завершении монтажа"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Монтаж успешно завершен",
		"data":    installation,
	})
}

// CancelInstallation отменяет монтаж
func (api *InstallationAPI) CancelInstallation(c *gin.Context) {
	id := c.Param("id")
	var installation models.Installation

	if err := api.DB.First(&installation, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Монтаж не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске монтажа"})
		}
		return
	}

	if installation.Status == "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Завершенный монтаж не может быть отменен"})
		return
	}

	var cancelData struct {
		Reason string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&cancelData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	installation.Status = "cancelled"
	installation.Notes = "Отменен: " + cancelData.Reason

	if err := api.DB.Save(&installation).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при отмене монтажа"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Монтаж отменен",
		"data":    installation,
	})
}

// GetInstallerSchedule возвращает расписание монтажника
func (api *InstallationAPI) GetInstallerSchedule(c *gin.Context) {
	installerID := c.Param("installer_id")

	// Получаем параметры даты
	dateFrom := c.DefaultQuery("date_from", time.Now().Format("2006-01-02"))
	dateTo := c.DefaultQuery("date_to", time.Now().AddDate(0, 0, 7).Format("2006-01-02"))

	parsedDateFrom, err := time.Parse("2006-01-02", dateFrom)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректная дата начала"})
		return
	}

	parsedDateTo, err := time.Parse("2006-01-02", dateTo)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректная дата окончания"})
		return
	}

	var installations []models.Installation
	if err := api.DB.Where("installer_id = ? AND scheduled_at BETWEEN ? AND ? AND status IN ('planned', 'in_progress')",
		installerID, parsedDateFrom, parsedDateTo.Add(24*time.Hour)).
		Preload("Object").Order("scheduled_at ASC").Find(&installations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении расписания"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": installations})
}

// GetInstallationStatistics возвращает статистику по монтажам
func (api *InstallationAPI) GetInstallationStatistics(c *gin.Context) {
	var stats struct {
		Total      int64 `json:"total"`
		Planned    int64 `json:"planned"`
		InProgress int64 `json:"in_progress"`
		Completed  int64 `json:"completed"`
		Cancelled  int64 `json:"cancelled"`
		Overdue    int64 `json:"overdue"`
		Today      int64 `json:"today"`
		ThisWeek   int64 `json:"this_week"`
		ThisMonth  int64 `json:"this_month"`
	}

	// Общая статистика
	api.DB.Model(&models.Installation{}).Count(&stats.Total)
	api.DB.Model(&models.Installation{}).Where("status = 'planned'").Count(&stats.Planned)
	api.DB.Model(&models.Installation{}).Where("status = 'in_progress'").Count(&stats.InProgress)
	api.DB.Model(&models.Installation{}).Where("status = 'completed'").Count(&stats.Completed)
	api.DB.Model(&models.Installation{}).Where("status = 'cancelled'").Count(&stats.Cancelled)

	// Просроченные
	api.DB.Model(&models.Installation{}).
		Where("status IN ('planned', 'in_progress') AND scheduled_at < ?", time.Now()).
		Count(&stats.Overdue)

	// По периодам
	today := time.Now().Truncate(24 * time.Hour)
	weekStart := today.AddDate(0, 0, -int(today.Weekday())+1)
	monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())

	api.DB.Model(&models.Installation{}).
		Where("scheduled_at >= ? AND scheduled_at < ?", today, today.Add(24*time.Hour)).
		Count(&stats.Today)

	api.DB.Model(&models.Installation{}).
		Where("scheduled_at >= ? AND scheduled_at < ?", weekStart, weekStart.AddDate(0, 0, 7)).
		Count(&stats.ThisWeek)

	api.DB.Model(&models.Installation{}).
		Where("scheduled_at >= ? AND scheduled_at < ?", monthStart, monthStart.AddDate(0, 1, 0)).
		Count(&stats.ThisMonth)

	c.JSON(http.StatusOK, gin.H{"data": stats})
}
