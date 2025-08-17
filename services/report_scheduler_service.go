package services

import (
	"backend_axenta/models"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// ReportSchedulerService управляет автоматическими отчетами по расписанию
type ReportSchedulerService struct {
	db            *gorm.DB
	reportService *ReportService
	cron          *cron.Cron
	emailService  *NotificationService // Для отправки отчетов по email
}

// NewReportSchedulerService создает новый экземпляр ReportSchedulerService
func NewReportSchedulerService(db *gorm.DB, reportService *ReportService, emailService *NotificationService) *ReportSchedulerService {
	c := cron.New(cron.WithSeconds())
	return &ReportSchedulerService{
		db:            db,
		reportService: reportService,
		cron:          c,
		emailService:  emailService,
	}
}

// Start запускает планировщик отчетов
func (rss *ReportSchedulerService) Start() error {
	// Загружаем все активные расписания
	if err := rss.loadSchedules(); err != nil {
		return fmt.Errorf("failed to load schedules: %w", err)
	}

	rss.cron.Start()
	log.Println("Report scheduler started")
	return nil
}

// Stop останавливает планировщик отчетов
func (rss *ReportSchedulerService) Stop() {
	rss.cron.Stop()
	log.Println("Report scheduler stopped")
}

// loadSchedules загружает все активные расписания из БД
func (rss *ReportSchedulerService) loadSchedules() error {
	var schedules []models.ReportSchedule
	if err := rss.db.Where("is_active = ?", true).Preload("Template").Find(&schedules).Error; err != nil {
		return err
	}

	for _, schedule := range schedules {
		if err := rss.addScheduleJob(schedule); err != nil {
			log.Printf("Failed to add schedule job for %d: %v", schedule.ID, err)
		}
	}

	log.Printf("Loaded %d active report schedules", len(schedules))
	return nil
}

// addScheduleJob добавляет задачу в планировщик
func (rss *ReportSchedulerService) addScheduleJob(schedule models.ReportSchedule) error {
	cronExpr := rss.buildCronExpression(schedule)
	if cronExpr == "" {
		return fmt.Errorf("failed to build cron expression for schedule %d", schedule.ID)
	}

	_, err := rss.cron.AddFunc(cronExpr, func() {
		rss.executeScheduledReport(schedule.ID)
	})

	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}

	// Обновляем время следующего запуска
	nextRun := rss.calculateNextRun(cronExpr)
	if nextRun != nil {
		schedule.NextRunAt = nextRun
		rss.db.Save(&schedule)
	}

	log.Printf("Added scheduled report job: %s (ID: %d, Cron: %s)", schedule.Name, schedule.ID, cronExpr)
	return nil
}

// buildCronExpression строит cron выражение из параметров расписания
func (rss *ReportSchedulerService) buildCronExpression(schedule models.ReportSchedule) string {
	// Если есть готовое cron выражение, используем его
	if schedule.CronExpression != "" {
		return schedule.CronExpression
	}

	// Парсим время дня
	timeOfDay := schedule.TimeOfDay
	if timeOfDay == "" {
		timeOfDay = "09:00" // По умолчанию 9 утра
	}

	timeParts := strings.Split(timeOfDay, ":")
	if len(timeParts) != 2 {
		return ""
	}

	hour := timeParts[0]
	minute := timeParts[1]

	switch schedule.Type {
	case models.ScheduleTypeDaily:
		// Каждый день в указанное время
		return fmt.Sprintf("0 %s %s * * *", minute, hour)
	case models.ScheduleTypeWeekly:
		// Еженедельно в указанный день недели
		dayOfWeek := schedule.DayOfWeek
		if dayOfWeek < 0 || dayOfWeek > 6 {
			dayOfWeek = 1 // По умолчанию понедельник
		}
		return fmt.Sprintf("0 %s %s * * %d", minute, hour, dayOfWeek)
	case models.ScheduleTypeMonthly:
		// Ежемесячно в указанный день месяца
		dayOfMonth := schedule.DayOfMonth
		if dayOfMonth < 1 || dayOfMonth > 31 {
			dayOfMonth = 1 // По умолчанию 1 число
		}
		return fmt.Sprintf("0 %s %s %d * *", minute, hour, dayOfMonth)
	case models.ScheduleTypeYearly:
		// Ежегодно 1 января в указанное время
		return fmt.Sprintf("0 %s %s 1 1 *", minute, hour)
	default:
		return ""
	}
}

// calculateNextRun вычисляет время следующего запуска
func (rss *ReportSchedulerService) calculateNextRun(cronExpr string) *time.Time {
	schedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return nil
	}

	nextRun := schedule.Next(time.Now())
	return &nextRun
}

// executeScheduledReport выполняет запланированный отчет
func (rss *ReportSchedulerService) executeScheduledReport(scheduleID uint) {
	log.Printf("Executing scheduled report: %d", scheduleID)

	// Создаем запись выполнения
	execution := models.ReportExecution{
		ScheduleID: scheduleID,
		Status:     models.ReportStatusPending,
		StartedAt:  &[]time.Time{time.Now()}[0],
		CompanyID:  0, // Будет установлен при загрузке расписания
	}

	// Загружаем расписание с шаблоном
	var schedule models.ReportSchedule
	if err := rss.db.Preload("Template").First(&schedule, scheduleID).Error; err != nil {
		rss.updateExecutionError(&execution, fmt.Sprintf("Failed to load schedule: %v", err))
		return
	}

	execution.CompanyID = schedule.CompanyID
	if err := rss.db.Create(&execution).Error; err != nil {
		log.Printf("Failed to create execution record: %v", err)
		return
	}

	// Обновляем статус выполнения
	execution.Status = models.ReportStatusProcessing
	rss.db.Save(&execution)

	// Парсим параметры из расписания
	var params ReportParams
	if err := json.Unmarshal([]byte(schedule.Parameters), &params); err != nil {
		rss.updateExecutionError(&execution, fmt.Sprintf("Failed to parse parameters: %v", err))
		return
	}

	// Устанавливаем дополнительные параметры
	params.CompanyID = schedule.CompanyID
	params.Format = schedule.Format
	params.Type = schedule.Template.Type

	// Если даты не указаны, используем период с прошлого запуска до текущего времени
	if params.DateFrom == nil && params.DateTo == nil {
		now := time.Now()
		params.DateTo = &now

		if schedule.LastRunAt != nil {
			params.DateFrom = schedule.LastRunAt
		} else {
			// Если это первый запуск, используем период в зависимости от типа расписания
			switch schedule.Type {
			case models.ScheduleTypeDaily:
				yesterday := now.AddDate(0, 0, -1)
				params.DateFrom = &yesterday
			case models.ScheduleTypeWeekly:
				weekAgo := now.AddDate(0, 0, -7)
				params.DateFrom = &weekAgo
			case models.ScheduleTypeMonthly:
				monthAgo := now.AddDate(0, -1, 0)
				params.DateFrom = &monthAgo
			case models.ScheduleTypeYearly:
				yearAgo := now.AddDate(-1, 0, 0)
				params.DateFrom = &yearAgo
			}
		}
	}

	// Создаем отчет
	report := models.Report{
		Name:        fmt.Sprintf("%s (автоматический)", schedule.Name),
		Description: fmt.Sprintf("Автоматически сгенерированный отчет по расписанию '%s'", schedule.Name),
		Type:        params.Type,
		DateFrom:    params.DateFrom,
		DateTo:      params.DateTo,
		Status:      models.ReportStatusPending,
		Format:      params.Format,
		CreatedByID: schedule.CreatedByID,
		CompanyID:   schedule.CompanyID,
	}

	// Сохраняем параметры в JSON
	paramsJSON, _ := json.Marshal(params)
	report.Parameters = string(paramsJSON)

	if err := rss.db.Create(&report).Error; err != nil {
		rss.updateExecutionError(&execution, fmt.Sprintf("Failed to create report: %v", err))
		return
	}

	// Связываем выполнение с отчетом
	execution.ReportID = &report.ID
	rss.db.Save(&execution)

	// Генерируем отчет
	if err := rss.reportService.GenerateReport(params, &report); err != nil {
		rss.updateExecutionError(&execution, fmt.Sprintf("Failed to generate report: %v", err))
		return
	}

	// Отправляем отчет получателям
	if schedule.Recipients != "" {
		rss.sendReportToRecipients(&execution, &schedule, &report)
	}

	// Обновляем статистику расписания
	now := time.Now()
	schedule.LastRunAt = &now
	schedule.RunCount++
	schedule.LastReportID = &report.ID
	schedule.NextRunAt = rss.calculateNextRun(rss.buildCronExpression(schedule))
	rss.db.Save(&schedule)

	// Завершаем выполнение
	completedAt := time.Now()
	execution.Status = models.ReportStatusCompleted
	execution.CompletedAt = &completedAt
	execution.Duration = int(completedAt.Sub(*execution.StartedAt).Seconds())
	rss.db.Save(&execution)

	log.Printf("Successfully completed scheduled report: %d", scheduleID)
}

// sendReportToRecipients отправляет отчет получателям по email
func (rss *ReportSchedulerService) sendReportToRecipients(execution *models.ReportExecution, schedule *models.ReportSchedule, report *models.Report) {
	var recipients []string
	if err := json.Unmarshal([]byte(schedule.Recipients), &recipients); err != nil {
		log.Printf("Failed to parse recipients: %v", err)
		return
	}

	emailsSent := 0
	emailsFailures := 0
	var deliveryLog []string

	for _, email := range recipients {
		if rss.emailService != nil {
			// Здесь должна быть логика отправки email с вложением
			// Пока просто логируем
			log.Printf("Sending report %d to %s", report.ID, email)
			deliveryLog = append(deliveryLog, fmt.Sprintf("Sent to %s at %s", email, time.Now().Format("15:04:05")))
			emailsSent++
		} else {
			deliveryLog = append(deliveryLog, fmt.Sprintf("Failed to send to %s: email service not available", email))
			emailsFailures++
		}
	}

	// Обновляем статистику доставки
	execution.EmailsSent = emailsSent
	execution.EmailsFailures = emailsFailures
	execution.DeliveryLog = strings.Join(deliveryLog, "\n")
	rss.db.Save(execution)
}

// updateExecutionError обновляет выполнение с информацией об ошибке
func (rss *ReportSchedulerService) updateExecutionError(execution *models.ReportExecution, errorMsg string) {
	log.Printf("Scheduled report execution failed: %s", errorMsg)

	now := time.Now()
	execution.Status = models.ReportStatusFailed
	execution.ErrorMsg = errorMsg
	execution.CompletedAt = &now
	if execution.StartedAt != nil {
		execution.Duration = int(now.Sub(*execution.StartedAt).Seconds())
	}
	rss.db.Save(execution)

	// Увеличиваем счетчик ошибок в расписании
	var schedule models.ReportSchedule
	if err := rss.db.First(&schedule, execution.ScheduleID).Error; err == nil {
		schedule.FailCount++
		rss.db.Save(&schedule)
	}
}

// AddSchedule добавляет новое расписание в планировщик
func (rss *ReportSchedulerService) AddSchedule(schedule models.ReportSchedule) error {
	if err := rss.addScheduleJob(schedule); err != nil {
		return fmt.Errorf("failed to add schedule job: %w", err)
	}
	return nil
}

// RemoveSchedule удаляет расписание из планировщика
func (rss *ReportSchedulerService) RemoveSchedule(scheduleID uint) error {
	// В данной реализации cron/v3 не предоставляет простого способа удаления задач по ID
	// Для production использования рекомендуется перезапуск планировщика или использование
	// более продвинутого планировщика задач
	log.Printf("Schedule %d should be removed (requires scheduler restart)", scheduleID)
	return nil
}

// UpdateSchedule обновляет расписание в планировщике
func (rss *ReportSchedulerService) UpdateSchedule(schedule models.ReportSchedule) error {
	// Аналогично RemoveSchedule, требует перезапуска планировщика
	log.Printf("Schedule %d should be updated (requires scheduler restart)", schedule.ID)
	return nil
}

// GetScheduleStatus возвращает статус расписания
func (rss *ReportSchedulerService) GetScheduleStatus(scheduleID uint) (map[string]interface{}, error) {
	var schedule models.ReportSchedule
	if err := rss.db.Preload("LastReport").First(&schedule, scheduleID).Error; err != nil {
		return nil, err
	}

	// Получаем последние выполнения
	var executions []models.ReportExecution
	rss.db.Where("schedule_id = ?", scheduleID).Order("created_at DESC").Limit(5).Find(&executions)

	status := map[string]interface{}{
		"schedule":          schedule,
		"recent_executions": executions,
		"is_running":        rss.cron.Entry(cron.EntryID(scheduleID)).Valid(),
	}

	return status, nil
}

// formatDate форматирует дату для отображения
func formatDate(date *time.Time) string {
	if date == nil {
		return "не указана"
	}
	return date.Format("02.01.2006")
}
