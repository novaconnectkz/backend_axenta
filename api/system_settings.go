package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// GetSystemSettings получает системные настройки для компании
func GetSystemSettings(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		fmt.Printf("GetSystemSettings: ошибка получения admin_account_id: %v\n", err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyIDUint, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат company_id",
		})
		return
	}
	companyID := uint(companyIDUint)

	fmt.Printf("GetSystemSettings: запрос для admin_account_id=%d, company_id=%d\n", adminAccountID, companyID)

	db := database.DB.Session(&gorm.Session{})
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		fmt.Printf("GetSystemSettings: ошибка установки search_path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var settings models.SystemSettings
	if err := db.Where("company_id = ? AND admin_account_id = ?", companyID, adminAccountID).First(&settings).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			fmt.Printf("GetSystemSettings: ошибка при загрузке настроек: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Ошибка получения настроек системы",
			})
			return
		}

		fmt.Printf("GetSystemSettings: настройки не найдены, создаем новые для company_id=%d\n", companyID)

		// Настройки не найдены - создаем с дефолтными значениями
		settings = models.SystemSettings{
			AdminAccountID:               adminAccountID,
			CompanyID:                    companyID,
			CompanyName:                  "Моя компания",
			Timezone:                     "Europe/Moscow",
			DateFormat:                   "DD.MM.YYYY",
			Currency:                     "RUB",
			Language:                     "ru",
			Theme:                        "light",
			SessionTimeout:               480,
			PasswordMinLength:            8,
			PasswordRequireSpecial:       true,
			MaxLoginAttempts:             5,
			EmailNotificationsEnabled:    true,
			SmsNotificationsEnabled:      false,
			TelegramNotificationsEnabled: true,
			VATRatePreset:                "russia",
			VATRateCustom:                20,
			BackupEnabled:                true,
			BackupSchedule:               "0 2 * * *",
			BackupRetentionDays:          30,
		}

		if err := db.Create(&settings).Error; err != nil {
			fmt.Printf("GetSystemSettings: ошибка создания настроек: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Ошибка создания настроек системы",
			})
			return
		}

		fmt.Printf("GetSystemSettings: настройки успешно созданы для company_id=%d\n", companyID)
	}

	fmt.Printf("GetSystemSettings: возвращаем настройки для company_id=%d\n", companyID)
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   settings,
	})
}

// UpdateSystemSettings обновляет системные настройки
func UpdateSystemSettings(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат company_id",
		})
		return
	}

	db := database.DB.Session(&gorm.Session{})
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var settings models.SystemSettings
	if err := db.Where("company_id = ? AND admin_account_id = ?", uint(companyID), adminAccountID).First(&settings).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Настройки системы не найдены",
		})
		return
	}

	var updateData models.SystemSettings
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Обновляем настройки
	if err := db.Model(&settings).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при обновлении настроек системы",
		})
		return
	}

	// Синхронизируем настройки НДС с BillingSettings
	if updateData.VATRatePreset != "" || updateData.VATRateCustom > 0 {
		var billingSettings models.BillingSettings
		if err := db.Where("company_id = ? AND admin_account_id = ?", uint(companyID), adminAccountID).First(&billingSettings).Error; err == nil {
			// Вычисляем DefaultTaxRate на основе пресета
			var taxRate float64
			switch settings.VATRatePreset {
			case "russia":
				taxRate = 20
			case "kazakhstan":
				taxRate = 12
			case "none":
				taxRate = 0
			case "custom":
				taxRate = settings.VATRateCustom
			default:
				taxRate = 20
			}

			billingSettings.VATRatePreset = settings.VATRatePreset
			billingSettings.VATRateCustom = decimal.NewFromFloat(settings.VATRateCustom)
			billingSettings.DefaultTaxRate = decimal.NewFromFloat(taxRate)

			if err := db.Save(&billingSettings).Error; err != nil {
				fmt.Printf("⚠️ Ошибка синхронизации настроек НДС с биллингом: %v\n", err)
			} else {
				fmt.Printf("✅ Настройки НДС синхронизированы с биллингом: preset=%s, rate=%.2f%%\n", settings.VATRatePreset, taxRate)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   settings,
	})
}

