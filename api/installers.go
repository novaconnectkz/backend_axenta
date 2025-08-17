package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend_axenta/models"
)

// InstallerAPI представляет API для работы с монтажниками
type InstallerAPI struct {
	DB *gorm.DB
}

// NewInstallerAPI создает новый экземпляр InstallerAPI
func NewInstallerAPI(db *gorm.DB) *InstallerAPI {
	return &InstallerAPI{DB: db}
}

// CreateInstaller создает нового монтажника
func (api *InstallerAPI) CreateInstaller(c *gin.Context) {
	var installer models.Installer
	if err := c.ShouldBindJSON(&installer); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	// Проверяем уникальность email
	var existingInstaller models.Installer
	if err := api.DB.Where("email = ?", installer.Email).First(&existingInstaller).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Монтажник с таким email уже существует"})
		return
	}

	// Проверяем уникальность телефона
	if err := api.DB.Where("phone = ?", installer.Phone).First(&existingInstaller).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Монтажник с таким телефоном уже существует"})
		return
	}

	// Устанавливаем значения по умолчанию
	if installer.Rating == 0 {
		installer.Rating = 5.0
	}
	if installer.MaxDailyInstallations == 0 {
		installer.MaxDailyInstallations = 3
	}
	if installer.WorkingHoursStart == "" {
		installer.WorkingHoursStart = "09:00"
	}
	if installer.WorkingHoursEnd == "" {
		installer.WorkingHoursEnd = "18:00"
	}
	if installer.Status == "" {
		installer.Status = "available"
	}

	// Устанавливаем рабочие дни по умолчанию (понедельник-пятница)
	if len(installer.WorkingDays) == 0 {
		installer.WorkingDays = []int{1, 2, 3, 4, 5}
	}

	if err := api.DB.Create(&installer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при создании монтажника: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Монтажник успешно создан",
		"data":    installer,
	})
}

// GetInstallers возвращает список монтажников с фильтрацией
func (api *InstallerAPI) GetInstallers(c *gin.Context) {
	var installers []models.Installer
	query := api.DB.Model(&models.Installer{})

	// Фильтры
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if installerType := c.Query("type"); installerType != "" {
		query = query.Where("type = ?", installerType)
	}
	if isActive := c.Query("is_active"); isActive != "" {
		if isActive == "true" {
			query = query.Where("is_active = true")
		} else if isActive == "false" {
			query = query.Where("is_active = false")
		}
	}
	if city := c.Query("city"); city != "" {
		query = query.Where("city ILIKE ?", "%"+city+"%")
	}
	if specialization := c.Query("specialization"); specialization != "" {
		query = query.Where("? = ANY(specialization)", specialization)
	}

	// Поиск по имени
	if search := c.Query("search"); search != "" {
		query = query.Where("first_name ILIKE ? OR last_name ILIKE ? OR email ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
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
	query.Count(&total)

	if err := query.Limit(limit).Offset(offset).Find(&installers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении списка монтажников"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": installers,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
			"pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetInstaller возвращает информацию о конкретном монтажнике
func (api *InstallerAPI) GetInstaller(c *gin.Context) {
	id := c.Param("id")
	var installer models.Installer

	if err := api.DB.Preload("Installations").First(&installer, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Монтажник не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при получении монтажника"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": installer})
}

// UpdateInstaller обновляет информацию о монтажнике
func (api *InstallerAPI) UpdateInstaller(c *gin.Context) {
	id := c.Param("id")
	var installer models.Installer

	if err := api.DB.First(&installer, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Монтажник не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске монтажника"})
		}
		return
	}

	var updateData models.Installer
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные: " + err.Error()})
		return
	}

	// Проверяем уникальность email при изменении
	if updateData.Email != "" && updateData.Email != installer.Email {
		var existingInstaller models.Installer
		if err := api.DB.Where("email = ? AND id != ?", updateData.Email, installer.ID).First(&existingInstaller).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Монтажник с таким email уже существует"})
			return
		}
	}

	// Проверяем уникальность телефона при изменении
	if updateData.Phone != "" && updateData.Phone != installer.Phone {
		var existingInstaller models.Installer
		if err := api.DB.Where("phone = ? AND id != ?", updateData.Phone, installer.ID).First(&existingInstaller).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Монтажник с таким телефоном уже существует"})
			return
		}
	}

	if err := api.DB.Model(&installer).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при обновлении монтажника"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Монтажник успешно обновлен",
		"data":    installer,
	})
}

// DeleteInstaller удаляет монтажника (мягкое удаление)
func (api *InstallerAPI) DeleteInstaller(c *gin.Context) {
	id := c.Param("id")
	var installer models.Installer

	if err := api.DB.First(&installer, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Монтажник не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске монтажника"})
		}
		return
	}

	// Проверяем, есть ли активные монтажи
	var activeInstallations int64
	api.DB.Model(&models.Installation{}).Where("installer_id = ? AND status IN ('planned', 'in_progress')", installer.ID).Count(&activeInstallations)

	if activeInstallations > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":                "Нельзя удалить монтажника с активными монтажами",
			"active_installations": activeInstallations,
		})
		return
	}

	if err := api.DB.Delete(&installer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при удалении монтажника"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Монтажник успешно удален"})
}

// DeactivateInstaller деактивирует монтажника
func (api *InstallerAPI) DeactivateInstaller(c *gin.Context) {
	id := c.Param("id")
	var installer models.Installer

	if err := api.DB.First(&installer, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Монтажник не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске монтажника"})
		}
		return
	}

	installer.IsActive = false
	installer.Status = "unavailable"

	if err := api.DB.Save(&installer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при деактивации монтажника"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Монтажник деактивирован",
		"data":    installer,
	})
}

// ActivateInstaller активирует монтажника
func (api *InstallerAPI) ActivateInstaller(c *gin.Context) {
	id := c.Param("id")
	var installer models.Installer

	if err := api.DB.First(&installer, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Монтажник не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске монтажника"})
		}
		return
	}

	installer.IsActive = true
	installer.Status = "available"

	if err := api.DB.Save(&installer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при активации монтажника"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Монтажник активирован",
		"data":    installer,
	})
}

// GetAvailableInstallers возвращает доступных монтажников для указанной даты и локации
func (api *InstallerAPI) GetAvailableInstallers(c *gin.Context) {
	date := c.Query("date")
	locationID := c.Query("location_id")
	specialization := c.Query("specialization")

	if date == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Параметр date обязателен"})
		return
	}

	// Парсим дату
	// parsedDate, err := time.Parse("2006-01-02", date)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректный формат даты"})
	// 	return
	// }

	query := api.DB.Where("is_active = true AND status = 'available'")

	// Фильтр по локации
	if locationID != "" {
		if locID, err := strconv.Atoi(locationID); err == nil {
			query = query.Where("? = ANY(location_ids)", locID)
		}
	}

	// Фильтр по специализации
	if specialization != "" {
		query = query.Where("? = ANY(specialization)", specialization)
	}

	var installers []models.Installer
	if err := query.Find(&installers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при поиске доступных монтажников"})
		return
	}

	// TODO: Здесь можно добавить проверку занятости на конкретную дату
	// Для этого нужно проверить существующие монтажи на указанную дату

	c.JSON(http.StatusOK, gin.H{"data": installers})
}

// GetInstallerStatistics возвращает статистику по монтажникам
func (api *InstallerAPI) GetInstallerStatistics(c *gin.Context) {
	var stats struct {
		Total       int64 `json:"total"`
		Active      int64 `json:"active"`
		Inactive    int64 `json:"inactive"`
		Available   int64 `json:"available"`
		Busy        int64 `json:"busy"`
		OnVacation  int64 `json:"on_vacation"`
		Staff       int64 `json:"staff"`
		Contractors int64 `json:"contractors"`
		Partners    int64 `json:"partners"`
	}

	// Общая статистика
	api.DB.Model(&models.Installer{}).Count(&stats.Total)
	api.DB.Model(&models.Installer{}).Where("is_active = true").Count(&stats.Active)
	api.DB.Model(&models.Installer{}).Where("is_active = false").Count(&stats.Inactive)

	// По статусу
	api.DB.Model(&models.Installer{}).Where("status = 'available'").Count(&stats.Available)
	api.DB.Model(&models.Installer{}).Where("status = 'busy'").Count(&stats.Busy)
	api.DB.Model(&models.Installer{}).Where("status = 'vacation'").Count(&stats.OnVacation)

	// По типу
	api.DB.Model(&models.Installer{}).Where("type = 'штатный'").Count(&stats.Staff)
	api.DB.Model(&models.Installer{}).Where("type = 'наемный'").Count(&stats.Contractors)
	api.DB.Model(&models.Installer{}).Where("type = 'партнер'").Count(&stats.Partners)

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// GetInstallerWorkload возвращает загруженность монтажника
func (api *InstallerAPI) GetInstallerWorkload(c *gin.Context) {
	id := c.Param("id")

	var workload struct {
		InstallerID             uint    `json:"installer_id"`
		TotalInstallations      int64   `json:"total_installations"`
		CompletedInstallations  int64   `json:"completed_installations"`
		PlannedInstallations    int64   `json:"planned_installations"`
		InProgressInstallations int64   `json:"in_progress_installations"`
		CancelledInstallations  int64   `json:"cancelled_installations"`
		AverageRating           float64 `json:"average_rating"`
		ThisMonthInstallations  int64   `json:"this_month_installations"`
		ThisWeekInstallations   int64   `json:"this_week_installations"`
	}

	installerID, err := strconv.Atoi(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректный ID монтажника"})
		return
	}

	workload.InstallerID = uint(installerID)

	// Общая статистика
	api.DB.Model(&models.Installation{}).Where("installer_id = ?", installerID).Count(&workload.TotalInstallations)
	api.DB.Model(&models.Installation{}).Where("installer_id = ? AND status = 'completed'", installerID).Count(&workload.CompletedInstallations)
	api.DB.Model(&models.Installation{}).Where("installer_id = ? AND status = 'planned'", installerID).Count(&workload.PlannedInstallations)
	api.DB.Model(&models.Installation{}).Where("installer_id = ? AND status = 'in_progress'", installerID).Count(&workload.InProgressInstallations)
	api.DB.Model(&models.Installation{}).Where("installer_id = ? AND status = 'cancelled'", installerID).Count(&workload.CancelledInstallations)

	// TODO: Добавить расчет среднего рейтинга из отзывов клиентов
	workload.AverageRating = 5.0

	c.JSON(http.StatusOK, gin.H{"data": workload})
}
