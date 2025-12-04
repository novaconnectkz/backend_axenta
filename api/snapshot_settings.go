package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetSnapshotSettings получает настройки снимков
// GET /api/auth/snapshot-settings
func GetSnapshotSettings(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	var settings models.SnapshotSettings
	// Настройки хранятся для суперадмина (ID=1)
	// Используем FirstOrCreate для автоматического создания записи, если её нет
	err := db.Where("company_id = ?", 1).FirstOrCreate(&settings, models.SnapshotSettings{
		CompanyID:   1,
		AxentaToken: "",
		IsActive:    true,
	}).Error
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения настроек: " + err.Error(),
		})
		return
	}

	// Возвращаем полный токен для редактирования (доступ только у авторизованных пользователей)
	// В настройках снимков пользователь должен иметь возможность видеть и редактировать токен
	response := gin.H{
		"status": "success",
		"settings": gin.H{
			"id":           settings.ID,
			"company_id":   settings.CompanyID,
			"axenta_token": settings.AxentaToken, // Возвращаем полный токен для редактирования
			"is_active":    settings.IsActive,
			"created_at":   settings.CreatedAt,
			"updated_at":   settings.UpdatedAt,
		},
	}

	c.JSON(http.StatusOK, response)
}

// UpdateSnapshotSettings обновляет настройки снимков
// POST /api/auth/snapshot-settings
func UpdateSnapshotSettings(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	var request struct {
		AxentaToken string `json:"axenta_token" binding:"required"`
		IsActive    *bool  `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных: " + err.Error(),
		})
		return
	}

	// Проверяем, существует ли запись
	var settings models.SnapshotSettings
	err := db.Where("company_id = ?", 1).First(&settings).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Создаем новую запись
		settings = models.SnapshotSettings{
			CompanyID:   1,
			AxentaToken: request.AxentaToken,
			IsActive:    true,
		}
		if request.IsActive != nil {
			settings.IsActive = *request.IsActive
		}
		if err := db.Create(&settings).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Ошибка создания настроек: " + err.Error(),
			})
			return
		}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения настроек: " + err.Error(),
		})
		return
	} else {
		// Обновляем существующую запись
		settings.AxentaToken = request.AxentaToken
		if request.IsActive != nil {
			settings.IsActive = *request.IsActive
		}
		if err := db.Save(&settings).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Ошибка сохранения настроек: " + err.Error(),
			})
			return
		}
	}

	// Возвращаем полный токен в ответе (для подтверждения сохранения)
	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"message":  "Настройки успешно сохранены",
		"settings": gin.H{
			"id":           settings.ID,
			"company_id":   settings.CompanyID,
			"axenta_token": settings.AxentaToken, // Возвращаем полный токен для подтверждения
			"is_active":    settings.IsActive,
			"updated_at":   settings.UpdatedAt,
		},
	})
}

