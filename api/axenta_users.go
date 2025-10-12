package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// GetAxentaUsers возвращает пользователей по типу (partner, client, local, all)
func GetAxentaUsers(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	userType := c.DefaultQuery("type", "all")

	axentaUserService := services.NewAxentaUserService(db)
	users, err := axentaUserService.GetUsersByType(userType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to get users: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   users,
		"count":  len(users),
		"type":   userType,
	})
}

// CreateLocalUserRequest представляет запрос на создание локального пользователя
type CreateLocalUserRequest struct {
	Username  string `json:"username" binding:"required,min=3,max=50"`
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=6"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone"`
	RoleID    uint   `json:"role_id" binding:"required"`
}

// CreateLocalUser создает нового локального пользователя
func CreateLocalUser(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	var req CreateLocalUserRequest
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

	// Хешируем пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to hash password",
		})
		return
	}

	axentaUserService := services.NewAxentaUserService(db)
	user, err := axentaUserService.CreateLocalUser(req.Username, req.Email, string(hashedPassword), req.RoleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create user: " + err.Error(),
		})
		return
	}

	// Обновляем дополнительные поля
	user.FirstName = req.FirstName
	user.LastName = req.LastName
	user.Phone = req.Phone
	if err := db.Save(user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to update user details",
		})
		return
	}

	// Загружаем роль для ответа
	if err := db.Preload("Role").First(user, user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to load user role",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   user,
	})
}

// UpdateUserAxentaRoleRequest представляет запрос на обновление роли Axenta пользователя
type UpdateUserAxentaRoleRequest struct {
	AxentaUserType string `json:"axenta_user_type" binding:"required,oneof=partner client local"`
	AxentaUserID   string `json:"axenta_user_id"`
	IsAxentaUser   bool   `json:"is_axenta_user"`
}

// UpdateUserAxentaRole обновляет роль Axenta для пользователя
func UpdateUserAxentaRole(c *gin.Context) {
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

	var req UpdateUserAxentaRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	var user models.User
	if err := db.First(&user, uint(userID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "User not found",
		})
		return
	}

	// Обновляем роль Axenta
	if req.AxentaUserType == "local" || !req.IsAxentaUser {
		user.ClearAxentaRole()
	} else {
		user.SetAxentaRole(req.AxentaUserType, req.AxentaUserID)
	}

	if err := db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to update user: " + err.Error(),
		})
		return
	}

	// Загружаем роль для ответа
	if err := db.Preload("Role").First(&user, user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to load user role",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   user,
	})
}

// GetUsersByAxentaType возвращает статистику пользователей по типам Axenta
func GetUsersByAxentaType(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	axentaUserService := services.NewAxentaUserService(db)

	// Получаем статистику по типам
	partners, err := axentaUserService.GetUsersByType("partner")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to get partners: " + err.Error(),
		})
		return
	}

	clients, err := axentaUserService.GetUsersByType("client")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to get clients: " + err.Error(),
		})
		return
	}

	localUsers, err := axentaUserService.GetUsersByType("local")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to get local users: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"partners": gin.H{
				"count": len(partners),
				"users": partners,
			},
			"clients": gin.H{
				"count": len(clients),
				"users": clients,
			},
			"local": gin.H{
				"count": len(localUsers),
				"users": localUsers,
			},
			"total": len(partners) + len(clients) + len(localUsers),
		},
	})
}

// EnsureAxentaRoles создает роли по умолчанию для Axenta пользователей
func EnsureAxentaRoles(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	axentaUserService := services.NewAxentaUserService(db)
	if err := axentaUserService.EnsureDefaultRoles(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to ensure default roles: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Default Axenta roles ensured",
	})
}

// SyncUserWithAxentaRequest представляет запрос на синхронизацию пользователя с Axenta
type SyncUserWithAxentaRequest struct {
	Token    string `json:"token" binding:"required"`
	Username string `json:"username" binding:"required"`
}

// SyncUserWithAxenta синхронизирует пользователя с Axenta API
func SyncUserWithAxenta(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	var req SyncUserWithAxentaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	axentaUserService := services.NewAxentaUserService(db)
	user, err := axentaUserService.SyncUserWithAxenta(req.Token, req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to sync user with Axenta: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   user,
	})
}
