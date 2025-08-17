package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CreateUserTemplateRequest представляет запрос на создание шаблона пользователя
type CreateUserTemplateRequest struct {
	Name        string `json:"name" binding:"required,min=2,max=100"`
	Description string `json:"description" binding:"max=500"`
	RoleID      uint   `json:"role_id" binding:"required,min=1"`
	Settings    string `json:"settings" binding:"omitempty"`
	IsActive    *bool  `json:"is_active"`
}

// UpdateUserTemplateRequest представляет запрос на обновление шаблона пользователя
type UpdateUserTemplateRequest struct {
	Name        string `json:"name" binding:"omitempty,min=2,max=100"`
	Description string `json:"description" binding:"max=500"`
	RoleID      *uint  `json:"role_id" binding:"omitempty,min=1"`
	Settings    string `json:"settings"`
	IsActive    *bool  `json:"is_active"`
}

// UserTemplateResponse представляет ответ с данными шаблона пользователя
type UserTemplateResponse struct {
	ID          uint        `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	RoleID      uint        `json:"role_id"`
	Role        models.Role `json:"role"`
	Settings    string      `json:"settings"`
	IsActive    bool        `json:"is_active"`
	UserCount   int64       `json:"user_count"`
	CreatedAt   string      `json:"created_at"`
	UpdatedAt   string      `json:"updated_at"`
}

// GetUserTemplates возвращает список шаблонов пользователей с фильтрацией
func GetUserTemplates(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	// Параметры фильтрации
	active := c.Query("active")
	roleID := c.Query("role_id")
	search := c.Query("search")
	withUsers := c.Query("with_users") == "true"

	// Построение запроса
	query := db.Model(&models.UserTemplate{}).Preload("Role")
	if withUsers {
		query = query.Preload("Users")
	}

	// Фильтр по активности
	if active != "" {
		isActive := active == "true"
		query = query.Where("is_active = ?", isActive)
	}

	// Фильтр по роли
	if roleID != "" {
		if id, err := strconv.ParseUint(roleID, 10, 32); err == nil {
			query = query.Where("role_id = ?", id)
		}
	}

	// Поиск по имени или описанию
	if search != "" {
		searchPattern := "%" + strings.ToLower(search) + "%"
		query = query.Where(
			"LOWER(name) LIKE ? OR LOWER(description) LIKE ?",
			searchPattern, searchPattern,
		)
	}

	// Сортировка по имени
	query = query.Order("name ASC")

	var templates []models.UserTemplate
	if err := query.Find(&templates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch user templates: " + err.Error(),
		})
		return
	}

	// Преобразование в response format с подсчетом пользователей
	templateResponses := make([]UserTemplateResponse, len(templates))
	for i, template := range templates {
		// Подсчитываем количество пользователей для каждого шаблона
		var userCount int64
		db.Model(&models.User{}).Where("template_id = ?", template.ID).Count(&userCount)

		templateResponses[i] = UserTemplateResponse{
			ID:          template.ID,
			Name:        template.Name,
			Description: template.Description,
			RoleID:      template.RoleID,
			Role:        template.Role,
			Settings:    template.Settings,
			IsActive:    template.IsActive,
			UserCount:   userCount,
			CreatedAt:   template.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   template.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   templateResponses,
	})
}

// GetUserTemplate возвращает данные конкретного шаблона пользователя
func GetUserTemplate(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	templateID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid template ID",
		})
		return
	}

	var template models.UserTemplate
	if err := db.Preload("Role").Preload("Users").First(&template, templateID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "User template not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch user template: " + err.Error(),
		})
		return
	}

	// Подсчитываем количество пользователей
	var userCount int64
	db.Model(&models.User{}).Where("template_id = ?", template.ID).Count(&userCount)

	templateResponse := UserTemplateResponse{
		ID:          template.ID,
		Name:        template.Name,
		Description: template.Description,
		RoleID:      template.RoleID,
		Role:        template.Role,
		Settings:    template.Settings,
		IsActive:    template.IsActive,
		UserCount:   userCount,
		CreatedAt:   template.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   template.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   templateResponse,
	})
}

// CreateUserTemplate создает новый шаблон пользователя
func CreateUserTemplate(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	var req CreateUserTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	// Проверяем, что роль существует
	var role models.Role
	if err := db.First(&role, req.RoleID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Role not found",
		})
		return
	}

	// Валидируем JSON настройки, если они указаны
	if req.Settings != "" {
		if !isValidJSON(req.Settings) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Invalid JSON format in settings",
			})
			return
		}
	}

	// Создаем шаблон
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	template := models.UserTemplate{
		Name:        req.Name,
		Description: req.Description,
		RoleID:      req.RoleID,
		Settings:    req.Settings,
		IsActive:    isActive,
	}

	if err := db.Create(&template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create user template: " + err.Error(),
		})
		return
	}

	// Загружаем созданный шаблон с ролью
	if err := db.Preload("Role").First(&template, template.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to load created template: " + err.Error(),
		})
		return
	}

	templateResponse := UserTemplateResponse{
		ID:          template.ID,
		Name:        template.Name,
		Description: template.Description,
		RoleID:      template.RoleID,
		Role:        template.Role,
		Settings:    template.Settings,
		IsActive:    template.IsActive,
		UserCount:   0, // Новый шаблон, пользователей еще нет
		CreatedAt:   template.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   template.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   templateResponse,
	})
}

// UpdateUserTemplate обновляет данные шаблона пользователя
func UpdateUserTemplate(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	templateID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid template ID",
		})
		return
	}

	var req UpdateUserTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	// Находим шаблон
	var template models.UserTemplate
	if err := db.First(&template, templateID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "User template not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch user template: " + err.Error(),
		})
		return
	}

	// Проверяем роль, если указана
	if req.RoleID != nil {
		var role models.Role
		if err := db.First(&role, *req.RoleID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Role not found",
			})
			return
		}
	}

	// Валидируем JSON настройки, если они указаны
	if req.Settings != "" {
		if !isValidJSON(req.Settings) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Invalid JSON format in settings",
			})
			return
		}
	}

	// Обновляем поля
	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.RoleID != nil {
		updates["role_id"] = *req.RoleID
	}
	if req.Settings != "" {
		updates["settings"] = req.Settings
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if err := db.Model(&template).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to update user template: " + err.Error(),
		})
		return
	}

	// Загружаем обновленный шаблон с ролью
	if err := db.Preload("Role").First(&template, template.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to load updated template: " + err.Error(),
		})
		return
	}

	// Подсчитываем количество пользователей
	var userCount int64
	db.Model(&models.User{}).Where("template_id = ?", template.ID).Count(&userCount)

	templateResponse := UserTemplateResponse{
		ID:          template.ID,
		Name:        template.Name,
		Description: template.Description,
		RoleID:      template.RoleID,
		Role:        template.Role,
		Settings:    template.Settings,
		IsActive:    template.IsActive,
		UserCount:   userCount,
		CreatedAt:   template.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   template.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   templateResponse,
	})
}

// DeleteUserTemplate удаляет шаблон пользователя (только если не используется)
func DeleteUserTemplate(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	templateID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid template ID",
		})
		return
	}

	// Находим шаблон
	var template models.UserTemplate
	if err := db.First(&template, templateID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "User template not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch user template: " + err.Error(),
		})
		return
	}

	// Проверяем, что шаблон не используется пользователями
	var userCount int64
	if err := db.Model(&models.User{}).Where("template_id = ?", templateID).Count(&userCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to check template usage: " + err.Error(),
		})
		return
	}

	if userCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"status": "error",
			"error":  "Cannot delete template: it is assigned to users",
		})
		return
	}

	// Soft delete
	if err := db.Delete(&template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to delete user template: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "User template deleted successfully",
	})
}

// isValidJSON проверяет, является ли строка валидным JSON
func isValidJSON(s string) bool {
	var js interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}
