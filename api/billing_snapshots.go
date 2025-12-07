package api

import (
	"fmt"
	"net/http"
	"time"

	"backend_axenta/services"

	"github.com/gin-gonic/gin"
)

// BillingRebuildRequest описывает входные данные для пересчета истории
type BillingRebuildRequest struct {
	StartDate string `json:"start_date"` // YYYY-MM-DD, опционально
	EndDate   string `json:"end_date"`   // YYYY-MM-DD, опционально
}

var (
	defaultBillingStartDate = time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)
	defaultBillingEndDate   = time.Date(2025, 12, 6, 0, 0, 0, 0, time.UTC)
)

// RebuildBillingSnapshotsHandler запускает одноразовый пересчет ежедневных снимков
func RebuildBillingSnapshotsHandler(c *gin.Context) {
	var req BillingRebuildRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Не удалось прочитать тело запроса: " + err.Error(),
			})
			return
		}
	}

	startDate := defaultBillingStartDate
	endDate := defaultBillingEndDate

	if req.StartDate != "" {
		parsed, err := time.Parse("2006-01-02", req.StartDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Неверный формат start_date, ожидается YYYY-MM-DD",
			})
			return
		}
		startDate = parsed
	}

	if req.EndDate != "" {
		parsed, err := time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Неверный формат end_date, ожидается YYYY-MM-DD",
			})
			return
		}
		endDate = parsed
	}

	service := services.NewBillingSnapshotService()
	result, err := service.RebuildBillingSnapshots(startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("Пересчет выполнен: обработано %d дней, итоговое количество объектов %d", result.DaysProcessed, result.FinalTotal),
		"data": gin.H{
			"days_processed": result.DaysProcessed,
			"final_total":    result.FinalTotal,
			"last_date":      result.LastDate.Format("2006-01-02"),
		},
	})
}

// RunDailyBillingSnapshotHandler создает недостающие ежедневные снимки
func RunDailyBillingSnapshotHandler(c *gin.Context) {
	service := services.NewBillingSnapshotService()
	result, err := service.RunDailySnapshot()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("Создано снимков: %d, итоговое количество объектов %d", len(result.ProcessedDates), result.FinalTotal),
		"data": gin.H{
			"processed_dates": formatDates(result.ProcessedDates),
			"final_total":     result.FinalTotal,
		},
	})
}

func formatDates(dates []time.Time) []string {
	formatted := make([]string, 0, len(dates))
	for _, d := range dates {
		formatted = append(formatted, d.Format("2006-01-02"))
	}
	return formatted
}
