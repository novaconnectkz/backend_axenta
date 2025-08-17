package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend_axenta/models"
)

// LocationAPI представляет API для работы с локациями
type LocationAPI struct {
	DB *gorm.DB
}

// NewLocationAPI создает новый экземпляр LocationAPI
func NewLocationAPI(db *gorm.DB) *LocationAPI {
	return &LocationAPI{DB: db}
}

// CreateLocation создает новую локацию
func (api *LocationAPI) CreateLocation(c *gin.Context) {
	var location models.Location
	if err := c.ShouldBindJSON(&location); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	// Проверяем уникальность комбинации город+регион
	var existingLocation models.Location
	if err := api.DB.Where("city = ? AND region = ?", location.City, location.Region).First(&existingLocation).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Локация с таким названием уже существует"})
		return
	}

	// Устанавливаем значения по умолчанию
	if location.Country == "" {
		location.Country = "Россия"
	}
	if location.Timezone == "" {
		location.Timezone = "Europe/Moscow"
	}
	// Type и Importance не поддерживаются в текущей модели
	// TODO: Добавить поля Type и Importance в модель Location

	if err := api.DB.Create(&location).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при создании локации: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Локация успешно создана",
		"data":    location,
	})
}

// GetLocations возвращает список локаций с фильтрацией
func (api *LocationAPI) GetLocations(c *gin.Context) {
	var locations []models.Location
	query := api.DB.Model(&models.Location{})

	// Фильтры
	if region := c.Query("region"); region != "" {
		query = query.Where("region ILIKE ?", "%"+region+"%")
	}
	if country := c.Query("country"); country != "" {
		query = query.Where("country ILIKE ?", "%"+country+"%")
	}
	if locationType := c.Query("type"); locationType != "" {
		query = query.Where("type = ?", locationType)
	}
	if importance := c.Query("importance"); importance != "" {
		query = query.Where("importance = ?", importance)
	}
	if isActive := c.Query("is_active"); isActive != "" {
		if isActive == "true" {
			query = query.Where("is_active = true")
		} else if isActive == "false" {
			query = query.Where("is_active = false")
		}
	}

	// Поиск по названию города
	if search := c.Query("search"); search != "" {
		query = query.Where("city ILIKE ? OR region ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	// Фильтр по наличию координат
	if hasCoordinates := c.Query("has_coordinates"); hasCoordinates == "true" {
		query = query.Where("latitude IS NOT NULL AND longitude IS NOT NULL")
	} else if hasCoordinates == "false" {
		query = query.Where("latitude IS NULL OR longitude IS NULL")
	}

	// Сортировка
	sortBy := c.DefaultQuery("sort_by", "city")
	sortOrder := c.DefaultQuery("sort_order", "asc")
	query = query.Order(sortBy + " " + sortOrder)

	// Пагинация
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset := (page - 1) * limit

	// Подсчет общего количества
	var total int64
	query.Count(&total)

	if err := query.Limit(limit).Offset(offset).Find(&locations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении списка локаций"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": locations,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
			"pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetLocation возвращает информацию о конкретной локации
func (api *LocationAPI) GetLocation(c *gin.Context) {
	id := c.Param("id")
	var location models.Location

	if err := api.DB.Preload("Objects").Preload("Installations").First(&location, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Локация не найдена"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении локации"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": location})
}

// UpdateLocation обновляет информацию о локации
func (api *LocationAPI) UpdateLocation(c *gin.Context) {
	id := c.Param("id")
	var location models.Location

	if err := api.DB.First(&location, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Локация не найдена"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске локации"})
		}
		return
	}

	var updateData models.Location
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	// Проверяем уникальность при изменении названия
	if (updateData.City != "" && updateData.City != location.City) ||
		(updateData.Region != "" && updateData.Region != location.Region) {

		city := updateData.City
		if city == "" {
			city = location.City
		}
		region := updateData.Region
		if region == "" {
			region = location.Region
		}

		var existingLocation models.Location
		if err := api.DB.Where("city = ? AND region = ? AND id != ?", city, region, location.ID).
			First(&existingLocation).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Локация с таким названием уже существует"})
			return
		}
	}

	if err := api.DB.Model(&location).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при обновлении локации"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Локация успешно обновлена",
		"data":    location,
	})
}

// DeleteLocation удаляет локацию (мягкое удаление)
func (api *LocationAPI) DeleteLocation(c *gin.Context) {
	id := c.Param("id")
	var location models.Location

	if err := api.DB.First(&location, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Локация не найдена"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске локации"})
		}
		return
	}

	// Проверяем, есть ли связанные объекты
	var objectCount int64
	api.DB.Model(&models.Object{}).Where("location_id = ?", location.ID).Count(&objectCount)

	if objectCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":        "Нельзя удалить локацию, к которой привязаны объекты",
			"object_count": objectCount,
		})
		return
	}

	// Проверяем, есть ли связанные монтажи
	var installationCount int64
	api.DB.Model(&models.Installation{}).Where("location_id = ?", location.ID).Count(&installationCount)

	if installationCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":              "Нельзя удалить локацию, к которой привязаны монтажи",
			"installation_count": installationCount,
		})
		return
	}

	if err := api.DB.Delete(&location).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при удалении локации"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Локация успешно удалена"})
}

// DeactivateLocation деактивирует локацию
func (api *LocationAPI) DeactivateLocation(c *gin.Context) {
	id := c.Param("id")
	var location models.Location

	if err := api.DB.First(&location, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Локация не найдена"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске локации"})
		}
		return
	}

	location.IsActive = false

	if err := api.DB.Save(&location).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при деактивации локации"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Локация деактивирована",
		"data":    location,
	})
}

// ActivateLocation активирует локацию
func (api *LocationAPI) ActivateLocation(c *gin.Context) {
	id := c.Param("id")
	var location models.Location

	if err := api.DB.First(&location, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Локация не найдена"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске локации"})
		}
		return
	}

	location.IsActive = true

	if err := api.DB.Save(&location).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при активации локации"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Локация активирована",
		"data":    location,
	})
}

// GetLocationStatistics возвращает статистику по локациям
func (api *LocationAPI) GetLocationStatistics(c *gin.Context) {
	var stats struct {
		Total              int64 `json:"total"`
		Active             int64 `json:"active"`
		Inactive           int64 `json:"inactive"`
		WithObjects        int64 `json:"with_objects"`
		WithoutObjects     int64 `json:"without_objects"`
		WithCoordinates    int64 `json:"with_coordinates"`
		WithoutCoordinates int64 `json:"without_coordinates"`
		Cities             int64 `json:"cities"`
		Towns              int64 `json:"towns"`
		Villages           int64 `json:"villages"`
	}

	// Общая статистика
	api.DB.Model(&models.Location{}).Count(&stats.Total)
	api.DB.Model(&models.Location{}).Where("is_active = true").Count(&stats.Active)
	api.DB.Model(&models.Location{}).Where("is_active = false").Count(&stats.Inactive)

	// По координатам
	api.DB.Model(&models.Location{}).Where("latitude IS NOT NULL AND longitude IS NOT NULL").Count(&stats.WithCoordinates)
	api.DB.Model(&models.Location{}).Where("latitude IS NULL OR longitude IS NULL").Count(&stats.WithoutCoordinates)

	// По типу
	api.DB.Model(&models.Location{}).Where("type = 'city'").Count(&stats.Cities)
	api.DB.Model(&models.Location{}).Where("type = 'town'").Count(&stats.Towns)
	api.DB.Model(&models.Location{}).Where("type = 'village'").Count(&stats.Villages)

	// Подсчет локаций с объектами
	api.DB.Raw(`
		SELECT COUNT(DISTINCT l.id) 
		FROM locations l 
		INNER JOIN objects o ON l.id = o.location_id
	`).Scan(&stats.WithObjects)

	stats.WithoutObjects = stats.Total - stats.WithObjects

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// GetLocationsByRegion возвращает локации, сгруппированные по регионам
func (api *LocationAPI) GetLocationsByRegion(c *gin.Context) {
	type RegionData struct {
		Region    string            `json:"region"`
		Count     int64             `json:"count"`
		Locations []models.Location `json:"locations"`
	}

	var regions []RegionData

	// Получаем список регионов с количеством локаций
	var regionCounts []struct {
		Region string `json:"region"`
		Count  int64  `json:"count"`
	}

	api.DB.Model(&models.Location{}).
		Select("region, COUNT(*) as count").
		Where("is_active = true").
		Group("region").
		Order("count DESC").
		Scan(&regionCounts)

	// Получаем локации для каждого региона
	for _, rc := range regionCounts {
		var locations []models.Location
		api.DB.Where("region = ? AND is_active = true", rc.Region).
			Order("city ASC").
			Find(&locations)

		regions = append(regions, RegionData{
			Region:    rc.Region,
			Count:     rc.Count,
			Locations: locations,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": regions})
}

// SearchLocations выполняет поиск локаций по различным критериям
func (api *LocationAPI) SearchLocations(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Параметр поиска 'q' обязателен"})
		return
	}

	var locations []models.Location

	// Поиск по городу, региону, почтовому индексу
	err := api.DB.Where("city ILIKE ? OR region ILIKE ? OR postal_code ILIKE ?",
		"%"+query+"%", "%"+query+"%", "%"+query+"%").
		Where("is_active = true").
		Order("city ASC").
		Limit(20).
		Find(&locations).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске локаций"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  locations,
		"query": query,
		"count": len(locations),
	})
}
