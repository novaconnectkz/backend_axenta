package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// getLatestObjectCreatedAt находит дату создания последнего объекта во всех компаниях
func getLatestObjectCreatedAt(publicDB *gorm.DB) (*time.Time, error) {
	var latestDate time.Time
	var found bool

	// Получаем все активные компании
	var companies []models.Company
	if err := publicDB.Find(&companies).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения компаний: %w", err)
	}

	// Ищем последний объект во всех компаниях
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		var object models.AxentaObjectSnapshot
		if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("axenta_created_at IS NOT NULL").
			Order("axenta_created_at DESC").
			First(&object).Error; err == nil && object.AxentaCreatedAt != nil {
			if !found || object.AxentaCreatedAt.After(latestDate) {
				latestDate = *object.AxentaCreatedAt
				found = true
			}
		}
	}

	if !found {
		return nil, nil
	}

	return &latestDate, nil
}

// getOrSetInitialBillingStartDate получает или устанавливает начальную дату биллинга
// Возвращает сохраненную дату, если она есть, иначе находит последний объект, сохраняет и возвращает
func getOrSetInitialBillingStartDate(_ *gorm.DB, tenantDB *gorm.DB) (*time.Time, error) {
	// Получаем настройки с company_id = 1 (глобальные настройки)
	var settings models.SnapshotSettings
	if err := tenantDB.Where("company_id = ?", 1).First(&settings).Error; err != nil {
		// Если настройки не найдены, создаем новые
		settings = models.SnapshotSettings{
			CompanyID: 1, // Глобальные настройки
			IsActive:  true,
		}
	}

	// Фиксированная дата старта биллинга: 2024-03-14 13:12:04 (UTC) — объект ID=8580
	fixedBillingStartDate := time.Date(2024, 3, 14, 13, 12, 4, 0, time.UTC)

	// Если начальная дата уже сохранена - проверяем, что она равна фиксированной дате
	if settings.InitialBillingStartDate != nil && !settings.InitialBillingStartDate.IsZero() {
		// Если сохраненная дата не равна фиксированной - перезаписываем
		if !settings.InitialBillingStartDate.Equal(fixedBillingStartDate) {
			log.Printf("📅 Сохраненная дата (%s) не соответствует фиксированной дате старта биллинга, обновляем на %s (UTC)",
				settings.InitialBillingStartDate.Format("2006-01-02 15:04:05"),
				fixedBillingStartDate.Format("2006-01-02 15:04:05"))
			settings.InitialBillingStartDate = &fixedBillingStartDate
			if err := tenantDB.Save(&settings).Error; err != nil {
				return nil, fmt.Errorf("ошибка обновления начальной даты биллинга: %w", err)
			}
		} else {
			log.Printf("📅 Используем сохраненную начальную дату биллинга: %s (UTC)",
				settings.InitialBillingStartDate.Format("2006-01-02 15:04:05"))
		}
		return settings.InitialBillingStartDate, nil
	}

	// Начальная дата не сохранена - устанавливаем фиксированную дату старта биллинга: 2024-03-14 13:12:04 (UTC) — объект ID=8580
	// Эта дата устанавливается один раз при первом запросе и больше не меняется
	log.Printf("📅 Начальная дата биллинга не найдена, устанавливаем фиксированную дату старта биллинга: %s (UTC) — объект ID=8580",
		fixedBillingStartDate.Format("2006-01-02 15:04:05"))
	settings.InitialBillingStartDate = &fixedBillingStartDate

	// Сохраняем настройки
	if err := tenantDB.Save(&settings).Error; err != nil {
		return nil, fmt.Errorf("ошибка сохранения начальной даты биллинга: %w", err)
	}

	log.Printf("✅ Начальная дата биллинга сохранена: %s (UTC)",
		settings.InitialBillingStartDate.Format("2006-01-02 15:04:05"))

	return settings.InitialBillingStartDate, nil
}

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
	status := c.Query("status")    // completed, failed, partial, running
	jobType := c.Query("job_type") // daily_auto, manual, scheduled

	// Таблица snapshot_jobs находится в схеме public (глобальная)
	// Переключаемся на схему public для чтения
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка переключения на схему public"})
		return
	}

	// Базовый запрос
	query := publicDB.Model(&models.SnapshotJob{})

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

	// Формируем ответ с дополнительным полем "billing_date" (Дата Биллинга)
	// Для типа "billing_start": используем начальную дату (устанавливается один раз при первом запросе и сохраняется в БД)
	// Для остальных типов: используем дату последнего созданного объекта (динамически)

	// Получаем первую активную компанию для работы с настройками
	var firstCompany models.Company
	if err := publicDB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		log.Printf("⚠️ Не найдено активных компаний для получения настроек")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Не найдено активных компаний"})
		return
	}

	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		log.Printf("⚠️ Не удалось получить tenant DB для компании %d", firstCompany.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения tenant DB"})
		return
	}

	// Получаем или устанавливаем начальную дату биллинга (для billing_start)
	// При первом запросе: находит последний объект и сохраняет дату в БД
	// При последующих запросах: использует сохраненную дату из БД
	initialBillingStartDate, err := getOrSetInitialBillingStartDate(publicDB, tenantDB)
	if err != nil {
		log.Printf("⚠️ Ошибка получения начальной даты биллинга: %v", err)
		// Используем дату по умолчанию
		defaultDate := time.Date(2024, 3, 14, 13, 12, 4, 0, time.UTC)
		initialBillingStartDate = &defaultDate
	}

	// Получаем дату последнего созданного объекта (больше не используется для billing_date, оставлено для совместимости)
	_, _ = getLatestObjectCreatedAt(publicDB)

	type JobResponse struct {
		ID                 uint        `json:"id"`
		CreatedAt          time.Time   `json:"created_at"`
		UpdatedAt          time.Time   `json:"updated_at"`
		JobType            string      `json:"job_type"`
		StartedAt          time.Time   `json:"started_at"`
		FinishedAt         *time.Time  `json:"finished_at,omitempty"`
		DurationSeconds    *int        `json:"duration_seconds,omitempty"`
		Status             string      `json:"status"`
		DateFrom           time.Time   `json:"date_from"`
		DateTo             time.Time   `json:"date_to"`
		TotalCompanies     int         `json:"total_companies"`
		TotalContracts     int         `json:"total_contracts"`
		TotalDaysProcessed int         `json:"total_days_processed"`
		SuccessCount       int         `json:"success_count"`
		ErrorCount         int         `json:"error_count"`
		SkippedCount       int         `json:"skipped_count"`
		TotalObjects       int         `json:"total_objects"`
		ActiveObjects      int         `json:"active_objects"`
		ScheduledTime      *time.Time  `json:"scheduled_time,omitempty"`
		ErrorMessage       string      `json:"error_message,omitempty"`
		Details            interface{} `json:"details,omitempty"`
		TriggeredBy        string      `json:"triggered_by,omitempty"`
		ServerInfo         interface{} `json:"server_info,omitempty"`
		BillingDate        *time.Time  `json:"billing_date"` // Дата Биллинга (всегда присутствует)
	}

	jobResponses := make([]JobResponse, len(jobs))
	for i, job := range jobs {
		jobResponses[i] = JobResponse{
			ID:                 job.ID,
			CreatedAt:          job.CreatedAt,
			UpdatedAt:          job.UpdatedAt,
			JobType:            string(job.JobType),
			StartedAt:          job.StartedAt,
			FinishedAt:         job.FinishedAt,
			DurationSeconds:    job.DurationSeconds,
			Status:             string(job.Status),
			DateFrom:           job.DateFrom,
			DateTo:             job.DateTo,
			TotalCompanies:     job.TotalCompanies,
			TotalContracts:     job.TotalContracts,
			TotalDaysProcessed: job.TotalDaysProcessed,
			SuccessCount:       job.SuccessCount,
			ErrorCount:         job.ErrorCount,
			SkippedCount:       job.SkippedCount,
			TotalObjects:       job.TotalObjects,
			ActiveObjects:      job.ActiveObjects,
			ScheduledTime:      job.ScheduledTime,
			ErrorMessage:       job.ErrorMessage,
			Details:            job.Details,
			TriggeredBy:        job.TriggeredBy,
			ServerInfo:         job.ServerInfo,
		}

		// Для типа "billing_start" используем начальную дату (сохраненную в БД один раз)
		if job.JobType == models.SnapshotJobTypeBillingStart {
			if initialBillingStartDate != nil {
				billingDateCopy := *initialBillingStartDate
				jobResponses[i].BillingDate = &billingDateCopy
				log.Printf("   ✅ Job ID=%d (billing_start): billing_date установлена на начальную дату %s (UTC)",
					job.ID, initialBillingStartDate.Format("2006-01-02 15:04:05"))
			} else {
				// Fallback: используем date_from
				dateFromCopy := job.DateFrom
				jobResponses[i].BillingDate = &dateFromCopy
			}
		} else {
			// Для остальных типов используем date_from (дату снимка, выбранную пользователем или установленную системой)
			// Это правильная дата биллинга, так как она соответствует дате, за которую создан снимок
			dateFromCopy := job.DateFrom
			jobResponses[i].BillingDate = &dateFromCopy
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"jobs":   jobResponses,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// triggerPartnerSnapshotsForAllContracts создает партнёрские снимки за период для всех активных партнёрских договоров тенанта
func triggerPartnerSnapshotsForAllContracts(c *gin.Context, body TriggerManualSnapshotRequest) error {
	// Получаем tenant DB из контекста
	tenantDBVal, exists := c.Get("tenant_db")
	if !exists {
		return fmt.Errorf("tenant DB не найдена (проверьте X-Tenant-ID)")
	}
	tenantDB := tenantDBVal.(*gorm.DB)

	// Токен Axenta
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return fmt.Errorf("отсутствует токен авторизации")
	}
	userToken := authHeader
	if len(authHeader) > 6 && authHeader[:6] == "Token " {
		userToken = authHeader[6:]
	}

	// Даты
	dateFromStr := body.DateFrom
	dateToStr := body.DateTo
	if body.Date != "" {
		dateFromStr = body.Date
		dateToStr = body.Date
	}

	dateFrom, err := time.Parse("2006-01-02", dateFromStr)
	if err != nil {
		return fmt.Errorf("неверный формат date_from/date (YYYY-MM-DD)")
	}
	dateTo, err := time.Parse("2006-01-02", dateToStr)
	if err != nil {
		return fmt.Errorf("неверный формат date_to (YYYY-MM-DD)")
	}
	if dateFrom.After(dateTo) {
		return fmt.Errorf("date_from не может быть позже date_to")
	}

	// Подготовка записи в snapshot_jobs
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		return fmt.Errorf("ошибка переключения схемы: %w", err)
	}

	job := &models.SnapshotJob{
		JobType:     models.SnapshotJobTypeManual,
		StartedAt:   time.Now(),
		DateFrom:    dateFrom,
		DateTo:      dateTo,
		Status:      models.SnapshotJobStatusRunning,
		TriggeredBy: "user",
	}
	if err := publicDB.Create(job).Error; err != nil {
		return fmt.Errorf("ошибка создания записи snapshot_job: %w", err)
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				job.AddError(models.JobError{
					Message: fmt.Sprintf("panic: %v", r),
					Date:    dateFrom.Format("2006-01-02"),
				})
				job.FinishJob(models.SnapshotJobStatusFailed, "panic в процессе генерации")
				_ = publicDB.Save(job).Error
			}
		}()

		// Получаем все активные партнёрские договоры.
		// Фильтр должен совпадать с auto-cron (partner_snapshot_scheduler.go) —
		// иначе manual backfill захватит draft-контракты, которых scheduler
		// не видит, и упрётся в unique-constraint (partner_company_id, snapshot_date),
		// потому что слот уже занят per-partner агрегатом (contract_id=0).
		var contracts []models.Contract
		if err := tenantDB.
			Where("contract_type = ? AND status = ?", "partner", "active").
			Where("partner_company_id IS NOT NULL").
			Find(&contracts).Error; err != nil {
			job.AddError(models.JobError{
				Message: fmt.Sprintf("ошибка получения договоров: %v", err),
			})
			job.FinishJob(models.SnapshotJobStatusFailed, "не удалось получить договоры")
			_ = publicDB.Save(job).Error
			return
		}

		if len(contracts) == 0 {
			job.SuccessCount = 0
			job.ErrorCount = 0
			job.TotalContracts = 0
			job.TotalDaysProcessed = 0
			job.FinishJob(models.SnapshotJobStatusCompleted, "")
			_ = publicDB.Save(job).Error
			return
		}

		snapshotService := services.NewPartnerSnapshotService()
		success := 0
		errorsCount := 0
		totalDays := 0

		// Определяем источник данных (по умолчанию "db" для быстрой работы)
		dataSource := body.DataSource
		if dataSource == "" {
			dataSource = "db" // По умолчанию используем БД
		}
		if dataSource != "db" && dataSource != "api" {
			dataSource = "db" // Если неверное значение, используем БД
		}

		skipped := 0
		for _, contract := range contracts {
			// На всякий случай пропускаем без партнёра
			if contract.PartnerCompanyID == nil {
				continue
			}
			for d := dateFrom; !d.After(dateTo); d = d.AddDate(0, 0, 1) {
				totalDays++
				if err := snapshotService.CreateSnapshotForContractWithTokenAndDBAndSource(&contract, d, userToken, tenantDB, dataSource); err != nil {
					// "snapshot already exists" — исторический снимок не пересоздаётся
					// (защита в CreateSnapshotForContractWithTokenAndDBAndSource).
					// Это нормальный исход backfill, а не ошибка → skipped.
					if err.Error() == "snapshot already exists" {
						skipped++
						continue
					}
					errorsCount++
					job.AddError(models.JobError{
						ContractID: contract.ID,
						Date:       d.Format("2006-01-02"),
						Message:    err.Error(),
					})
				} else {
					success++
				}
			}
		}

		job.TotalContracts = len(contracts)
		job.TotalDaysProcessed = totalDays
		job.SuccessCount = success
		job.ErrorCount = errorsCount
		job.SkippedCount = skipped

		status := models.SnapshotJobStatusCompleted
		errMsg := ""
		if errorsCount > 0 {
			status = models.SnapshotJobStatusPartial
			errMsg = "Часть снимков не создана"
		}
		job.FinishJob(status, errMsg)
		if err := publicDB.Save(job).Error; err != nil {
			log.Printf("⚠️ Ошибка сохранения snapshot_job: %v", err)
		}
	}()

	return nil
}

// GetSnapshotJob возвращает детальную информацию о конкретной задаче
// GET /api/auth/snapshot-jobs/:id
func GetSnapshotJob(c *gin.Context) {
	jobID := c.Param("id")

	// Таблица snapshot_jobs находится в схеме public
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка переключения на схему public"})
		return
	}

	var job models.SnapshotJob
	if err := publicDB.First(&job, jobID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Задача не найдена"})
		return
	}

	// Формируем ответ с полем "billing_date" (Дата Биллинга)
	// Для типа "billing_start": используем начальную дату (устанавливается один раз при первом запросе и сохраняется в БД)
	// Для остальных типов: используем дату последнего созданного объекта (динамически)

	// Получаем первую активную компанию для работы с настройками
	var firstCompany models.Company
	if err := publicDB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		log.Printf("⚠️ Не найдено активных компаний для получения настроек")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Не найдено активных компаний"})
		return
	}

	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		log.Printf("⚠️ Не удалось получить tenant DB для компании %d", firstCompany.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения tenant DB"})
		return
	}

	// Получаем или устанавливаем начальную дату биллинга (для billing_start)
	// При первом запросе: находит последний объект и сохраняет дату в БД
	// При последующих запросах: использует сохраненную дату из БД
	initialBillingStartDate, err := getOrSetInitialBillingStartDate(publicDB, tenantDB)
	if err != nil {
		log.Printf("⚠️ Ошибка получения начальной даты биллинга: %v", err)
		defaultDate := time.Date(2024, 3, 14, 13, 12, 4, 0, time.UTC)
		initialBillingStartDate = &defaultDate
	}

	// Получаем дату последнего созданного объекта (больше не используется для billing_date, оставлено для совместимости)
	_, _ = getLatestObjectCreatedAt(publicDB)

	type JobResponse struct {
		ID                 uint        `json:"id"`
		CreatedAt          time.Time   `json:"created_at"`
		UpdatedAt          time.Time   `json:"updated_at"`
		JobType            string      `json:"job_type"`
		StartedAt          time.Time   `json:"started_at"`
		FinishedAt         *time.Time  `json:"finished_at,omitempty"`
		DurationSeconds    *int        `json:"duration_seconds,omitempty"`
		Status             string      `json:"status"`
		DateFrom           time.Time   `json:"date_from"`
		DateTo             time.Time   `json:"date_to"`
		TotalCompanies     int         `json:"total_companies"`
		TotalContracts     int         `json:"total_contracts"`
		TotalDaysProcessed int         `json:"total_days_processed"`
		SuccessCount       int         `json:"success_count"`
		ErrorCount         int         `json:"error_count"`
		SkippedCount       int         `json:"skipped_count"`
		TotalObjects       int         `json:"total_objects"`
		ActiveObjects      int         `json:"active_objects"`
		ScheduledTime      *time.Time  `json:"scheduled_time,omitempty"`
		ErrorMessage       string      `json:"error_message,omitempty"`
		Details            interface{} `json:"details,omitempty"`
		TriggeredBy        string      `json:"triggered_by,omitempty"`
		ServerInfo         interface{} `json:"server_info,omitempty"`
		BillingDate        *time.Time  `json:"billing_date"` // Дата Биллинга (всегда присутствует)
	}

	response := JobResponse{
		ID:                 job.ID,
		CreatedAt:          job.CreatedAt,
		UpdatedAt:          job.UpdatedAt,
		JobType:            string(job.JobType),
		StartedAt:          job.StartedAt,
		FinishedAt:         job.FinishedAt,
		DurationSeconds:    job.DurationSeconds,
		Status:             string(job.Status),
		DateFrom:           job.DateFrom,
		DateTo:             job.DateTo,
		TotalCompanies:     job.TotalCompanies,
		TotalContracts:     job.TotalContracts,
		TotalDaysProcessed: job.TotalDaysProcessed,
		SuccessCount:       job.SuccessCount,
		ErrorCount:         job.ErrorCount,
		SkippedCount:       job.SkippedCount,
		TotalObjects:       job.TotalObjects,
		ActiveObjects:      job.ActiveObjects,
		ScheduledTime:      job.ScheduledTime,
		ErrorMessage:       job.ErrorMessage,
		Details:            job.Details,
		TriggeredBy:        job.TriggeredBy,
		ServerInfo:         job.ServerInfo,
	}

	// Для типа "billing_start" используем начальную дату (сохраненную в БД один раз)
	if job.JobType == models.SnapshotJobTypeBillingStart {
		if initialBillingStartDate != nil {
			billingDateCopy := *initialBillingStartDate
			response.BillingDate = &billingDateCopy
			log.Printf("   ✅ GetSnapshotJob: Job ID=%d (billing_start): billing_date установлена на начальную дату %s (UTC)",
				job.ID, initialBillingStartDate.Format("2006-01-02 15:04:05"))
		} else {
			// Fallback: используем date_from
			dateFromCopy := job.DateFrom
			response.BillingDate = &dateFromCopy
		}
	} else {
		// Для остальных типов используем date_from (дату снимка, выбранную пользователем или установленную системой)
		// Это правильная дата биллинга, так как она соответствует дате, за которую создан снимок
		dateFromCopy := job.DateFrom
		response.BillingDate = &dateFromCopy
	}

	c.JSON(http.StatusOK, response)
}

// GetSnapshotJobStats возвращает статистику по задачам
// GET /api/auth/snapshot-jobs/stats
func GetSnapshotJobStats(c *gin.Context) {
	// Таблица snapshot_jobs находится в схеме public
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка переключения на схему public"})
		return
	}

	type Stats struct {
		TotalJobs        int64   `json:"total_jobs"`
		CompletedJobs    int64   `json:"completed_jobs"`
		FailedJobs       int64   `json:"failed_jobs"`
		PartialJobs      int64   `json:"partial_jobs"`
		RunningJobs      int64   `json:"running_jobs"`
		TotalSnapshots   int     `json:"total_snapshots"`
		TotalErrors      int     `json:"total_errors"`
		AvgDurationS     float64 `json:"avg_duration_s"`
		LastJobStartedAt *string `json:"last_job_started_at,omitempty"`
	}

	stats := Stats{}

	// Общее количество задач
	publicDB.Model(&models.SnapshotJob{}).Count(&stats.TotalJobs)

	// По статусам
	publicDB.Model(&models.SnapshotJob{}).Where("status = ?", "completed").Count(&stats.CompletedJobs)
	publicDB.Model(&models.SnapshotJob{}).Where("status = ?", "failed").Count(&stats.FailedJobs)
	publicDB.Model(&models.SnapshotJob{}).Where("status = ?", "partial").Count(&stats.PartialJobs)
	publicDB.Model(&models.SnapshotJob{}).Where("status = ?", "running").Count(&stats.RunningJobs)

	// Суммарная статистика
	publicDB.Model(&models.SnapshotJob{}).Select("COALESCE(SUM(success_count), 0)").Scan(&stats.TotalSnapshots)
	publicDB.Model(&models.SnapshotJob{}).Select("COALESCE(SUM(error_count), 0)").Scan(&stats.TotalErrors)

	// Средняя длительность (только завершенных задач)
	publicDB.Model(&models.SnapshotJob{}).
		Where("finished_at IS NOT NULL AND duration_seconds IS NOT NULL").
		Select("COALESCE(AVG(duration_seconds), 0)").
		Scan(&stats.AvgDurationS)

	// Последняя задача
	var lastJob models.SnapshotJob
	if err := publicDB.Model(&models.SnapshotJob{}).
		Order("started_at DESC").
		First(&lastJob).Error; err == nil {
		// Форматируем время без изменения (часовой пояс будет применен на фронтенде)
		lastJobTime := lastJob.StartedAt.Format("2006-01-02 15:04:05")
		stats.LastJobStartedAt = &lastJobTime
	}

	c.JSON(http.StatusOK, stats)
}

// GetLatestSnapshotJob возвращает последнюю задачу
// GET /api/auth/snapshot-jobs/latest
func GetLatestSnapshotJob(c *gin.Context) {
	// Таблица snapshot_jobs находится в схеме public
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка переключения на схему public"})
		return
	}

	var job models.SnapshotJob
	if err := publicDB.Order("started_at DESC").First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Задачи не найдены"})
		return
	}

	// Формируем ответ с полем "billing_date" (Дата Биллинга)
	// Для типа "billing_start": используем начальную дату (устанавливается один раз при первом запросе и сохраняется в БД)
	// Для остальных типов: используем дату последнего созданного объекта (динамически)

	// Получаем первую активную компанию для работы с настройками
	var firstCompany models.Company
	if err := publicDB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		log.Printf("⚠️ Не найдено активных компаний для получения настроек")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Не найдено активных компаний"})
		return
	}

	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		log.Printf("⚠️ Не удалось получить tenant DB для компании %d", firstCompany.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения tenant DB"})
		return
	}

	// Получаем или устанавливаем начальную дату биллинга (для billing_start)
	// При первом запросе: находит последний объект и сохраняет дату в БД
	// При последующих запросах: использует сохраненную дату из БД
	initialBillingStartDate, err := getOrSetInitialBillingStartDate(publicDB, tenantDB)
	if err != nil {
		log.Printf("⚠️ Ошибка получения начальной даты биллинга: %v", err)
		defaultDate := time.Date(2024, 3, 14, 13, 12, 4, 0, time.UTC)
		initialBillingStartDate = &defaultDate
	}

	// Получаем дату последнего созданного объекта (больше не используется для billing_date, оставлено для совместимости)
	_, _ = getLatestObjectCreatedAt(publicDB)

	type JobResponse struct {
		ID                 uint        `json:"id"`
		CreatedAt          time.Time   `json:"created_at"`
		UpdatedAt          time.Time   `json:"updated_at"`
		JobType            string      `json:"job_type"`
		StartedAt          time.Time   `json:"started_at"`
		FinishedAt         *time.Time  `json:"finished_at,omitempty"`
		DurationSeconds    *int        `json:"duration_seconds,omitempty"`
		Status             string      `json:"status"`
		DateFrom           time.Time   `json:"date_from"`
		DateTo             time.Time   `json:"date_to"`
		TotalCompanies     int         `json:"total_companies"`
		TotalContracts     int         `json:"total_contracts"`
		TotalDaysProcessed int         `json:"total_days_processed"`
		SuccessCount       int         `json:"success_count"`
		ErrorCount         int         `json:"error_count"`
		SkippedCount       int         `json:"skipped_count"`
		TotalObjects       int         `json:"total_objects"`
		ActiveObjects      int         `json:"active_objects"`
		ScheduledTime      *time.Time  `json:"scheduled_time,omitempty"`
		ErrorMessage       string      `json:"error_message,omitempty"`
		Details            interface{} `json:"details,omitempty"`
		TriggeredBy        string      `json:"triggered_by,omitempty"`
		ServerInfo         interface{} `json:"server_info,omitempty"`
		BillingDate        *time.Time  `json:"billing_date"` // Дата Биллинга (всегда присутствует)
	}

	response := JobResponse{
		ID:                 job.ID,
		CreatedAt:          job.CreatedAt,
		UpdatedAt:          job.UpdatedAt,
		JobType:            string(job.JobType),
		StartedAt:          job.StartedAt,
		FinishedAt:         job.FinishedAt,
		DurationSeconds:    job.DurationSeconds,
		Status:             string(job.Status),
		DateFrom:           job.DateFrom,
		DateTo:             job.DateTo,
		TotalCompanies:     job.TotalCompanies,
		TotalContracts:     job.TotalContracts,
		TotalDaysProcessed: job.TotalDaysProcessed,
		SuccessCount:       job.SuccessCount,
		ErrorCount:         job.ErrorCount,
		SkippedCount:       job.SkippedCount,
		TotalObjects:       job.TotalObjects,
		ActiveObjects:      job.ActiveObjects,
		ScheduledTime:      job.ScheduledTime,
		ErrorMessage:       job.ErrorMessage,
		Details:            job.Details,
		TriggeredBy:        job.TriggeredBy,
		ServerInfo:         job.ServerInfo,
	}

	// Для типа "billing_start" используем начальную дату (сохраненную в БД один раз)
	if job.JobType == models.SnapshotJobTypeBillingStart {
		if initialBillingStartDate != nil {
			billingDateCopy := *initialBillingStartDate
			response.BillingDate = &billingDateCopy
			log.Printf("   ✅ GetLatestSnapshotJob: Job ID=%d (billing_start): billing_date установлена на начальную дату %s (UTC)",
				job.ID, initialBillingStartDate.Format("2006-01-02 15:04:05"))
		} else {
			// Fallback: используем date_from
			dateFromCopy := job.DateFrom
			response.BillingDate = &dateFromCopy
		}
	} else {
		// Для остальных типов используем date_from (дату снимка, выбранную пользователем или установленную системой)
		// Это правильная дата биллинга, так как она соответствует дате, за которую создан снимок
		dateFromCopy := job.DateFrom
		response.BillingDate = &dateFromCopy
	}

	c.JSON(http.StatusOK, response)
}

// ClearAllSnapshotHistory удаляет ВСЮ историю снимков (задачи и снимки партнеров)
// DELETE /api/auth/snapshot-jobs/clear-all
func ClearAllSnapshotHistory(c *gin.Context) {
	// Обработка паники
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ClearAllSnapshotHistory: ПАНИКА: %v", r)
			log.Printf("ClearAllSnapshotHistory: Стек паники: %+v", r)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Внутренняя ошибка сервера: %v", r),
			})
		}
	}()

	log.Printf("ClearAllSnapshotHistory: ===== НАЧАЛО ОЧИСТКИ ИСТОРИИ СНИМКОВ =====")
	log.Printf("ClearAllSnapshotHistory: Функция вызвана, начинаем обработку...")

	// Таблица snapshot_jobs находится в схеме public (глобальная)
	// Переключаемся на схему public для удаления
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("ClearAllSnapshotHistory: Ошибка переключения на схему public: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Ошибка переключения на схему public: " + err.Error(),
		})
		return
	}

	log.Printf("ClearAllSnapshotHistory: Переключились на схему public, проверяем наличие таблицы snapshot_jobs...")

	var jobsDeleted int64 = 0
	if !publicDB.Migrator().HasTable(&models.SnapshotJob{}) {
		log.Printf("ClearAllSnapshotHistory: Таблица snapshot_jobs не существует в схеме public, пропускаем удаление")
	} else {
		log.Printf("ClearAllSnapshotHistory: Таблица snapshot_jobs найдена, удаляем задачи...")
		// GORM требует условие WHERE при удалении, используем Where("1=1") для удаления всех записей
		result := publicDB.Unscoped().Where("1=1").Delete(&models.SnapshotJob{})
		if result.Error != nil {
			log.Printf("ClearAllSnapshotHistory: Ошибка удаления задач: %v", result.Error)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": "Ошибка удаления задач: " + result.Error.Error(),
			})
			return
		} else {
			jobsDeleted = result.RowsAffected
			log.Printf("ClearAllSnapshotHistory: Удалено задач из схемы public: %d", jobsDeleted)
		}
	}

	// Получаем все компании для очистки тенантных таблиц
	// Используем ту же сессию publicDB для получения списка компаний
	log.Printf("ClearAllSnapshotHistory: Получаем список компаний...")
	var companies []models.Company
	if err := publicDB.Find(&companies).Error; err != nil {
		log.Printf("ClearAllSnapshotHistory: Ошибка получения списка компаний: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Ошибка получения списка компаний: " + err.Error(),
		})
		return
	}
	log.Printf("ClearAllSnapshotHistory: Найдено компаний: %d", len(companies))

	totalSnapshotsDeleted := int64(0)
	totalObjectsDeleted := int64(0)
	totalAccountsDeleted := int64(0)

	// Очищаем тенантные таблицы для каждой компании
	log.Printf("ClearAllSnapshotHistory: Начинаем очистку тенантных таблиц...")
	for _, company := range companies {
		log.Printf("ClearAllSnapshotHistory: Обрабатываем компанию %d (%s, схема: %s)", company.ID, company.Name, company.DatabaseSchema)
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			log.Printf("ClearAllSnapshotHistory: Не удалось получить DB для компании %d (%s), пропускаем", company.ID, company.DatabaseSchema)
			continue
		}
		log.Printf("ClearAllSnapshotHistory: Получена DB для компании %d, начинаем удаление...", company.ID)

		// Удаляем partner_daily_snapshots
		// GORM требует условие WHERE при удалении, используем Where("1=1") для удаления всех записей
		result := tenantDB.Unscoped().Where("1=1").Delete(&models.PartnerDailySnapshot{})
		if result.Error != nil {
			// Логируем ошибку, но продолжаем (таблица может не существовать)
			log.Printf("ClearAllSnapshotHistory: Ошибка удаления partner_daily_snapshots для компании %d: %v", company.ID, result.Error)
		} else {
			totalSnapshotsDeleted += result.RowsAffected
			if result.RowsAffected > 0 {
				log.Printf("ClearAllSnapshotHistory: Удалено partner_daily_snapshots для компании %d: %d", company.ID, result.RowsAffected)
			}
		}

		// Опционально: удаляем axenta_object_snapshots и axenta_account_snapshots
		result = tenantDB.Unscoped().Where("1=1").Delete(&models.AxentaObjectSnapshot{})
		if result.Error != nil {
			// Логируем ошибку, но продолжаем (таблица может не существовать)
			log.Printf("ClearAllSnapshotHistory: Ошибка удаления axenta_object_snapshots для компании %d: %v", company.ID, result.Error)
		} else {
			totalObjectsDeleted += result.RowsAffected
			if result.RowsAffected > 0 {
				log.Printf("ClearAllSnapshotHistory: Удалено axenta_object_snapshots для компании %d: %d", company.ID, result.RowsAffected)
			}
		}

		result = tenantDB.Unscoped().Where("1=1").Delete(&models.AxentaAccountSnapshot{})
		if result.Error != nil {
			// Логируем ошибку, но продолжаем (таблица может не существовать)
			log.Printf("ClearAllSnapshotHistory: Ошибка удаления axenta_account_snapshots для компании %d: %v", company.ID, result.Error)
		} else {
			totalAccountsDeleted += result.RowsAffected
			if result.RowsAffected > 0 {
				log.Printf("ClearAllSnapshotHistory: Удалено axenta_account_snapshots для компании %d: %d", company.ID, result.RowsAffected)
			}
		}
	}

	log.Printf("ClearAllSnapshotHistory: Очистка завершена. Итого удалено: jobs=%d, partner_snapshots=%d, object_snapshots=%d, account_snapshots=%d",
		jobsDeleted, totalSnapshotsDeleted, totalObjectsDeleted, totalAccountsDeleted)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "История снимков полностью очищена",
		"deleted": gin.H{
			"jobs":              jobsDeleted,
			"partner_snapshots": totalSnapshotsDeleted,
			"object_snapshots":  totalObjectsDeleted,
			"account_snapshots": totalAccountsDeleted,
			"total":             jobsDeleted + totalSnapshotsDeleted + totalObjectsDeleted + totalAccountsDeleted,
		},
	})
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
	Date       string `json:"date"`        // Опционально: одна дата в формате YYYY-MM-DD
	DateFrom   string `json:"date_from"`   // Опционально: начало периода в формате YYYY-MM-DD
	DateTo     string `json:"date_to"`     // Опционально: конец периода в формате YYYY-MM-DD
	Mode       string `json:"mode"`        // Опционально: partner_all — создать партнёрские снимки по всем договорам за период
	DataSource string `json:"data_source"` // Опционально: "db" (из БД) или "api" (из Axenta API), по умолчанию "db"
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

	// Режим partner_all: генерируем партнёрские снимки для всех договоров за период
	if (hasBody && requestBody.Mode == "partner_all") || (!hasBody && c.Query("mode") == "partner_all") {
		if err := triggerPartnerSnapshotsForAllContracts(c, requestBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "Запрос на создание партнёрских снимков принят. Проверяйте историю.",
		})
		return
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
				"status": "success",
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
