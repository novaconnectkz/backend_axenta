package api

import (
	"backend_axenta/database"
	"backend_axenta/services"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Состояние последнего ручного axenta-sync для FE-polling (как у других
// провайдеров inline-sync, но Axenta = async-фон, поэтому tracker отдельно).
// In-process (per-instance) — допустимо: trigger ручной и редкий, race
// между серверами не критичен (worst-case stale-state до next trigger).
var (
	axentaSyncMu       sync.RWMutex
	axentaSyncRunning  bool
	axentaSyncStartAt  time.Time
	axentaSyncFinishAt time.Time
	axentaSyncErrMsg   string
)

// TriggerAxentaSync запускает синхронизацию всех компаний вручную
// POST /api/auth/axenta-sync/trigger
// POST /api/test/axenta-sync/trigger (без авторизации для тестирования)
func TriggerAxentaSync(c *gin.Context) {
	axentaSyncMu.Lock()
	if axentaSyncRunning {
		axentaSyncMu.Unlock()
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "Синхронизация уже выполняется — дождитесь окончания.",
			"running": true,
		})
		return
	}
	axentaSyncRunning = true
	axentaSyncStartAt = time.Now()
	axentaSyncErrMsg = ""
	axentaSyncMu.Unlock()

	go func() {
		defer func() {
			axentaSyncMu.Lock()
			axentaSyncRunning = false
			axentaSyncFinishAt = time.Now()
			if r := recover(); r != nil {
				axentaSyncErrMsg = "panic during sync"
			}
			axentaSyncMu.Unlock()
		}()
		syncService := services.NewAxentaSyncService(database.DB)
		syncService.SyncAllAdmins()
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Запрос на синхронизацию всех компаний принят. Синхронизация выполняется в фоновом режиме.",
		"running": true,
	})
}

// GetAxentaSyncStatus отдаёт состояние последнего ручного axenta-sync
// для FE-polling. GET /api/auth/axenta-sync/status
func GetAxentaSyncStatus(c *gin.Context) {
	axentaSyncMu.RLock()
	defer axentaSyncMu.RUnlock()

	resp := gin.H{
		"running": axentaSyncRunning,
	}
	if !axentaSyncStartAt.IsZero() {
		resp["started_at"] = axentaSyncStartAt
	}
	if !axentaSyncFinishAt.IsZero() {
		resp["finished_at"] = axentaSyncFinishAt
		resp["duration_s"] = axentaSyncFinishAt.Sub(axentaSyncStartAt).Seconds()
	}
	if axentaSyncErrMsg != "" {
		resp["error"] = axentaSyncErrMsg
	}
	c.JSON(http.StatusOK, resp)
}
