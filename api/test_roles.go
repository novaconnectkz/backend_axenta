package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// TestRolesCreation тестирует создание ролей в основной схеме (public)
func TestRolesCreation(c *gin.Context) {
	db := database.GetDB()
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	// Проверяем существующие роли
	var existingRoles []models.Role
	if err := db.Find(&existingRoles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to check existing roles: " + err.Error(),
		})
		return
	}

	// Создаем роли по умолчанию, если их нет
	axentaUserService := services.NewAxentaUserService(db)
	if err := axentaUserService.EnsureDefaultRoles(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create default roles: " + err.Error(),
		})
		return
	}

	// Получаем роли после создания
	var roles []models.Role
	if err := db.Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to get roles after creation: " + err.Error(),
		})
		return
	}

	// Тестируем маппинг ролей
	testResults := make([]gin.H, 0)
	testAccountTypes := []string{"partner", "client", "admin", "unknown", ""}

	for _, accountType := range testAccountTypes {
		role, roleData := getRoleByAxentaType(db, accountType)
		testResults = append(testResults, gin.H{
			"account_type": accountType,
			"found_role":   role != nil,
			"role_data":    roleData,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"existing_roles_count": len(existingRoles),
			"current_roles_count":  len(roles),
			"roles":                roles,
			"mapping_test":         testResults,
		},
	})
}

// TestUserWithRole тестирует получение пользователя с назначенной ролью
func TestUserWithRole(c *gin.Context) {
	db := database.GetDB()
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	// Создаем роли, если их нет
	axentaUserService := services.NewAxentaUserService(db)
	if err := axentaUserService.EnsureDefaultRoles(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to ensure roles: " + err.Error(),
		})
		return
	}

	// Симулируем пользователя из Axenta API
	mockAxentaUser := map[string]interface{}{
		"id":          12345,
		"username":    "test_partner",
		"name":        "Тестовый Партнер",
		"email":       "test@partner.com",
		"accountType": "partner",
		"isActive":    true,
		"lastLogin":   "2024-01-27T15:30:00Z",
	}

	// Определяем роль
	var roleInfo gin.H
	var roleID interface{} = 0

	accountType, ok := mockAxentaUser["accountType"].(string)
	if ok {
		role, roleData := getRoleByAxentaType(db, accountType)
		if role != nil {
			roleID = role.ID
			roleInfo = roleData
		}
	}

	// Формируем пользователя как в GetUsersFromAxentaCloud
	testUser := gin.H{
		"id":               mockAxentaUser["id"],
		"username":         mockAxentaUser["username"],
		"email":            mockAxentaUser["email"],
		"first_name":       mockAxentaUser["name"],
		"account_type":     mockAxentaUser["accountType"],
		"role_id":          roleID,
		"role":             roleInfo,
		"axenta_user_type": mapAccountTypeToAxentaType(mockAxentaUser["accountType"]),
		"is_axenta_user":   true,
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"test_user":    testUser,
			"role_found":   roleInfo != nil,
			"role_id":      roleID,
			"account_type": accountType,
		},
	})
}
