package api

import (
	"log"
	"net/http"

	"backend_axenta/middleware"
	"backend_axenta/services"

	"github.com/gin-gonic/gin"
)

// GetAxentaSyncSettings получает настройки синхронизации AxentaSync
func GetAxentaSyncSettings(c *gin.Context) {
	// Получаем текущий интервал из планировщика
	scheduler := services.GetAxentaSyncScheduler()
	if scheduler == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Планировщик синхронизации не инициализирован",
		})
		return
	}

	interval := scheduler.GetInterval()

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"sync_interval": interval, // Интервал в минутах
		},
	})
}

// UpdateAxentaSyncSettings обновляет настройки синхронизации AxentaSync
func UpdateAxentaSyncSettings(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Проверяем, что пользователь авторизован (adminAccountID > 0)
	// Разрешаем изменять настройки всем авторизованным администраторам
	if adminAccountID == 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"status": "error",
			"error":  "Требуется авторизация администратора",
		})
		return
	}

	var req struct {
		SyncInterval int `json:"sync_interval" binding:"required,min=1,max=60"` // Интервал в минутах (1-60)
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных: " + err.Error(),
		})
		return
	}

	// Получаем планировщик
	scheduler := services.GetAxentaSyncScheduler()
	if scheduler == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Планировщик синхронизации не инициализирован",
		})
		return
	}

	// Обновляем интервал в планировщике
	if err := scheduler.UpdateInterval(req.SyncInterval); err != nil {
		log.Printf("❌ Ошибка обновления интервала синхронизации: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка обновления интервала: " + err.Error(),
		})
		return
	}

	log.Printf("✅ Интервал синхронизации AxentaSync обновлен на %d минут (admin_account_id=%d)", req.SyncInterval, adminAccountID)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Интервал синхронизации успешно обновлен",
		"data": gin.H{
			"sync_interval": req.SyncInterval,
		},
	})
}

