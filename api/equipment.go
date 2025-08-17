package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend_axenta/models"
)

// EquipmentAPI представляет API для работы с оборудованием
type EquipmentAPI struct {
	DB *gorm.DB
}

// NewEquipmentAPI создает новый экземпляр EquipmentAPI
func NewEquipmentAPI(db *gorm.DB) *EquipmentAPI {
	return &EquipmentAPI{DB: db}
}

// CreateEquipment создает новое оборудование
func (api *EquipmentAPI) CreateEquipment(c *gin.Context) {
	var equipment models.Equipment
	if err := c.ShouldBindJSON(&equipment); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	// Проверяем уникальность серийного номера
	var existingEquipment models.Equipment
	if err := api.DB.Where("serial_number = ?", equipment.SerialNumber).First(&existingEquipment).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Оборудование с таким серийным номером уже существует"})
		return
	}

	// Проверяем уникальность IMEI если указан
	if equipment.IMEI != "" {
		if err := api.DB.Where("imei = ?", equipment.IMEI).First(&existingEquipment).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Оборудование с таким IMEI уже существует"})
			return
		}
	}

	// Проверяем уникальность номера телефона если указан
	if equipment.PhoneNumber != "" {
		if err := api.DB.Where("phone_number = ?", equipment.PhoneNumber).First(&existingEquipment).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Оборудование с таким номером телефона уже существует"})
			return
		}
	}

	// Проверяем уникальность QR кода если указан
	if equipment.QRCode != "" {
		if err := api.DB.Where("qr_code = ?", equipment.QRCode).First(&existingEquipment).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Оборудование с таким QR кодом уже существует"})
			return
		}
	}

	// Устанавливаем значения по умолчанию
	if equipment.Status == "" {
		equipment.Status = "in_stock"
	}
	if equipment.Condition == "" {
		equipment.Condition = "new"
	}

	// QR код не поддерживается в текущей модели
	// TODO: Добавить поддержку QR кода в модель Equipment

	if err := api.DB.Create(&equipment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при создании оборудования: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Оборудование успешно создано",
		"data":    equipment,
	})
}

// GetEquipment возвращает список оборудования с фильтрацией
func (api *EquipmentAPI) GetEquipment(c *gin.Context) {
	var equipment []models.Equipment
	query := api.DB.Preload("Object").Preload("Installations").Preload("Category")

	// Фильтры
	if equipmentType := c.Query("type"); equipmentType != "" {
		query = query.Where("type = ?", equipmentType)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if condition := c.Query("condition"); condition != "" {
		query = query.Where("condition = ?", condition)
	}
	if manufacturer := c.Query("manufacturer"); manufacturer != "" {
		query = query.Where("manufacturer ILIKE ?", "%"+manufacturer+"%")
	}
	if model := c.Query("model"); model != "" {
		query = query.Where("model ILIKE ?", "%"+model+"%")
	}

	// Поиск по различным полям
	if search := c.Query("search"); search != "" {
		query = query.Where("serial_number ILIKE ? OR imei ILIKE ? OR phone_number ILIKE ? OR model ILIKE ? OR qr_code ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// Фильтр по доступности
	if available := c.Query("available"); available == "true" {
		query = query.Where("status = 'in_stock' AND condition != 'broken'")
	}

	// Фильтр по необходимости обслуживания
	if needsMaintenance := c.Query("needs_maintenance"); needsMaintenance == "true" {
		query = query.Where("next_maintenance < NOW()")
	}

	// Сортировка
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	query = query.Order(sortBy + " " + sortOrder)

	// Пагинация
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset := (page - 1) * limit

	// Подсчет общего количества
	var total int64
	countQuery := api.DB.Model(&models.Equipment{})
	if equipmentType := c.Query("type"); equipmentType != "" {
		countQuery = countQuery.Where("type = ?", equipmentType)
	}
	if status := c.Query("status"); status != "" {
		countQuery = countQuery.Where("status = ?", status)
	}
	countQuery.Count(&total)

	if err := query.Limit(limit).Offset(offset).Find(&equipment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении списка оборудования"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": equipment,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
			"pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetEquipmentItem возвращает информацию о конкретном оборудовании
func (api *EquipmentAPI) GetEquipmentItem(c *gin.Context) {
	id := c.Param("id")
	var equipment models.Equipment

	if err := api.DB.Preload("Object").Preload("Installations").Preload("Category").First(&equipment, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Оборудование не найдено"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении оборудования"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": equipment})
}

// UpdateEquipment обновляет информацию об оборудовании
func (api *EquipmentAPI) UpdateEquipment(c *gin.Context) {
	id := c.Param("id")
	var equipment models.Equipment

	if err := api.DB.First(&equipment, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Оборудование не найдено"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске оборудования"})
		}
		return
	}

	var updateData models.Equipment
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	// Проверяем уникальность серийного номера при изменении
	if updateData.SerialNumber != "" && updateData.SerialNumber != equipment.SerialNumber {
		var existingEquipment models.Equipment
		if err := api.DB.Where("serial_number = ? AND id != ?", updateData.SerialNumber, equipment.ID).
			First(&existingEquipment).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Оборудование с таким серийным номером уже существует"})
			return
		}
	}

	// Проверяем уникальность IMEI при изменении
	if updateData.IMEI != "" && updateData.IMEI != equipment.IMEI {
		var existingEquipment models.Equipment
		if err := api.DB.Where("imei = ? AND id != ?", updateData.IMEI, equipment.ID).
			First(&existingEquipment).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Оборудование с таким IMEI уже существует"})
			return
		}
	}

	if err := api.DB.Model(&equipment).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при обновлении оборудования"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Оборудование успешно обновлено",
		"data":    equipment,
	})
}

// DeleteEquipment удаляет оборудование (мягкое удаление)
func (api *EquipmentAPI) DeleteEquipment(c *gin.Context) {
	id := c.Param("id")
	var equipment models.Equipment

	if err := api.DB.First(&equipment, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Оборудование не найдено"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске оборудования"})
		}
		return
	}

	// Проверяем, не установлено ли оборудование на объект
	if equipment.ObjectID != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Нельзя удалить оборудование, установленное на объект"})
		return
	}

	if err := api.DB.Delete(&equipment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при удалении оборудования"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Оборудование успешно удалено"})
}

// InstallEquipment устанавливает оборудование на объект
func (api *EquipmentAPI) InstallEquipment(c *gin.Context) {
	id := c.Param("id")
	var equipment models.Equipment

	if err := api.DB.First(&equipment, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Оборудование не найдено"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске оборудования"})
		}
		return
	}

	if !equipment.IsAvailable() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Оборудование недоступно для установки"})
		return
	}

	var installData struct {
		ObjectID uint `json:"object_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&installData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	// Проверяем существование объекта
	var object models.Object
	if err := api.DB.First(&object, installData.ObjectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Объект не найден"})
		return
	}

	equipment.ObjectID = &installData.ObjectID
	equipment.Status = "installed"

	if err := api.DB.Save(&equipment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при установке оборудования"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Оборудование успешно установлено",
		"data":    equipment,
	})
}

// UninstallEquipment снимает оборудование с объекта
func (api *EquipmentAPI) UninstallEquipment(c *gin.Context) {
	id := c.Param("id")
	var equipment models.Equipment

	if err := api.DB.First(&equipment, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Оборудование не найдено"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске оборудования"})
		}
		return
	}

	if equipment.Status != "installed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Оборудование не установлено"})
		return
	}

	equipment.ObjectID = nil
	equipment.Status = "in_stock"

	if err := api.DB.Save(&equipment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при снятии оборудования"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Оборудование успешно снято с объекта",
		"data":    equipment,
	})
}

// GetEquipmentStatistics возвращает статистику по оборудованию
func (api *EquipmentAPI) GetEquipmentStatistics(c *gin.Context) {
	var stats struct {
		Total            int64            `json:"total"`
		InStock          int64            `json:"in_stock"`
		Installed        int64            `json:"installed"`
		Maintenance      int64            `json:"maintenance"`
		Broken           int64            `json:"broken"`
		Retired          int64            `json:"retired"`
		NeedsMaintenance int64            `json:"needs_maintenance"`
		ExpiredWarranty  int64            `json:"expired_warranty"`
		ByType           map[string]int64 `json:"by_type"`
		ByManufacturer   map[string]int64 `json:"by_manufacturer"`
	}

	// Общая статистика
	api.DB.Model(&models.Equipment{}).Count(&stats.Total)
	api.DB.Model(&models.Equipment{}).Where("status = 'in_stock'").Count(&stats.InStock)
	api.DB.Model(&models.Equipment{}).Where("status = 'installed'").Count(&stats.Installed)
	api.DB.Model(&models.Equipment{}).Where("status = 'maintenance'").Count(&stats.Maintenance)
	api.DB.Model(&models.Equipment{}).Where("status = 'broken'").Count(&stats.Broken)
	api.DB.Model(&models.Equipment{}).Where("status = 'retired'").Count(&stats.Retired)

	// Оборудование, требующее обслуживания
	api.DB.Model(&models.Equipment{}).Where("next_maintenance < NOW()").Count(&stats.NeedsMaintenance)

	// Оборудование с истекшей гарантией
	api.DB.Model(&models.Equipment{}).Where("warranty_expiry < NOW()").Count(&stats.ExpiredWarranty)

	// Статистика по типам
	var typeStats []struct {
		Type  string `json:"type"`
		Count int64  `json:"count"`
	}
	api.DB.Model(&models.Equipment{}).Select("type, COUNT(*) as count").Group("type").Scan(&typeStats)

	stats.ByType = make(map[string]int64)
	for _, ts := range typeStats {
		stats.ByType[ts.Type] = ts.Count
	}

	// Статистика по производителям
	var manufacturerStats []struct {
		Manufacturer string `json:"manufacturer"`
		Count        int64  `json:"count"`
	}
	api.DB.Model(&models.Equipment{}).Select("manufacturer, COUNT(*) as count").Group("manufacturer").Scan(&manufacturerStats)

	stats.ByManufacturer = make(map[string]int64)
	for _, ms := range manufacturerStats {
		stats.ByManufacturer[ms.Manufacturer] = ms.Count
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// GetLowStockEquipment возвращает оборудование с низкими остатками
func (api *EquipmentAPI) GetLowStockEquipment(c *gin.Context) {
	threshold := c.DefaultQuery("threshold", "5")
	thresholdInt, _ := strconv.Atoi(threshold)

	type LowStockItem struct {
		Type         string `json:"type"`
		Model        string `json:"model"`
		Manufacturer string `json:"manufacturer"`
		InStock      int64  `json:"in_stock"`
		Total        int64  `json:"total"`
	}

	var lowStockItems []LowStockItem

	// Группируем по типу, модели и производителю
	api.DB.Model(&models.Equipment{}).
		Select("type, model, manufacturer, "+
			"COUNT(CASE WHEN status = 'in_stock' THEN 1 END) as in_stock, "+
			"COUNT(*) as total").
		Group("type, model, manufacturer").
		Having("COUNT(CASE WHEN status = 'in_stock' THEN 1 END) < ?", thresholdInt).
		Scan(&lowStockItems)

	c.JSON(http.StatusOK, gin.H{
		"data":      lowStockItems,
		"threshold": thresholdInt,
	})
}

// SearchEquipmentByQR ищет оборудование по QR коду
func (api *EquipmentAPI) SearchEquipmentByQR(c *gin.Context) {
	qrCode := c.Param("qr_code")
	var equipment models.Equipment

	if err := api.DB.Preload("Object").Preload("Category").Where("qr_code = ?", qrCode).First(&equipment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Оборудование с таким QR кодом не найдено"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске оборудования"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": equipment})
}
