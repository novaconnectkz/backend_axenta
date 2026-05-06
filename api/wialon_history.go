package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"backend_axenta/middleware"
	"backend_axenta/services"
)

// historyService — singleton сервис истории. Инициализируется в main.go через
// SetWialonHistoryService.
var historyService *services.WialonHistoryService

// SetWialonHistoryService устанавливает singleton (вызывается из main.go).
func SetWialonHistoryService(s *services.WialonHistoryService) {
	historyService = s
}

// GET /api/auth/wialon-history/settings
// Возвращает настройки истории для текущей company.
func GetWialonHistorySettings(c *gin.Context) {
	if historyService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "history service not initialized"})
		return
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no company context"})
		return
	}

	settings, err := historyService.GetSettings(companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	progress := historyService.GetProgress(companyID)

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"settings": settings,
			"progress": progress,
			"running":  historyService.IsRunning(companyID),
		},
	})
}

// PUT /api/auth/wialon-history/settings
// Body: {enabled: bool}
// Включает/выключает использование исторических данных.
func UpdateWialonHistorySettings(c *gin.Context) {
	if historyService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "history service not initialized"})
		return
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no company context"})
		return
	}

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := historyService.SetEnabled(companyID, body.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// POST /api/auth/wialon-history/backfill
// Body: {from: "2025-05-01", to: "2026-05-06"}
// Запускает асинхронный backfill. Если уже идёт — 409.
func StartWialonHistoryBackfill(c *gin.Context) {
	if historyService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "history service not initialized"})
		return
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no company context"})
		return
	}

	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	from, err := time.Parse("2006-01-02", body.From)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from date (YYYY-MM-DD)"})
		return
	}
	to, err := time.Parse("2006-01-02", body.To)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to date (YYYY-MM-DD)"})
		return
	}
	if from.After(to) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from must be <= to"})
		return
	}
	// Лимит — макс 2 года
	if to.Sub(from) > 366*2*24*time.Hour {
		c.JSON(http.StatusBadRequest, gin.H{"error": "период не может превышать 2 года"})
		return
	}

	if err := historyService.StartBackfill(companyID, from, to); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"status":  "success",
		"message": "backfill запущен",
	})
}

// GET /api/auth/wialon-history/progress
// Текущий прогресс backfill (для polling из UI).
func GetWialonHistoryProgress(c *gin.Context) {
	if historyService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "history service not initialized"})
		return
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no company context"})
		return
	}

	progress := historyService.GetProgress(companyID)
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"progress": progress,
			"running":  historyService.IsRunning(companyID),
		},
	})
}

// DELETE /api/auth/wialon-history/snapshots?from=&to=
// Стереть исторические данные за период.
func DeleteWialonHistorySnapshots(c *gin.Context) {
	if historyService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "history service not initialized"})
		return
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no company context"})
		return
	}

	from, err := time.Parse("2006-01-02", c.Query("from"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from"})
		return
	}
	to, err := time.Parse("2006-01-02", c.Query("to"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to"})
		return
	}

	rows, err := historyService.DeleteSnapshots(companyID, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"deleted": rows,
	})
}
