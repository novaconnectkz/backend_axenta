package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// GetSnapshotJobs возвращает список задач создания снимков
// GET /api/auth/snapshot-jobs?limit=50&offset=0&status=completed
func GetSnapshotJobs(c *gin.Context) {
	// Параметры пагинации
	limit := 50
	offset := 0
	
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Фильтры
	status := c.Query("status")     // completed, failed, partial, running
	jobType := c.Query("job_type")  // daily_auto, manual, scheduled

	// Базовый запрос
	query := database.DB.Model(&models.SnapshotJob{})

	// Применяем фильтры
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if jobType != "" {
		query = query.Where("job_type = ?", jobType)
	}

	// Получаем общее количество
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка подсчета записей"})
		return
	}

	// Получаем записи с пагинацией
	var jobs []models.SnapshotJob
	if err := query.
		Order("started_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения задач"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"jobs":   jobs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetSnapshotJob возвращает детальную информацию о конкретной задаче
// GET /api/auth/snapshot-jobs/:id
func GetSnapshotJob(c *gin.Context) {
	jobID := c.Param("id")

	var job models.SnapshotJob
	if err := database.DB.First(&job, jobID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Задача не найдена"})
		return
	}

	c.JSON(http.StatusOK, job)
}

// GetSnapshotJobStats возвращает статистику по задачам
// GET /api/auth/snapshot-jobs/stats
func GetSnapshotJobStats(c *gin.Context) {
	type Stats struct {
		TotalJobs         int64   `json:"total_jobs"`
		CompletedJobs     int64   `json:"completed_jobs"`
		FailedJobs        int64   `json:"failed_jobs"`
		PartialJobs       int64   `json:"partial_jobs"`
		RunningJobs       int64   `json:"running_jobs"`
		TotalSnapshots    int     `json:"total_snapshots"`
		TotalErrors       int     `json:"total_errors"`
		AvgDurationS      float64 `json:"avg_duration_s"`
		LastJobStartedAt  *string `json:"last_job_started_at,omitempty"`
	}

	stats := Stats{}

	// Общее количество задач
	database.DB.Model(&models.SnapshotJob{}).Count(&stats.TotalJobs)

	// По статусам
	database.DB.Model(&models.SnapshotJob{}).Where("status = ?", "completed").Count(&stats.CompletedJobs)
	database.DB.Model(&models.SnapshotJob{}).Where("status = ?", "failed").Count(&stats.FailedJobs)
	database.DB.Model(&models.SnapshotJob{}).Where("status = ?", "partial").Count(&stats.PartialJobs)
	database.DB.Model(&models.SnapshotJob{}).Where("status = ?", "running").Count(&stats.RunningJobs)

	// Суммарная статистика
	database.DB.Model(&models.SnapshotJob{}).Select("COALESCE(SUM(success_count), 0)").Scan(&stats.TotalSnapshots)
	database.DB.Model(&models.SnapshotJob{}).Select("COALESCE(SUM(error_count), 0)").Scan(&stats.TotalErrors)

	// Средняя длительность (только завершенных задач)
	database.DB.Model(&models.SnapshotJob{}).
		Where("finished_at IS NOT NULL AND duration_seconds IS NOT NULL").
		Select("COALESCE(AVG(duration_seconds), 0)").
		Scan(&stats.AvgDurationS)

	// Последняя задача
	var lastJob models.SnapshotJob
	if err := database.DB.Model(&models.SnapshotJob{}).
		Order("started_at DESC").
		First(&lastJob).Error; err == nil {
		lastJobTime := lastJob.StartedAt.Format("2006-01-02 15:04:05")
		stats.LastJobStartedAt = &lastJobTime
	}

	c.JSON(http.StatusOK, stats)
}

// GetLatestSnapshotJob возвращает последнюю задачу
// GET /api/auth/snapshot-jobs/latest
func GetLatestSnapshotJob(c *gin.Context) {
	var job models.SnapshotJob
	if err := database.DB.Order("started_at DESC").First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Задачи не найдены"})
		return
	}

	c.JSON(http.StatusOK, job)
}

// DeleteOldSnapshotJobs удаляет старые записи о задачах (старше N дней)
// DELETE /api/auth/snapshot-jobs/cleanup?days=90
func DeleteOldSnapshotJobs(c *gin.Context) {
	days := 90 // По умолчанию 90 дней
	
	if daysStr := c.Query("days"); daysStr != "" {
		if parsedDays, err := strconv.Atoi(daysStr); err == nil && parsedDays > 0 {
			days = parsedDays
		}
	}

	// Удаляем записи старше N дней
	result := database.DB.Where("started_at < ?", 
		database.DB.NowFunc().AddDate(0, 0, -days)).
		Delete(&models.SnapshotJob{})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления старых записей"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"deleted": result.RowsAffected,
		"message": "Удалены записи старше " + strconv.Itoa(days) + " дней",
	})
}

// TriggerManualSnapshotRequest представляет запрос на ручное создание снимков
type TriggerManualSnapshotRequest struct {
	Date     string `json:"date"`      // Опционально: одна дата в формате YYYY-MM-DD
	DateFrom string `json:"date_from"` // Опционально: начало периода в формате YYYY-MM-DD
	DateTo   string `json:"date_to"`   // Опционально: конец периода в формате YYYY-MM-DD
}

// TriggerManualSnapshot запускает создание снимков вручную (для тестирования)
// POST /api/auth/snapshot-jobs/trigger
// Поддерживает:
//   - Query параметр date (одна дата) - для обратной совместимости
//   - POST body с полями date, date_from и date_to
//   - Если ничего не указано - создает снимки за вчера
func TriggerManualSnapshot(c *gin.Context) {
	// Создаем планировщик
	scheduler := services.NewPartnerSnapshotScheduler()
	
	// Проверяем, есть ли данные в body
	var requestBody TriggerManualSnapshotRequest
	hasBody := false
	
	// Пытаемся прочитать body (может быть пустым)
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&requestBody); err == nil {
			hasBody = true
		}
	}
	
	// Определяем режим работы
	if hasBody {
		// Используем данные из body
		
		// Проверяем период (date_from и date_to)
		if requestBody.DateFrom != "" && requestBody.DateTo != "" {
			dateFrom, errFrom := time.Parse("2006-01-02", requestBody.DateFrom)
			dateTo, errTo := time.Parse("2006-01-02", requestBody.DateTo)
			
			if errFrom != nil || errTo != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"status":  "error",
					"message": "Неверный формат дат. Используйте YYYY-MM-DD (например: 2025-12-01)",
				})
				return
			}
			
			if dateFrom.After(dateTo) {
				c.JSON(http.StatusBadRequest, gin.H{
					"status":  "error",
					"message": "Дата начала не может быть позже даты окончания",
				})
				return
			}
			
			// Проверяем, что период не слишком большой (максимум 90 дней)
			daysDiff := int(dateTo.Sub(dateFrom).Hours() / 24)
			if daysDiff > 90 {
				c.JSON(http.StatusBadRequest, gin.H{
					"status":  "error",
					"message": fmt.Sprintf("Период не может превышать 90 дней. Выбранный период: %d дней", daysDiff),
				})
				return
			}
			
			// Запускаем создание снимков за период
			go scheduler.RunManualSnapshotForPeriod(dateFrom, dateTo)
			
			c.JSON(http.StatusOK, gin.H{
				"status":  "success",
				"message": fmt.Sprintf("Запрос на создание снимков за период %s - %s принят. Проверьте историю через несколько минут.", 
					requestBody.DateFrom, requestBody.DateTo),
			})
			return
		}
		
		// Проверяем одну дату
		if requestBody.Date != "" {
			targetDate, err := time.Parse("2006-01-02", requestBody.Date)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"status":  "error",
					"message": "Неверный формат даты. Используйте YYYY-MM-DD (например: 2025-12-02)",
				})
				return
			}
			
			// Запускаем снимки за указанную дату
			go scheduler.RunManualSnapshotForDate(targetDate)
			
			c.JSON(http.StatusOK, gin.H{
				"status":  "success",
				"message": fmt.Sprintf("Запрос на создание снимков за %s принят. Проверьте историю через несколько минут.", requestBody.Date),
			})
			return
		}
	}
	
	// Проверяем query параметр date (для обратной совместимости)
	dateParam := c.Query("date")
	if dateParam != "" {
		// Парсим дату из параметра
		targetDate, err := time.Parse("2006-01-02", dateParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Неверный формат даты. Используйте YYYY-MM-DD (например: 2025-12-02)",
			})
			return
		}
		
		// Запускаем снимки за указанную дату
		go scheduler.RunManualSnapshotForDate(targetDate)
		
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": fmt.Sprintf("Запрос на создание снимков за %s принят. Проверьте историю через несколько минут.", dateParam),
		})
		return
	}
	
	// По умолчанию - запускаем стандартный снимок (за вчера)
	go scheduler.RunManualSnapshot()
	
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Запрос на создание снимков за вчера принят. Проверьте историю через несколько минут.",
	})
}

