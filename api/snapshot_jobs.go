package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"net/http"
	"strconv"

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

// TriggerManualSnapshot запускает создание снимков вручную (для тестирования)
// POST /api/auth/snapshot-jobs/trigger
func TriggerManualSnapshot(c *gin.Context) {
	// Создаем планировщик и запускаем вручную
	scheduler := services.NewPartnerSnapshotScheduler()
	
	// Запускаем в горутине чтобы не блокировать ответ
	go scheduler.RunManualSnapshot()
	
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Тестовый запуск создания снимков инициирован. Проверьте историю через несколько минут.",
	})
}

