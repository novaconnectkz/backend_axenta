package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
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
	Settings    string `json:"settings"`
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

// GetUserTemplates возвращает список шаблонов пользователей с фильтрацией и пагинацией
func GetUserTemplates(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	// Параметры пагинации
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	offset := (page - 1) * limit

	// Параметры фильтрации
	active := c.Query("active_only")
	search := c.Query("search")

	// Построение запроса
	query := db.Model(&models.UserTemplate{}).Preload("Role")

	// Фильтр по активности
	if active != "" {
		isActive := active == "true"
		query = query.Where("is_active = ?", isActive)
	}

	// Поиск по имени или описанию
	if search != "" {
		searchPattern := "%" + strings.ToLower(search) + "%"
		query = query.Where(
			"LOWER(name) LIKE ? OR LOWER(description) LIKE ?",
			searchPattern, searchPattern,
		)
	}

	// Подсчет общего количества
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to count user templates: " + err.Error(),
		})
		return
	}

	// Сортировка по имени
	query = query.Order("name ASC")

	// Получение данных с пагинацией
	var templates []models.UserTemplate
	if err := query.Offset(offset).Limit(limit).Find(&templates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch user templates: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"items": templates,
			"total": total,
			"page":  page,
			"limit": limit,
			"pages": (total + int64(limit) - 1) / int64(limit),
		},
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
	if err := db.Preload("Role").First(&template, templateID).Error; err != nil {
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

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   template,
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
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(strings.ToLower(err.Error()), "constraint") {
			c.JSON(http.StatusConflict, gin.H{
				"status": "error",
				"error":  "User template with this name already exists",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create user template: " + err.Error(),
		})
		return
	}

	// Загружаем созданный шаблон с связями
	if err := db.Preload("Role").First(&template, template.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to load created template: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   template,
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
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(strings.ToLower(err.Error()), "constraint") {
			c.JSON(http.StatusConflict, gin.H{
				"status": "error",
				"error":  "User template with this name already exists",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to update user template: " + err.Error(),
		})
		return
	}

	// Загружаем обновленный шаблон с связями
	if err := db.Preload("Role").First(&template, template.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to load updated template: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   template,
	})
}

// DeleteUserTemplate удаляет шаблон пользователя (soft delete)
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
