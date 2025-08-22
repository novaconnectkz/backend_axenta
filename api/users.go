package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// CreateUserRequest представляет запрос на создание пользователя
type CreateUserRequest struct {
	Username   string `json:"username" binding:"required,min=3,max=50"`
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required,min=6,max=100"`
	FirstName  string `json:"first_name" binding:"max=50"`
	LastName   string `json:"last_name" binding:"max=50"`
	RoleID     uint   `json:"role_id" binding:"required,min=1"`
	TemplateID *uint  `json:"template_id"`
	IsActive   *bool  `json:"is_active"`
}

// UpdateUserRequest представляет запрос на обновление пользователя
type UpdateUserRequest struct {
	Username   string `json:"username" binding:"omitempty,min=3,max=50"`
	Email      string `json:"email" binding:"omitempty,email"`
	FirstName  string `json:"first_name" binding:"max=50"`
	LastName   string `json:"last_name" binding:"max=50"`
	RoleID     *uint  `json:"role_id" binding:"omitempty,min=1"`
	TemplateID *uint  `json:"template_id"`
	IsActive   *bool  `json:"is_active"`
}

// UserResponse представляет ответ с данными пользователя
type UserResponse struct {
	ID         uint                 `json:"id"`
	Username   string               `json:"username"`
	Email      string               `json:"email"`
	FirstName  string               `json:"first_name"`
	LastName   string               `json:"last_name"`
	IsActive   bool                 `json:"is_active"`
	RoleID     uint                 `json:"role_id"`
	Role       *models.Role         `json:"role,omitempty"`
	TemplateID *uint                `json:"template_id"`
	Template   *models.UserTemplate `json:"template,omitempty"`
	LastLogin  *string              `json:"last_login"`
	LoginCount int                  `json:"login_count"`
	CreatedAt  string               `json:"created_at"`
	UpdatedAt  string               `json:"updated_at"`
}

// GetUsers возвращает список пользователей с фильтрацией и пагинацией
func GetUsers(c *gin.Context) {
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
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Параметры фильтрации
	role := c.Query("role")
	active := c.Query("active")
	search := c.Query("search")

	// Построение запроса
	query := db.Model(&models.User{}).Preload("Role").Preload("Template")

	// Фильтр по роли
	if role != "" {
		query = query.Joins("JOIN roles ON users.role_id = roles.id").
			Where("roles.name = ?", role)
	}

	// Фильтр по активности
	if active != "" {
		isActive := active == "true"
		query = query.Where("is_active = ?", isActive)
	}

	// Поиск по имени, email или username
	if search != "" {
		searchPattern := "%" + strings.ToLower(search) + "%"
		query = query.Where(
			"LOWER(username) LIKE ? OR LOWER(email) LIKE ? OR LOWER(first_name) LIKE ? OR LOWER(last_name) LIKE ?",
			searchPattern, searchPattern, searchPattern, searchPattern,
		)
	}

	// Подсчет общего количества
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to count users: " + err.Error(),
		})
		return
	}

	// Получение данных с пагинацией
	var users []models.User
	if err := query.Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch users: " + err.Error(),
		})
		return
	}

	// Преобразование в response format
	userResponses := make([]UserResponse, len(users))
	for i, user := range users {
		userResponses[i] = UserResponse{
			ID:         user.ID,
			Username:   user.Username,
			Email:      user.Email,
			FirstName:  user.FirstName,
			LastName:   user.LastName,
			IsActive:   user.IsActive,
			RoleID:     user.RoleID,
			Role:       user.Role,
			TemplateID: user.TemplateID,
			Template:   user.Template,
			LoginCount: user.LoginCount,
			CreatedAt:  user.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:  user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if user.LastLogin != nil {
			lastLogin := user.LastLogin.Format("2006-01-02T15:04:05Z")
			userResponses[i].LastLogin = &lastLogin
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"items": userResponses,
			"total": total,
			"page":  page,
			"limit": limit,
			"pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetUser возвращает данные конкретного пользователя
func GetUser(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid user ID",
		})
		return
	}

	var user models.User
	if err := db.Preload("Role").Preload("Template").First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "User not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch user: " + err.Error(),
		})
		return
	}

	userResponse := UserResponse{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		FirstName:  user.FirstName,
		LastName:   user.LastName,
		IsActive:   user.IsActive,
		RoleID:     user.RoleID,
		Role:       user.Role,
		TemplateID: user.TemplateID,
		Template:   user.Template,
		LoginCount: user.LoginCount,
		CreatedAt:  user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:  user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if user.LastLogin != nil {
		lastLogin := user.LastLogin.Format("2006-01-02T15:04:05Z")
		userResponse.LastLogin = &lastLogin
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   userResponse,
	})
}

// CreateUser создает нового пользователя
func CreateUser(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	var req CreateUserRequest
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

	// Проверяем шаблон, если указан
	if req.TemplateID != nil {
		var template models.UserTemplate
		if err := db.First(&template, *req.TemplateID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "User template not found",
			})
			return
		}
	}

	// Хешируем пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to hash password",
		})
		return
	}

	// Создаем пользователя
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	user := models.User{
		Username:   req.Username,
		Email:      req.Email,
		Password:   string(hashedPassword),
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		IsActive:   isActive,
		RoleID:     req.RoleID,
		TemplateID: req.TemplateID,
		LoginCount: 0,
	}

	if err := db.Create(&user).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(strings.ToLower(err.Error()), "constraint") {
			c.JSON(http.StatusConflict, gin.H{
				"status": "error",
				"error":  "User with this username or email already exists",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create user: " + err.Error(),
		})
		return
	}

	// Загружаем созданного пользователя с связями
	if err := db.Preload("Role").Preload("Template").First(&user, user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to load created user: " + err.Error(),
		})
		return
	}

	userResponse := UserResponse{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		FirstName:  user.FirstName,
		LastName:   user.LastName,
		IsActive:   user.IsActive,
		RoleID:     user.RoleID,
		Role:       user.Role,
		TemplateID: user.TemplateID,
		Template:   user.Template,
		LoginCount: user.LoginCount,
		CreatedAt:  user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:  user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   userResponse,
	})
}

// UpdateUser обновляет данные пользователя
func UpdateUser(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid user ID",
		})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	// Находим пользователя
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "User not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch user: " + err.Error(),
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

	// Проверяем шаблон, если указан
	if req.TemplateID != nil {
		var template models.UserTemplate
		if err := db.First(&template, *req.TemplateID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "User template not found",
			})
			return
		}
	}

	// Обновляем поля
	updates := make(map[string]interface{})
	if req.Username != "" {
		updates["username"] = req.Username
	}
	if req.Email != "" {
		updates["email"] = req.Email
	}
	if req.FirstName != "" {
		updates["first_name"] = req.FirstName
	}
	if req.LastName != "" {
		updates["last_name"] = req.LastName
	}
	if req.RoleID != nil {
		updates["role_id"] = *req.RoleID
	}
	if req.TemplateID != nil {
		updates["template_id"] = *req.TemplateID
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if err := db.Model(&user).Updates(updates).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(strings.ToLower(err.Error()), "constraint") {
			c.JSON(http.StatusConflict, gin.H{
				"status": "error",
				"error":  "User with this username or email already exists",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to update user: " + err.Error(),
		})
		return
	}

	// Загружаем обновленного пользователя с связями
	if err := db.Preload("Role").Preload("Template").First(&user, user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to load updated user: " + err.Error(),
		})
		return
	}

	userResponse := UserResponse{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		FirstName:  user.FirstName,
		LastName:   user.LastName,
		IsActive:   user.IsActive,
		RoleID:     user.RoleID,
		Role:       user.Role,
		TemplateID: user.TemplateID,
		Template:   user.Template,
		LoginCount: user.LoginCount,
		CreatedAt:  user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:  user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if user.LastLogin != nil {
		lastLogin := user.LastLogin.Format("2006-01-02T15:04:05Z")
		userResponse.LastLogin = &lastLogin
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   userResponse,
	})
}

// GetUsersStats возвращает статистику пользователей
func GetUsersStats(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	// Подсчет общего количества пользователей
	var totalUsers int64
	if err := db.Model(&models.User{}).Count(&totalUsers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to count total users: " + err.Error(),
		})
		return
	}

	// Подсчет активных пользователей
	var activeUsers int64
	if err := db.Model(&models.User{}).Where("is_active = ?", true).Count(&activeUsers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to count active users: " + err.Error(),
		})
		return
	}

	// Подсчет неактивных пользователей
	inactiveUsers := totalUsers - activeUsers

	// Подсчет пользователей по ролям
	type RoleStats struct {
		RoleName string `json:"role_name"`
		Count    int64  `json:"count"`
	}

	var roleStats []RoleStats
	if err := db.Table("users").
		Select("roles.display_name as role_name, COUNT(users.id) as count").
		Joins("LEFT JOIN roles ON users.role_id = roles.id").
		Where("users.deleted_at IS NULL").
		Group("roles.id, roles.display_name").
		Scan(&roleStats).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to get role statistics: " + err.Error(),
		})
		return
	}

	// Подсчет пользователей, созданных за последние 30 дней
	var recentUsers int64
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	if err := db.Model(&models.User{}).
		Where("created_at >= ?", thirtyDaysAgo).
		Count(&recentUsers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to count recent users: " + err.Error(),
		})
		return
	}

	stats := gin.H{
		"total_users":    totalUsers,
		"active_users":   activeUsers,
		"inactive_users": inactiveUsers,
		"recent_users":   recentUsers,
		"role_stats":     roleStats,
		"last_updated":   time.Now().Format("2006-01-02T15:04:05Z"),
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// DeleteUser удаляет пользователя (soft delete)
func DeleteUser(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid user ID",
		})
		return
	}

	// Находим пользователя
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "User not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch user: " + err.Error(),
		})
		return
	}

	// Soft delete
	if err := db.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to delete user: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "User deleted successfully",
	})
}
