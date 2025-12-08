package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"errors"
	"fmt"
	"log"
	"runtime"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BillingSnapshotService отвечает за расчет ежедневных снимков количества объектов
type BillingSnapshotService struct {
	publicDB       *gorm.DB
	countCreatedFn func(time.Time) (int, error)
	nowFn          func() time.Time
}

// BillingRebuildResult содержит краткий результат пересчета истории
type BillingRebuildResult struct {
	DaysProcessed int
	FinalTotal    int
	LastDate      time.Time
}

// BillingDailyRunResult содержит результат ежедневного запуска
type BillingDailyRunResult struct {
	ProcessedDates []time.Time
	FinalTotal     int
}

// NewBillingSnapshotService создает сервис с подключением к public схеме и стандартными зависимостями
func NewBillingSnapshotService() *BillingSnapshotService {
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на схему public для billing snapshots: %v", err)
	}

	service := &BillingSnapshotService{
		publicDB: publicDB,
		nowFn:    time.Now,
	}
	service.countCreatedFn = service.countCreatedOnDate
	return service
}

// NewBillingSnapshotServiceWithDeps позволяет переопределить зависимости (удобно для тестов)
func NewBillingSnapshotServiceWithDeps(publicDB *gorm.DB, countFn func(time.Time) (int, error), nowFn func() time.Time) *BillingSnapshotService {
	if nowFn == nil {
		nowFn = time.Now
	}
	s := &BillingSnapshotService{
		publicDB:       publicDB,
		countCreatedFn: countFn,
		nowFn:          nowFn,
	}
	if s.countCreatedFn == nil {
		s.countCreatedFn = s.countCreatedOnDate
	}
	if s.publicDB == nil {
		s.publicDB = database.DB
	}
	return s
}

// RebuildBillingSnapshots выполняет одноразовый исторический пересчет за указанный период (включительно)
func (s *BillingSnapshotService) RebuildBillingSnapshots(startDate, endDate time.Time) (*BillingRebuildResult, error) {
	start := normalizeDateUTC(startDate)
	end := normalizeDateUTC(endDate)

	if end.Before(start) {
		return nil, fmt.Errorf("дата окончания раньше даты начала")
	}

	job := s.newJob(models.SnapshotJobTypeBillingStart, start, end, "system")
	if err := s.publicDB.Create(job).Error; err != nil {
		return nil, fmt.Errorf("не удалось создать запись задачи пересчета: %w", err)
	}

	var (
		currentTotal int
		lastDate     time.Time
		processed    int
		successful   int
	)

	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		processed++
		createdToday, err := s.countCreatedFn(date)
		if err != nil {
			job.AddError(models.JobError{
				Date:      date.Format("2006-01-02"),
				Message:   err.Error(),
				ErrorType: "db_error",
			})
			continue
		}

		currentTotal += createdToday
		lastDate = date

		snapshot := &models.BillingDailySnapshot{
			SnapshotDate:           date,
			CreatedToday:           createdToday,
			TotalObjectsCumulative: currentTotal,
		}

		if err := s.upsertSnapshot(snapshot); err != nil {
			job.AddError(models.JobError{
				Date:      date.Format("2006-01-02"),
				Message:   err.Error(),
				ErrorType: "db_error",
			})
			continue
		}

		successful++
	}

	job.TotalDaysProcessed = processed
	job.SuccessCount = successful
	job.ErrorCount = len(job.Details.Errors)
	job.TotalObjects = currentTotal

	status := models.SnapshotJobStatusCompleted
	errorMessage := ""
	if job.ErrorCount > 0 {
		status = models.SnapshotJobStatusPartial
		errorMessage = "Часть дней обработана с ошибками"
	}
	job.FinishJob(status, errorMessage)

	if err := s.publicDB.Save(job).Error; err != nil {
		return nil, fmt.Errorf("не удалось обновить запись задачи: %w", err)
	}

	return &BillingRebuildResult{
		DaysProcessed: successful,
		FinalTotal:    currentTotal,
		LastDate:      lastDate,
	}, nil
}

// RunDailySnapshot создает недостающие ежедневные снимки до текущей даты
func (s *BillingSnapshotService) RunDailySnapshot() (*BillingDailyRunResult, error) {
	var lastSnapshot models.BillingDailySnapshot
	err := s.publicDB.Order("snapshot_date DESC").First(&lastSnapshot).Error

	var startDate time.Time
	var currentTotal int

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Если нет данных, начинаем с начальной даты биллинга
			log.Printf("ℹ️ Billing snapshots: нет данных, начинаем с начальной даты биллинга (2024-03-14)")
			startDate = time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)
			currentTotal = 0
		} else {
			return nil, fmt.Errorf("не удалось получить последний снимок: %w", err)
		}
	} else {
		// Есть данные, продолжаем с следующей даты после последнего снимка
		startDate = normalizeDateUTC(lastSnapshot.SnapshotDate.AddDate(0, 0, 1))
		currentTotal = lastSnapshot.TotalObjectsCumulative
	}

	today := normalizeDateUTC(s.nowFn().UTC())

	// Если все актуально — просто возвращаем результат
	if startDate.After(today) {
		return &BillingDailyRunResult{
			ProcessedDates: []time.Time{},
			FinalTotal:     currentTotal,
		}, nil
	}

	processedDates := make([]time.Time, 0)

	for date := startDate; !date.After(today); date = date.AddDate(0, 0, 1) {
		createdToday, err := s.countCreatedFn(date)
		if err != nil {
			return nil, fmt.Errorf("не удалось подсчитать объекты за %s: %w", date.Format("2006-01-02"), err)
		}

		currentTotal += createdToday

		snapshot := &models.BillingDailySnapshot{
			SnapshotDate:           date,
			CreatedToday:           createdToday,
			TotalObjectsCumulative: currentTotal,
		}

		if err := s.upsertSnapshot(snapshot); err != nil {
			return nil, fmt.Errorf("не удалось сохранить снимок за %s: %w", date.Format("2006-01-02"), err)
		}

		if err := s.upsertDailyJob(date, createdToday, currentTotal); err != nil {
			log.Printf("⚠️ Ошибка записи в snapshot_jobs за %s: %v", date.Format("2006-01-02"), err)
		}

		processedDates = append(processedDates, date)
	}

	return &BillingDailyRunResult{
		ProcessedDates: processedDates,
		FinalTotal:     currentTotal,
	}, nil
}

func (s *BillingSnapshotService) countCreatedOnDate(date time.Time) (int, error) {
	start := normalizeDateUTC(date)
	end := start.AddDate(0, 0, 1)

	var companies []models.Company
	if err := s.publicDB.Table("public.companies").Where("is_active = ?", true).Find(&companies).Error; err != nil {
		return 0, fmt.Errorf("не удалось получить компании: %w", err)
	}

	var total int64

	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			log.Printf("⚠️ Не удалось получить tenant DB для компании %d", company.ID)
			continue
		}

		// Проверяем наличие таблицы, чтобы избежать ошибок на окружениях без миграций
		if !tenantDB.Migrator().HasTable(&models.AxentaObjectSnapshot{}) {
			continue
		}

		var count int64
		query := tenantDB.Unscoped().
			Model(&models.AxentaObjectSnapshot{}).
			Where("(axenta_created_at IS NOT NULL AND axenta_created_at >= ? AND axenta_created_at < ?) OR (axenta_created_at IS NULL AND created_at >= ? AND created_at < ?)",
				start, end, start, end)

		if err := query.Count(&count).Error; err != nil {
			return 0, fmt.Errorf("ошибка подсчета объектов для компании %d: %w", company.ID, err)
		}

		total += count
	}

	return int(total), nil
}

func (s *BillingSnapshotService) upsertSnapshot(snapshot *models.BillingDailySnapshot) error {
	snapshot.SnapshotDate = normalizeDateUTC(snapshot.SnapshotDate)
	snapshot.UpdatedAt = s.nowFn()
	return s.publicDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "snapshot_date"}},
		DoUpdates: clause.AssignmentColumns([]string{"created_today", "total_objects_cumulative", "updated_at"}),
	}).Create(snapshot).Error
}

func (s *BillingSnapshotService) upsertDailyJob(date time.Time, createdToday, total int) error {
	date = normalizeDateUTC(date)

	var existing models.SnapshotJob
	err := s.publicDB.
		Where("job_type = ? AND date_from = ?", models.SnapshotJobTypeDailyAuto, date).
		First(&existing).Error

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("не удалось получить запись snapshot_job за %s: %w", date.Format("2006-01-02"), err)
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		now := s.nowFn()
		job := models.SnapshotJob{
			JobType:            models.SnapshotJobTypeDailyAuto,
			StartedAt:          now,
			DateFrom:           date,
			DateTo:             date,
			Status:             models.SnapshotJobStatusCompleted,
			TotalObjects:       total,
			ActiveObjects:      createdToday,
			SuccessCount:       1,
			TotalDaysProcessed: 1,
			TriggeredBy:        "system",
		}
		job.FinishJob(models.SnapshotJobStatusCompleted, "")
		return s.publicDB.Create(&job).Error
	}

	existing.StartedAt = s.nowFn()
	existing.DateFrom = date
	existing.DateTo = date
	existing.TotalObjects = total
	existing.ActiveObjects = createdToday
	existing.SuccessCount = 1
	existing.TotalDaysProcessed = 1
	existing.ErrorCount = 0
	existing.SkippedCount = 0
	existing.ErrorMessage = ""
	existing.FinishJob(models.SnapshotJobStatusCompleted, "")

	return s.publicDB.Save(&existing).Error
}

func (s *BillingSnapshotService) newJob(jobType models.SnapshotJobType, dateFrom, dateTo time.Time, triggeredBy string) *models.SnapshotJob {
	now := s.nowFn()
	return &models.SnapshotJob{
		JobType:            jobType,
		StartedAt:          now,
		DateFrom:           dateFrom,
		DateTo:             dateTo,
		Status:             models.SnapshotJobStatusRunning,
		TotalDaysProcessed: 0,
		SuccessCount:       0,
		TriggeredBy:        triggeredBy,
		ServerInfo: models.ServerInfo{
			GoVersion: runtime.Version(),
		},
	}
}

func normalizeDateUTC(date time.Time) time.Time {
	date = date.In(time.UTC)
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
}
