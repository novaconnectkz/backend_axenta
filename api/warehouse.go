package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend_axenta/models"
)

// WarehouseAPI представляет API для работы со складом
type WarehouseAPI struct {
	DB *gorm.DB
}

// NewWarehouseAPI создает новый экземпляр WarehouseAPI
func NewWarehouseAPI(db *gorm.DB) *WarehouseAPI {
	return &WarehouseAPI{DB: db}
}

// CreateWarehouseOperation создает складскую операцию
func (api *WarehouseAPI) CreateWarehouseOperation(c *gin.Context) {
	var operation models.WarehouseOperation
	if err := c.ShouldBindJSON(&operation); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	// Проверяем существование оборудования
	var equipment models.Equipment
	if err := api.DB.First(&equipment, operation.EquipmentID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Оборудование не найдено"})
		return
	}

	if err := api.DB.Create(&operation).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при создании операции: " + err.Error()})
		return
	}

	// Загружаем связанные данные для ответа
	api.DB.Preload("Equipment").Preload("User").Preload("Installation").First(&operation, operation.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "Операция успешно создана",
		"data":    operation,
	})
}

// GetWarehouseOperations возвращает список складских операций
func (api *WarehouseAPI) GetWarehouseOperations(c *gin.Context) {
	var operations []models.WarehouseOperation
	query := api.DB.Preload("Equipment").Preload("User").Preload("Installation")

	// Фильтры
	if operationType := c.Query("type"); operationType != "" {
		query = query.Where("type = ?", operationType)
	}
	if equipmentID := c.Query("equipment_id"); equipmentID != "" {
		query = query.Where("equipment_id = ?", equipmentID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// Фильтр по дате
	if dateFrom := c.Query("date_from"); dateFrom != "" {
		query = query.Where("created_at >= ?", dateFrom)
	}
	if dateTo := c.Query("date_to"); dateTo != "" {
		query = query.Where("created_at <= ?", dateTo)
	}

	// Сортировка
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	query = query.Order(sortBy + " " + sortOrder)

	// Пагинация
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	api.DB.Model(&models.WarehouseOperation{}).Count(&total)

	if err := query.Limit(limit).Offset(offset).Find(&operations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении списка операций"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": operations,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
			"pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// CreateEquipmentCategory создает новую категорию оборудования
func (api *WarehouseAPI) CreateEquipmentCategory(c *gin.Context) {
	var category models.EquipmentCategory
	if err := c.ShouldBindJSON(&category); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	if err := api.DB.Create(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при создании категории: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Категория успешно создана",
		"data":    category,
	})
}

// GetEquipmentCategories возвращает список категорий оборудования
func (api *WarehouseAPI) GetEquipmentCategories(c *gin.Context) {
	var categories []models.EquipmentCategory
	query := api.DB.Model(&models.EquipmentCategory{})

	// Фильтр по активности
	if active := c.Query("active"); active == "true" {
		query = query.Where("is_active = ?", true)
	}

	if err := query.Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении списка категорий"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": categories})
}

// UpdateEquipmentCategory обновляет категорию оборудования
func (api *WarehouseAPI) UpdateEquipmentCategory(c *gin.Context) {
	id := c.Param("id")
	var category models.EquipmentCategory

	if err := api.DB.First(&category, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Категория не найдена"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске категории"})
		}
		return
	}

	var updateData models.EquipmentCategory
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	if err := api.DB.Model(&category).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при обновлении категории"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Категория успешно обновлена",
		"data":    category,
	})
}

// DeleteEquipmentCategory удаляет категорию оборудования
func (api *WarehouseAPI) DeleteEquipmentCategory(c *gin.Context) {
	id := c.Param("id")
	var category models.EquipmentCategory

	if err := api.DB.First(&category, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Категория не найдена"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске категории"})
		}
		return
	}

	// Проверяем, есть ли оборудование в этой категории
	var equipmentCount int64
	api.DB.Model(&models.Equipment{}).Where("category_id = ?", id).Count(&equipmentCount)
	if equipmentCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Нельзя удалить категорию, в которой есть оборудование"})
		return
	}

	if err := api.DB.Delete(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при удалении категории"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Категория успешно удалена"})
}

// GetStockAlerts возвращает список уведомлений о складских проблемах
func (api *WarehouseAPI) GetStockAlerts(c *gin.Context) {
	var alerts []models.StockAlert
	query := api.DB.Preload("Equipment").Preload("EquipmentCategory").Preload("AssignedUser")

	// Фильтры
	if alertType := c.Query("type"); alertType != "" {
		query = query.Where("type = ?", alertType)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if severity := c.Query("severity"); severity != "" {
		query = query.Where("severity = ?", severity)
	}

	// Только активные уведомления по умолчанию
	if c.Query("include_resolved") != "true" {
		query = query.Where("status = 'active'")
	}

	// Сортировка по важности и дате
	query = query.Order("severity DESC, created_at DESC")

	if err := query.Find(&alerts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении списка уведомлений"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": alerts})
}

// CreateStockAlert создает новое уведомление
func (api *WarehouseAPI) CreateStockAlert(c *gin.Context) {
	var alert models.StockAlert
	if err := c.ShouldBindJSON(&alert); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	if err := api.DB.Create(&alert).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при создании уведомления: " + err.Error()})
		return
	}

	// Загружаем связанные данные для ответа
	api.DB.Preload("Equipment").Preload("EquipmentCategory").Preload("AssignedUser").First(&alert, alert.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "Уведомление успешно создано",
		"data":    alert,
	})
}

// AcknowledgeStockAlert отмечает уведомление как прочитанное
func (api *WarehouseAPI) AcknowledgeStockAlert(c *gin.Context) {
	id := c.Param("id")
	var alert models.StockAlert

	if err := api.DB.First(&alert, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Уведомление не найдено"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске уведомления"})
		}
		return
	}

	now := time.Now()
	alert.Status = "acknowledged"
	alert.ReadAt = &now

	if err := api.DB.Save(&alert).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при обновлении уведомления"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Уведомление отмечено как прочитанное",
		"data":    alert,
	})
}

// ResolveStockAlert разрешает уведомление
func (api *WarehouseAPI) ResolveStockAlert(c *gin.Context) {
	id := c.Param("id")
	var alert models.StockAlert

	if err := api.DB.First(&alert, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Уведомление не найдено"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске уведомления"})
		}
		return
	}

	now := time.Now()
	alert.Status = "resolved"
	alert.ResolvedAt = &now

	if err := api.DB.Save(&alert).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при обновлении уведомления"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Уведомление разрешено",
		"data":    alert,
	})
}

// GetWarehouseStatistics возвращает статистику склада
func (api *WarehouseAPI) GetWarehouseStatistics(c *gin.Context) {
	var stats struct {
		TotalEquipment     int64            `json:"total_equipment"`
		InStock            int64            `json:"in_stock"`
		Installed          int64            `json:"installed"`
		Reserved           int64            `json:"reserved"`
		Maintenance        int64            `json:"maintenance"`
		Broken             int64            `json:"broken"`
		LowStockCategories int64            `json:"low_stock_categories"`
		ActiveAlerts       int64            `json:"active_alerts"`
		RecentOperations   int64            `json:"recent_operations"`
		ByCategory         map[string]int64 `json:"by_category"`
		OperationsByType   map[string]int64 `json:"operations_by_type"`
		AlertsBySeverity   map[string]int64 `json:"alerts_by_severity"`
	}

	// Статистика по оборудованию
	api.DB.Model(&models.Equipment{}).Count(&stats.TotalEquipment)
	api.DB.Model(&models.Equipment{}).Where("status = 'in_stock'").Count(&stats.InStock)
	api.DB.Model(&models.Equipment{}).Where("status = 'installed'").Count(&stats.Installed)
	api.DB.Model(&models.Equipment{}).Where("status = 'reserved'").Count(&stats.Reserved)
	api.DB.Model(&models.Equipment{}).Where("status = 'maintenance'").Count(&stats.Maintenance)
	api.DB.Model(&models.Equipment{}).Where("status = 'broken'").Count(&stats.Broken)

	// Статистика по уведомлениям
	api.DB.Model(&models.StockAlert{}).Where("status = 'active'").Count(&stats.ActiveAlerts)

	// Недавние операции (за последние 7 дней)
	weekAgo := time.Now().AddDate(0, 0, -7)
	api.DB.Model(&models.WarehouseOperation{}).Where("created_at >= ?", weekAgo).Count(&stats.RecentOperations)

	// Статистика по категориям
	var categoryStats []struct {
		CategoryName string `json:"category_name"`
		Count        int64  `json:"count"`
	}
	api.DB.Table("equipment e").
		Joins("LEFT JOIN equipment_categories c ON e.category_id = c.id").
		Select("COALESCE(c.name, 'Без категории') as category_name, COUNT(*) as count").
		Group("c.name").
		Scan(&categoryStats)

	stats.ByCategory = make(map[string]int64)
	for _, cs := range categoryStats {
		stats.ByCategory[cs.CategoryName] = cs.Count
	}

	// Статистика операций по типам (за последний месяц)
	monthAgo := time.Now().AddDate(0, -1, 0)
	var operationStats []struct {
		Type  string `json:"type"`
		Count int64  `json:"count"`
	}
	api.DB.Model(&models.WarehouseOperation{}).
		Select("type, COUNT(*) as count").
		Where("created_at >= ?", monthAgo).
		Group("type").
		Scan(&operationStats)

	stats.OperationsByType = make(map[string]int64)
	for _, os := range operationStats {
		stats.OperationsByType[os.Type] = os.Count
	}

	// Статистика уведомлений по важности
	var alertStats []struct {
		Severity string `json:"severity"`
		Count    int64  `json:"count"`
	}
	api.DB.Model(&models.StockAlert{}).
		Select("severity, COUNT(*) as count").
		Where("status = 'active'").
		Group("severity").
		Scan(&alertStats)

	stats.AlertsBySeverity = make(map[string]int64)
	for _, as := range alertStats {
		stats.AlertsBySeverity[as.Severity] = as.Count
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// TransferEquipment переносит оборудование между локациями
func (api *WarehouseAPI) TransferEquipment(c *gin.Context) {
	var transferData struct {
		EquipmentID  uint   `json:"equipment_id" binding:"required"`
		FromLocation string `json:"from_location"`
		ToLocation   string `json:"to_location" binding:"required"`
		Notes        string `json:"notes"`
		UserID       uint   `json:"user_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&transferData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	// Проверяем существование оборудования
	var equipment models.Equipment
	if err := api.DB.First(&equipment, transferData.EquipmentID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Оборудование не найдено"})
		return
	}

	// Создаем операцию перемещения
	operation := models.WarehouseOperation{
		Type:         "transfer",
		Description:  "Перемещение оборудования",
		EquipmentID:  transferData.EquipmentID,
		FromLocation: transferData.FromLocation,
		ToLocation:   transferData.ToLocation,
		UserID:       transferData.UserID,
		Notes:        transferData.Notes,
		Status:       "completed",
	}

	if err := api.DB.Create(&operation).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при создании операции: " + err.Error()})
		return
	}

	// Обновляем местоположение оборудования
	equipment.WarehouseLocation = transferData.ToLocation
	if err := api.DB.Save(&equipment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при обновлении оборудования"})
		return
	}

	// Загружаем связанные данные для ответа
	api.DB.Preload("Equipment").Preload("User").First(&operation, operation.ID)

	c.JSON(http.StatusOK, gin.H{
		"message": "Оборудование успешно перемещено",
		"data":    operation,
	})
}
