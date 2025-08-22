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

// CreateRoleRequest представляет запрос на создание роли
type CreateRoleRequest struct {
	Name        string `json:"name" binding:"required,min=2,max=100"`
	DisplayName string `json:"display_name" binding:"required,min=2,max=100"`
	Description string `json:"description" binding:"max=500"`
	Color       string `json:"color" binding:"omitempty,len=7"`
	Priority    int    `json:"priority" binding:"min=0"`
	IsActive    *bool  `json:"is_active"`
}

// UpdateRoleRequest представляет запрос на обновление роли
type UpdateRoleRequest struct {
	Name        string `json:"name" binding:"omitempty,min=2,max=100"`
	DisplayName string `json:"display_name" binding:"omitempty,min=2,max=100"`
	Description string `json:"description" binding:"max=500"`
	Color       string `json:"color" binding:"omitempty,len=7"`
	Priority    *int   `json:"priority" binding:"omitempty,min=0"`
	IsActive    *bool  `json:"is_active"`
}

// RolePermissionsRequest представляет запрос на обновление разрешений роли
type RolePermissionsRequest struct {
	PermissionIDs []uint `json:"permission_ids" binding:"required"`
}

// CreatePermissionRequest представляет запрос на создание разрешения
type CreatePermissionRequest struct {
	Name        string `json:"name" binding:"required,min=2,max=100"`
	DisplayName string `json:"display_name" binding:"required,min=2,max=100"`
	Description string `json:"description" binding:"max=500"`
	Resource    string `json:"resource" binding:"required,min=2,max=50"`
	Action      string `json:"action" binding:"required,min=2,max=50"`
	Category    string `json:"category" binding:"max=50"`
	IsActive    *bool  `json:"is_active"`
}

// GetRoles возвращает список ролей с фильтрацией и пагинацией
func GetRoles(c *gin.Context) {
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
	withPermissions := c.Query("with_permissions") == "true"

	// Построение запроса
	query := db.Model(&models.Role{})
	if withPermissions {
		query = query.Preload("Permissions")
	}

	// Фильтр по активности
	if active != "" {
		isActive := active == "true"
		query = query.Where("is_active = ?", isActive)
	}

	// Поиск по имени или описанию
	if search != "" {
		searchPattern := "%" + strings.ToLower(search) + "%"
		query = query.Where(
			"LOWER(name) LIKE ? OR LOWER(display_name) LIKE ? OR LOWER(description) LIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
	}

	// Подсчет общего количества
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to count roles: " + err.Error(),
		})
		return
	}

	// Сортировка по приоритету (убывание) и имени
	query = query.Order("priority DESC, name ASC")

	// Получение данных с пагинацией
	var roles []models.Role
	if err := query.Offset(offset).Limit(limit).Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch roles: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"items": roles,
			"total": total,
			"page":  page,
			"limit": limit,
			"pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetRole возвращает данные конкретной роли
func GetRole(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid role ID",
		})
		return
	}

	var role models.Role
	if err := db.Preload("Permissions").Preload("Users").First(&role, roleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Role not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch role: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   role,
	})
}

// CreateRole создает новую роль
func CreateRole(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	// Создаем роль
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	role := models.Role{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Color:       req.Color,
		Priority:    req.Priority,
		IsActive:    isActive,
		IsSystem:    false, // Пользовательские роли не являются системными
	}

	if err := db.Create(&role).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(strings.ToLower(err.Error()), "constraint") {
			c.JSON(http.StatusConflict, gin.H{
				"status": "error",
				"error":  "Role with this name already exists",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create role: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   role,
	})
}

// UpdateRole обновляет данные роли
func UpdateRole(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid role ID",
		})
		return
	}

	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	// Находим роль
	var role models.Role
	if err := db.First(&role, roleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Role not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch role: " + err.Error(),
		})
		return
	}

	// Проверяем, что роль не системная
	if role.IsSystem {
		c.JSON(http.StatusForbidden, gin.H{
			"status": "error",
			"error":  "Cannot modify system role",
		})
		return
	}

	// Обновляем поля
	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.DisplayName != "" {
		updates["display_name"] = req.DisplayName
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Color != "" {
		updates["color"] = req.Color
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if err := db.Model(&role).Updates(updates).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(strings.ToLower(err.Error()), "constraint") {
			c.JSON(http.StatusConflict, gin.H{
				"status": "error",
				"error":  "Role with this name already exists",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to update role: " + err.Error(),
		})
		return
	}

	// Загружаем обновленную роль
	if err := db.First(&role, role.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to load updated role: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   role,
	})
}

// DeleteRole удаляет роль (только если не системная и не используется)
func DeleteRole(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid role ID",
		})
		return
	}

	// Находим роль
	var role models.Role
	if err := db.First(&role, roleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Role not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch role: " + err.Error(),
		})
		return
	}

	// Проверяем, что роль не системная
	if role.IsSystem {
		c.JSON(http.StatusForbidden, gin.H{
			"status": "error",
			"error":  "Cannot delete system role",
		})
		return
	}

	// Проверяем, что роль не используется пользователями
	var userCount int64
	if err := db.Model(&models.User{}).Where("role_id = ?", roleID).Count(&userCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to check role usage: " + err.Error(),
		})
		return
	}

	if userCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"status": "error",
			"error":  "Cannot delete role: it is assigned to users",
		})
		return
	}

	// Soft delete
	if err := db.Delete(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to delete role: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Role deleted successfully",
	})
}

// UpdateRolePermissions обновляет разрешения роли
func UpdateRolePermissions(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	roleID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid role ID",
		})
		return
	}

	var req RolePermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	// Находим роль
	var role models.Role
	if err := db.First(&role, roleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Role not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch role: " + err.Error(),
		})
		return
	}

	// Проверяем, что все разрешения существуют
	var permissions []models.Permission
	if err := db.Where("id IN ?", req.PermissionIDs).Find(&permissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch permissions: " + err.Error(),
		})
		return
	}

	if len(permissions) != len(req.PermissionIDs) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Some permissions not found",
		})
		return
	}

	// Обновляем связи
	if err := db.Model(&role).Association("Permissions").Replace(permissions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to update role permissions: " + err.Error(),
		})
		return
	}

	// Загружаем роль с обновленными разрешениями
	if err := db.Preload("Permissions").First(&role, role.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to load updated role: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   role,
	})
}

// GetPermissions возвращает список разрешений с фильтрацией
func GetPermissions(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	// Параметры фильтрации
	resource := c.Query("resource")
	action := c.Query("action")
	category := c.Query("category")
	active := c.Query("active")
	search := c.Query("search")

	// Построение запроса
	query := db.Model(&models.Permission{})

	// Фильтры
	if resource != "" {
		query = query.Where("resource = ?", resource)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if active != "" {
		isActive := active == "true"
		query = query.Where("is_active = ?", isActive)
	}

	// Поиск
	if search != "" {
		searchPattern := "%" + strings.ToLower(search) + "%"
		query = query.Where(
			"LOWER(name) LIKE ? OR LOWER(display_name) LIKE ? OR LOWER(description) LIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
	}

	// Сортировка
	query = query.Order("resource ASC, action ASC, name ASC")

	var permissions []models.Permission
	if err := query.Find(&permissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch permissions: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   permissions,
	})
}

// CreatePermission создает новое разрешение
func CreatePermission(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	var req CreatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	// Создаем разрешение
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	permission := models.Permission{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Resource:    req.Resource,
		Action:      req.Action,
		Category:    req.Category,
		IsActive:    isActive,
	}

	if err := db.Create(&permission).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(strings.ToLower(err.Error()), "constraint") {
			c.JSON(http.StatusConflict, gin.H{
				"status": "error",
				"error":  "Permission with this name already exists",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create permission: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   permission,
	})
}
