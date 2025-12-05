package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetSnapshotSettings получает настройки снимков
// GET /api/auth/snapshot-settings
func GetSnapshotSettings(c *gin.Context) {
	// Настройки хранятся в схеме первой компании (с минимальным ID) как суперадмина
	// Ищем первую компанию из списка
	mainDB := database.DB
	var companies []models.Company
	if err := mainDB.Table("public.companies").Order("id ASC").Limit(1).Find(&companies).Error; err != nil || len(companies) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Не найдено компаний в базе данных",
		})
		return
	}

	firstCompany := companies[0]
	superAdminDB := database.GetTenantDBByID(firstCompany.ID)
	if superAdminDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Database connection not available for company %d", firstCompany.ID),
		})
		return
	}

	var settings models.SnapshotSettings
	// Настройки хранятся для первой компании (как суперадмина) с company_id = 1 (глобальные настройки)
	// Используем FirstOrCreate для автоматического создания записи, если её нет
	err := superAdminDB.Where("company_id = ?", 1).FirstOrCreate(&settings, models.SnapshotSettings{
		CompanyID:   1, // Глобальные настройки с company_id = 1
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
	// Настройки хранятся в схеме первой компании (с минимальным ID) как суперадмина
	// Ищем первую компанию из списка
	mainDB := database.DB
	var companies []models.Company
	if err := mainDB.Table("public.companies").Order("id ASC").Limit(1).Find(&companies).Error; err != nil || len(companies) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Не найдено компаний в базе данных",
		})
		return
	}

	firstCompany := companies[0]
	superAdminDB := database.GetTenantDBByID(firstCompany.ID)
	if superAdminDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Database connection not available for company %d", firstCompany.ID),
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

	// Проверяем, существует ли запись в схеме первой компании с company_id = 1 (глобальные настройки)
	var settings models.SnapshotSettings
	err := superAdminDB.Where("company_id = ?", 1).First(&settings).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Создаем новую запись в схеме первой компании с company_id = 1 (глобальные настройки)
		settings = models.SnapshotSettings{
			CompanyID:   1, // Глобальные настройки с company_id = 1
			AxentaToken: request.AxentaToken,
			IsActive:    true,
		}
		if request.IsActive != nil {
			settings.IsActive = *request.IsActive
		}
		if err := superAdminDB.Create(&settings).Error; err != nil {
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
		// Обновляем существующую запись в схеме первой компании
		settings.AxentaToken = request.AxentaToken
		if request.IsActive != nil {
			settings.IsActive = *request.IsActive
		}
		if err := superAdminDB.Save(&settings).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Ошибка сохранения настроек: " + err.Error(),
			})
			return
		}
	}

	// Возвращаем полный токен в ответе (для подтверждения сохранения)
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Настройки успешно сохранены",
		"settings": gin.H{
			"id":           settings.ID,
			"company_id":   settings.CompanyID,
			"axenta_token": settings.AxentaToken, // Возвращаем полный токен для подтверждения
			"is_active":    settings.IsActive,
			"updated_at":   settings.UpdatedAt,
		},
	})
}
