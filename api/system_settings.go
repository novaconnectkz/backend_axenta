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

	// Синхронизируем настройки НДС с BillingSettings и Company
	if updateData.VATRatePreset != "" || updateData.VATRateCustom > 0 || updateData.DefaultTaxRate > 0 {
		// Вычисляем DefaultTaxRate на основе пресета (приоритет над default_tax_rate)
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
			// Если пресет не установлен, используем default_tax_rate или 20 по умолчанию
			if settings.DefaultTaxRate > 0 {
				taxRate = settings.DefaultTaxRate
			} else {
				taxRate = 20
			}
		}

		// Обновляем default_tax_rate в самих настройках для консистентности
		settings.DefaultTaxRate = taxRate
		
		// Сохраняем обновленный default_tax_rate обратно в system_settings
		if err := db.Model(&settings).Update("default_tax_rate", taxRate).Error; err != nil {
			fmt.Printf("⚠️ Ошибка обновления default_tax_rate в system_settings: %v\n", err)
		}

		// Синхронизируем с BillingSettings
		var billingSettings models.BillingSettings
		if err := db.Where("company_id = ? AND admin_account_id = ?", uint(companyID), adminAccountID).First(&billingSettings).Error; err == nil {
			billingSettings.VATRatePreset = settings.VATRatePreset
			billingSettings.VATRateCustom = decimal.NewFromFloat(settings.VATRateCustom)
			billingSettings.DefaultTaxRate = decimal.NewFromFloat(taxRate)
			billingSettings.TaxIncluded = settings.TaxIncluded

			if err := db.Save(&billingSettings).Error; err != nil {
				fmt.Printf("⚠️ Ошибка синхронизации настроек НДС с биллингом: %v\n", err)
			} else {
				fmt.Printf("✅ Настройки НДС синхронизированы с биллингом: preset=%s, rate=%.2f%%, included=%v\n", settings.VATRatePreset, taxRate, settings.TaxIncluded)
			}
		}

		// Синхронизируем с Company
		var company models.Company
		if err := db.Where("id = ?", uint(companyID)).First(&company).Error; err == nil {
			company.DefaultTaxRate = decimal.NewFromFloat(taxRate)
			company.TaxIncluded = settings.TaxIncluded

			if err := db.Save(&company).Error; err != nil {
				fmt.Printf("⚠️ Ошибка синхронизации настроек НДС с компанией: %v\n", err)
			} else {
				fmt.Printf("✅ Настройки НДС синхронизированы с компанией: rate=%.2f%%, included=%v\n", taxRate, settings.TaxIncluded)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   settings,
	})
}

