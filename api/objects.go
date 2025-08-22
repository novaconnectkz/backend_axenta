package api

import (
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// GetObjects получает список объектов для текущей компании
func GetObjects(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем параметры запроса
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	status := c.Query("status")
	objectType := c.Query("type")
	search := c.Query("search")

	// Базовый запрос
	query := tenantDB.Model(&models.Object{}).
		Preload("Contract").
		Preload("Template").
		Preload("Location")

	// Фильтры
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if objectType != "" {
		query = query.Where("type = ?", objectType)
	}
	if search != "" {
		query = query.Where("name ILIKE ? OR imei ILIKE ? OR phone_number ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// Подсчет общего количества
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подсчета объектов: " + err.Error()})
		return
	}

	// Получение объектов с пагинацией
	var objects []models.Object
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Find(&objects).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка получения объектов: " + err.Error()})
		return
	}

	// Формируем ответ
	response := gin.H{
		"status": "success",
		"data": gin.H{
			"items":       objects,
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": (total + int64(limit) - 1) / int64(limit),
		},
	}

	c.JSON(200, response)
}

// GetObject получает один объект по ID
func GetObject(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем ID объекта
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID объекта"})
		return
	}

	// Ищем объект
	var object models.Object
	if err := tenantDB.Preload("Contract").Preload("Template").Preload("Location").Preload("Equipment").First(&object, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"status": "error", "error": "Объект не найден"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка получения объекта: " + err.Error()})
		}
		return
	}

	c.JSON(200, gin.H{"status": "success", "data": object})
}

// CreateObject создает новый объект мониторинга
func CreateObject(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Парсим данные из запроса
	var object models.Object
	if err := c.ShouldBindJSON(&object); err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректные данные: " + err.Error()})
		return
	}

	// Валидация обязательных полей
	if object.Name == "" {
		c.JSON(400, gin.H{"status": "error", "error": "Название объекта обязательно"})
		return
	}
	if object.Type == "" {
		c.JSON(400, gin.H{"status": "error", "error": "Тип объекта обязателен"})
		return
	}
	if object.ContractID == 0 {
		c.JSON(400, gin.H{"status": "error", "error": "ID договора обязателен"})
		return
	}

	// Проверяем существование договора
	var contract models.Contract
	if err := tenantDB.First(&contract, object.ContractID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(400, gin.H{"status": "error", "error": "Договор не найден"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка проверки договора: " + err.Error()})
		}
		return
	}

	// Проверяем существование шаблона, если указан
	if object.TemplateID != nil {
		var template models.ObjectTemplate
		if err := tenantDB.First(&template, *object.TemplateID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(400, gin.H{"status": "error", "error": "Шаблон объекта не найден"})
			} else {
				c.JSON(500, gin.H{"status": "error", "error": "Ошибка проверки шаблона: " + err.Error()})
			}
			return
		}

		// Увеличиваем счетчик использований шаблона
		if err := template.IncrementUsage(tenantDB); err != nil {
			// Логируем ошибку, но не прерываем создание объекта
			// TODO: добавить логирование
		}
	}

	// Устанавливаем значения по умолчанию
	if object.Status == "" {
		object.Status = "active"
	}
	object.IsActive = true

	// Создаем объект
	if err := tenantDB.Create(&object).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка создания объекта: " + err.Error()})
		return
	}

	// Автоматически применяем тариф на основе договора
	if err := applyContractTariff(tenantDB, &object); err != nil {
		// Логируем ошибку, но не прерываем выполнение
		c.Header("X-Tariff-Warning", "Ошибка применения тарифа: "+err.Error())
	}

	// Загружаем созданный объект со всеми связями
	if err := tenantDB.Preload("Contract").Preload("Template").Preload("Location").First(&object, object.ID).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка загрузки созданного объекта: " + err.Error()})
		return
	}

	// Синхронизируем объект с Axetna.cloud асинхронно
	if integrationService := services.GetIntegrationService(); integrationService != nil {
		// Синхронизация временно отключена
		// if tenantID, exists := c.Get("tenant_id"); exists {
		// 	if tid, ok := tenantID.(uint); ok {
		// 		integrationService.SyncObjectAsync(tid, "create", &object)
		// 	}
		// }
	}

	c.JSON(201, gin.H{"status": "success", "data": object})
}

// UpdateObject обновляет существующий объект
func UpdateObject(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем ID объекта
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID объекта"})
		return
	}

	// Ищем существующий объект
	var existingObject models.Object
	if err := tenantDB.First(&existingObject, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"status": "error", "error": "Объект не найден"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка поиска объекта: " + err.Error()})
		}
		return
	}

	// Парсим данные из запроса
	var updates models.Object
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректные данные: " + err.Error()})
		return
	}

	// Валидация обязательных полей
	if updates.Name != "" && updates.Name != existingObject.Name {
		existingObject.Name = updates.Name
	}
	if updates.Type != "" && updates.Type != existingObject.Type {
		existingObject.Type = updates.Type
	}
	if updates.Description != existingObject.Description {
		existingObject.Description = updates.Description
	}
	if updates.Latitude != nil {
		existingObject.Latitude = updates.Latitude
	}
	if updates.Longitude != nil {
		existingObject.Longitude = updates.Longitude
	}
	if updates.Address != existingObject.Address {
		existingObject.Address = updates.Address
	}
	if updates.IMEI != "" && updates.IMEI != existingObject.IMEI {
		existingObject.IMEI = updates.IMEI
	}
	if updates.PhoneNumber != existingObject.PhoneNumber {
		existingObject.PhoneNumber = updates.PhoneNumber
	}
	if updates.SerialNumber != existingObject.SerialNumber {
		existingObject.SerialNumber = updates.SerialNumber
	}
	if updates.Status != "" && updates.Status != existingObject.Status {
		existingObject.Status = updates.Status
	}
	if updates.ContractID != 0 && updates.ContractID != existingObject.ContractID {
		// Проверяем существование нового договора
		var contract models.Contract
		if err := tenantDB.First(&contract, updates.ContractID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(400, gin.H{"status": "error", "error": "Договор не найден"})
			} else {
				c.JSON(500, gin.H{"status": "error", "error": "Ошибка проверки договора: " + err.Error()})
			}
			return
		}
		existingObject.ContractID = updates.ContractID
	}
	if updates.LocationID != 0 && updates.LocationID != existingObject.LocationID {
		existingObject.LocationID = updates.LocationID
	}
	if updates.TemplateID != nil && (existingObject.TemplateID == nil || *updates.TemplateID != *existingObject.TemplateID) {
		// Проверяем существование шаблона
		var template models.ObjectTemplate
		if err := tenantDB.First(&template, *updates.TemplateID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(400, gin.H{"status": "error", "error": "Шаблон объекта не найден"})
			} else {
				c.JSON(500, gin.H{"status": "error", "error": "Ошибка проверки шаблона: " + err.Error()})
			}
			return
		}
		existingObject.TemplateID = updates.TemplateID
	}
	if updates.Settings != "" && updates.Settings != existingObject.Settings {
		existingObject.Settings = updates.Settings
	}
	if len(updates.Tags) > 0 {
		existingObject.Tags = updates.Tags
	}
	if updates.Notes != existingObject.Notes {
		existingObject.Notes = updates.Notes
	}
	if updates.ExternalID != existingObject.ExternalID {
		existingObject.ExternalID = updates.ExternalID
	}

	// Обновляем время последнего изменения
	existingObject.UpdatedAt = time.Now()

	// Сохраняем изменения
	if err := tenantDB.Save(&existingObject).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка обновления объекта: " + err.Error()})
		return
	}

	// Загружаем обновленный объект со всеми связями
	if err := tenantDB.Preload("Contract").Preload("Template").Preload("Location").First(&existingObject, existingObject.ID).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка загрузки обновленного объекта: " + err.Error()})
		return
	}

	// Синхронизируем обновление объекта с Axetna.cloud асинхронно
	// Синхронизация временно отключена
	// if integrationService := services.GetIntegrationService(); integrationService != nil {
	// 	if tenantID, exists := c.Get("tenant_id"); exists {
	// 		if tid, ok := tenantID.(uint); ok {
	// 			integrationService.SyncObjectAsync(tid, "update", &existingObject)
	// 		}
	// 	}
	// }

	c.JSON(200, gin.H{"status": "success", "data": existingObject})
}

// DeleteObject удаляет объект (мягкое удаление)
func DeleteObject(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем ID объекта
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID объекта"})
		return
	}

	// Ищем объект
	var object models.Object
	if err := tenantDB.First(&object, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"status": "error", "error": "Объект не найден"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка поиска объекта: " + err.Error()})
		}
		return
	}

	// Синхронизация временно отключена
	// if integrationService := services.GetIntegrationService(); integrationService != nil {
	// 	if tenantID, exists := c.Get("tenant_id"); exists {
	// 		if tid, ok := tenantID.(uint); ok {
	// 			integrationService.SyncObjectAsync(tid, "delete", &object)
	// 		}
	// 	}
	// }

	// Мягкое удаление объекта
	if err := tenantDB.Delete(&object).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка удаления объекта: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{"status": "success", "message": "Объект успешно удален"})
}

// ScheduleObjectDelete планирует удаление объекта на указанную дату
func ScheduleObjectDelete(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем ID объекта
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID объекта"})
		return
	}

	// Парсим данные запроса
	var request struct {
		ScheduledDeleteAt string `json:"scheduled_delete_at" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректные данные: " + err.Error()})
		return
	}

	// Парсим дату
	scheduledDate, err := time.Parse("2006-01-02T15:04:05Z", request.ScheduledDeleteAt)
	if err != nil {
		// Пробуем другой формат
		scheduledDate, err = time.Parse("2006-01-02", request.ScheduledDeleteAt)
		if err != nil {
			c.JSON(400, gin.H{"status": "error", "error": "Некорректный формат даты. Используйте YYYY-MM-DD или RFC3339"})
			return
		}
	}

	// Проверяем, что дата в будущем
	if scheduledDate.Before(time.Now()) {
		c.JSON(400, gin.H{"status": "error", "error": "Дата планового удаления должна быть в будущем"})
		return
	}

	// Ищем объект
	var object models.Object
	if err := tenantDB.First(&object, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"status": "error", "error": "Объект не найден"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка поиска объекта: " + err.Error()})
		}
		return
	}

	// Устанавливаем плановое удаление
	object.ScheduledDeleteAt = &scheduledDate
	object.Status = "scheduled_delete"
	object.IsActive = false

	// Сохраняем изменения
	if err := tenantDB.Save(&object).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка планирования удаления: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Плановое удаление объекта запланировано",
		"data": gin.H{
			"id":                  object.ID,
			"scheduled_delete_at": object.ScheduledDeleteAt,
			"status":              object.Status,
		},
	})
}

// CancelScheduledDelete отменяет плановое удаление объекта
func CancelScheduledDelete(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем ID объекта
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID объекта"})
		return
	}

	// Ищем объект
	var object models.Object
	if err := tenantDB.First(&object, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"status": "error", "error": "Объект не найден"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка поиска объекта: " + err.Error()})
		}
		return
	}

	// Проверяем, что у объекта есть плановое удаление
	if object.ScheduledDeleteAt == nil {
		c.JSON(400, gin.H{"status": "error", "error": "У объекта нет запланированного удаления"})
		return
	}

	// Отменяем плановое удаление
	object.ScheduledDeleteAt = nil
	object.Status = "active"
	object.IsActive = true

	// Сохраняем изменения
	if err := tenantDB.Save(&object).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка отмены планового удаления: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Плановое удаление объекта отменено",
		"data": gin.H{
			"id":        object.ID,
			"status":    object.Status,
			"is_active": object.IsActive,
		},
	})
}

// GetDeletedObjects получает список удаленных объектов (корзина)
func GetDeletedObjects(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем параметры запроса
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	search := c.Query("search")

	// Базовый запрос для удаленных объектов
	query := tenantDB.Unscoped().Where("deleted_at IS NOT NULL").Model(&models.Object{}).
		Preload("Contract").
		Preload("Template").
		Preload("Location")

	// Фильтр поиска
	if search != "" {
		query = query.Where("name ILIKE ? OR imei ILIKE ? OR phone_number ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// Подсчет общего количества
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подсчета удаленных объектов: " + err.Error()})
		return
	}

	// Получение объектов с пагинацией
	var objects []models.Object
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Find(&objects).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка получения удаленных объектов: " + err.Error()})
		return
	}

	// Формируем ответ
	response := gin.H{
		"status": "success",
		"data": gin.H{
			"items":       objects,
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": (total + int64(limit) - 1) / int64(limit),
		},
	}

	c.JSON(200, response)
}

// RestoreObject восстанавливает удаленный объект из корзины
func RestoreObject(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем ID объекта
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID объекта"})
		return
	}

	// Ищем удаленный объект
	var object models.Object
	if err := tenantDB.Unscoped().Where("id = ? AND deleted_at IS NOT NULL", id).First(&object).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"status": "error", "error": "Удаленный объект не найден"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка поиска удаленного объекта: " + err.Error()})
		}
		return
	}

	// Восстанавливаем объект
	object.DeletedAt = gorm.DeletedAt{}
	object.Status = "active"
	object.IsActive = true

	if err := tenantDB.Unscoped().Save(&object).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка восстановления объекта: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Объект успешно восстановлен",
		"data": gin.H{
			"id":     object.ID,
			"name":   object.Name,
			"status": object.Status,
		},
	})
}

// PermanentDeleteObject окончательно удаляет объект из корзины
func PermanentDeleteObject(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем ID объекта
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID объекта"})
		return
	}

	// Ищем удаленный объект
	var object models.Object
	if err := tenantDB.Unscoped().Where("id = ? AND deleted_at IS NOT NULL", id).First(&object).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"status": "error", "error": "Удаленный объект не найден"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка поиска удаленного объекта: " + err.Error()})
		}
		return
	}

	// Окончательно удаляем объект
	if err := tenantDB.Unscoped().Delete(&object).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка окончательного удаления объекта: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{"status": "success", "message": "Объект окончательно удален"})
}

// GetObjectTemplates получает список шаблонов объектов
func GetObjectTemplates(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем параметры запроса
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	category := c.Query("category")
	search := c.Query("search")
	activeOnly := c.DefaultQuery("active_only", "true") == "true"

	// Базовый запрос
	query := tenantDB.Model(&models.ObjectTemplate{})

	// Фильтры
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if search != "" {
		query = query.Where("name ILIKE ? OR description ILIKE ?",
			"%"+search+"%", "%"+search+"%")
	}

	// Подсчет общего количества
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подсчета шаблонов: " + err.Error()})
		return
	}

	// Получение шаблонов с пагинацией
	var templates []models.ObjectTemplate
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Order("usage_count DESC, name ASC").Find(&templates).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка получения шаблонов: " + err.Error()})
		return
	}

	// Формируем ответ
	response := gin.H{
		"status": "success",
		"data": gin.H{
			"items":       templates,
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": (total + int64(limit) - 1) / int64(limit),
		},
	}

	c.JSON(200, response)
}

// GetObjectTemplate получает один шаблон объекта по ID
func GetObjectTemplate(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем ID шаблона
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID шаблона"})
		return
	}

	// Ищем шаблон
	var template models.ObjectTemplate
	if err := tenantDB.First(&template, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"status": "error", "error": "Шаблон объекта не найден"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка получения шаблона: " + err.Error()})
		}
		return
	}

	c.JSON(200, gin.H{"status": "success", "data": template})
}

// CreateObjectTemplate создает новый шаблон объекта
func CreateObjectTemplate(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Парсим данные из запроса
	var template models.ObjectTemplate
	if err := c.ShouldBindJSON(&template); err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректные данные: " + err.Error()})
		return
	}

	// Валидация обязательных полей
	if template.Name == "" {
		c.JSON(400, gin.H{"status": "error", "error": "Название шаблона обязательно"})
		return
	}
	if template.Category == "" {
		c.JSON(400, gin.H{"status": "error", "error": "Категория шаблона обязательна"})
		return
	}

	// Устанавливаем значения по умолчанию
	template.IsActive = true
	template.UsageCount = 0

	// Создаем шаблон
	if err := tenantDB.Create(&template).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка создания шаблона: " + err.Error()})
		return
	}

	c.JSON(201, gin.H{"status": "success", "data": template})
}

// UpdateObjectTemplate обновляет существующий шаблон объекта
func UpdateObjectTemplate(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем ID шаблона
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID шаблона"})
		return
	}

	// Ищем существующий шаблон
	var existingTemplate models.ObjectTemplate
	if err := tenantDB.First(&existingTemplate, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"status": "error", "error": "Шаблон объекта не найден"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка поиска шаблона: " + err.Error()})
		}
		return
	}

	// Проверяем, что это не системный шаблон
	if existingTemplate.IsSystem {
		c.JSON(400, gin.H{"status": "error", "error": "Системные шаблоны нельзя изменять"})
		return
	}

	// Парсим данные из запроса
	var updates models.ObjectTemplate
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректные данные: " + err.Error()})
		return
	}

	// Обновляем поля
	if updates.Name != "" && updates.Name != existingTemplate.Name {
		existingTemplate.Name = updates.Name
	}
	if updates.Description != existingTemplate.Description {
		existingTemplate.Description = updates.Description
	}
	if updates.Category != "" && updates.Category != existingTemplate.Category {
		existingTemplate.Category = updates.Category
	}
	if updates.Icon != existingTemplate.Icon {
		existingTemplate.Icon = updates.Icon
	}
	if updates.Color != existingTemplate.Color {
		existingTemplate.Color = updates.Color
	}
	if updates.Config != "" && updates.Config != existingTemplate.Config {
		existingTemplate.Config = updates.Config
	}
	if updates.DefaultSettings != "" && updates.DefaultSettings != existingTemplate.DefaultSettings {
		existingTemplate.DefaultSettings = updates.DefaultSettings
	}
	if len(updates.RequiredEquipment) > 0 {
		existingTemplate.RequiredEquipment = updates.RequiredEquipment
	}
	// IsActive можно изменить только для несистемных шаблонов
	existingTemplate.IsActive = updates.IsActive

	// Обновляем время последнего изменения
	existingTemplate.UpdatedAt = time.Now()

	// Сохраняем изменения
	if err := tenantDB.Save(&existingTemplate).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка обновления шаблона: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{"status": "success", "data": existingTemplate})
}

// DeleteObjectTemplate удаляет шаблон объекта (мягкое удаление)
func DeleteObjectTemplate(c *gin.Context) {
	// Получаем подключение к БД текущей компании
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	// Получаем ID шаблона
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID шаблона"})
		return
	}

	// Ищем шаблон
	var template models.ObjectTemplate
	if err := tenantDB.First(&template, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"status": "error", "error": "Шаблон объекта не найден"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка поиска шаблона: " + err.Error()})
		}
		return
	}

	// Проверяем, что это не системный шаблон
	if template.IsSystem {
		c.JSON(400, gin.H{"status": "error", "error": "Системные шаблоны нельзя удалять"})
		return
	}

	// Проверяем, не используется ли шаблон
	var objectCount int64
	if err := tenantDB.Model(&models.Object{}).Where("template_id = ?", template.ID).Count(&objectCount).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка проверки использования шаблона: " + err.Error()})
		return
	}

	if objectCount > 0 {
		c.JSON(400, gin.H{"status": "error", "error": "Шаблон используется объектами и не может быть удален"})
		return
	}

	// Мягкое удаление шаблона
	if err := tenantDB.Delete(&template).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка удаления шаблона: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{"status": "success", "message": "Шаблон объекта успешно удален"})
}

// applyContractTariff автоматически применяет тариф на основе договора объекта
func applyContractTariff(db *gorm.DB, object *models.Object) error {
	// Загружаем договор с тарифным планом
	var contract models.Contract
	if err := db.Preload("TariffPlan").First(&contract, object.ContractID).Error; err != nil {
		return err
	}

	// Если у договора нет тарифного плана, пропускаем
	if contract.TariffPlanID == 0 {
		return nil
	}

	// Получаем количество объектов по договору
	var totalObjects int64
	var activeObjects int64
	db.Model(&models.Object{}).Where("contract_id = ?", contract.ID).Count(&totalObjects)
	db.Model(&models.Object{}).Where("contract_id = ? AND is_active = ?", contract.ID, true).Count(&activeObjects)
	inactiveObjects := totalObjects - activeObjects

	// Создаем TariffPlan для расчета стоимости
	tariffPlan := models.TariffPlan{
		BillingPlan:        contract.TariffPlan,
		PricePerObject:     contract.TariffPlan.Price,
		InactivePriceRatio: decimal.NewFromFloat(0.5), // 50% для неактивных объектов
	}

	// Рассчитываем новую стоимость договора
	newCost := tariffPlan.CalculateObjectPrice(int(totalObjects), int(inactiveObjects))

	// Обновляем стоимость договора
	if err := db.Model(&contract).Update("total_amount", newCost).Error; err != nil {
		return err
	}

	return nil
}
