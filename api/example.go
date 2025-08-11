package api

import (
	"github.com/gin-gonic/gin"
)

// GetStatus возвращает статус системы
func GetStatus(c *gin.Context) {
	// Логика хендлера
	c.JSON(200, gin.H{
		"status": "success",
		"data": gin.H{
			"system":  "Axenta Backend",
			"version": "1.0.0",
			"health":  "ok",
		},
	})
}

// GetVersion возвращает версию API
func GetVersion(c *gin.Context) {
	// Логика хендлера
	c.JSON(200, gin.H{
		"status": "success",
		"data": gin.H{
			"api_version": "v1",
			"build":       "2025-01-27",
		},
	})
}

// HealthCheck проверка работоспособности
func HealthCheck(c *gin.Context) {
	// Логика хендлера
	c.JSON(200, gin.H{
		"status": "success",
		"data": gin.H{
			"alive":     true,
			"timestamp": "2025-01-27T15:30:00Z",
		},
	})
}
