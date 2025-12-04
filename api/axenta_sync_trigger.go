package api

import (
	"backend_axenta/database"
	"backend_axenta/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// TriggerAxentaSync запускает синхронизацию всех компаний вручную
// POST /api/auth/axenta-sync/trigger
// POST /api/test/axenta-sync/trigger (без авторизации для тестирования)
func TriggerAxentaSync(c *gin.Context) {
	// Запускаем синхронизацию в фоновом режиме
	go func() {
		syncService := services.NewAxentaSyncService(database.DB)
		syncService.SyncAllAdmins()
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Запрос на синхронизацию всех компаний принят. Синхронизация выполняется в фоновом режиме.",
	})
}

