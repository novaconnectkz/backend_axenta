package services

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"backend_axenta/models"
)

// InstallationService представляет сервис для работы с монтажами
type InstallationService struct {
	DB                  *gorm.DB
	NotificationService *NotificationService
}

// NewInstallationService создает новый экземпляр InstallationService
func NewInstallationService(db *gorm.DB, notificationService *NotificationService) *InstallationService {
	return &InstallationService{
		DB:                  db,
		NotificationService: notificationService,
	}
}

// ScheduleInstallation планирует новый монтаж с проверкой доступности
func (s *InstallationService) ScheduleInstallation(installation *models.Installation) error {
	// Проверяем доступность монтажника
	var installer models.Installer
	if err := s.DB.First(&installer, installation.InstallerID).Error; err != nil {
		return errors.New("монтажник не найден")
	}

	if !installer.IsAvailableOnDate(installation.ScheduledAt) {
		return errors.New("монтажник недоступен на указанную дату")
	}

	// Проверяем конфликты в расписании
	conflicts, err := s.CheckScheduleConflicts(installation.InstallerID, installation.ScheduledAt, installation.EstimatedDuration, 0)
	if err != nil {
		return fmt.Errorf("ошибка при проверке расписания: %v", err)
	}

	if len(conflicts) > 0 {
		return errors.New("у монтажника уже есть работы в это время")
	}

	// Проверяем максимальное количество монтажей в день
	dayStart := time.Date(installation.ScheduledAt.Year(), installation.ScheduledAt.Month(), installation.ScheduledAt.Day(), 0, 0, 0, 0, installation.ScheduledAt.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	var dailyCount int64
	s.DB.Model(&models.Installation{}).
		Where("installer_id = ? AND scheduled_at BETWEEN ? AND ? AND status IN ('planned', 'in_progress')",
			installation.InstallerID, dayStart, dayEnd).
		Count(&dailyCount)

	if int(dailyCount) >= installer.MaxDailyInstallations {
		return fmt.Errorf("превышено максимальное количество монтажей в день (%d)", installer.MaxDailyInstallations)
	}

	// Создаем монтаж
	if err := s.DB.Create(installation).Error; err != nil {
		return fmt.Errorf("ошибка при создании монтажа: %v", err)
	}

	// Отправляем уведомления
	if s.NotificationService != nil {
		go s.sendInstallationNotifications(installation, "created")
	}

	return nil
}

// CheckScheduleConflicts проверяет конфликты в расписании монтажника
func (s *InstallationService) CheckScheduleConflicts(installerID uint, scheduledAt time.Time, duration int, excludeInstallationID uint) ([]models.Installation, error) {
	startTime := scheduledAt
	endTime := startTime.Add(time.Duration(duration) * time.Minute)

	// Добавляем буферное время (30 минут до и после)
	bufferStart := startTime.Add(-30 * time.Minute)
	bufferEnd := endTime.Add(30 * time.Minute)

	var conflicts []models.Installation
	query := s.DB.Where("installer_id = ? AND status IN ('planned', 'in_progress')", installerID)

	if excludeInstallationID > 0 {
		query = query.Where("id != ?", excludeInstallationID)
	}

	// Проверяем пересечения по времени
	query = query.Where("(scheduled_at BETWEEN ? AND ?) OR (scheduled_at + INTERVAL '1 minute' * estimated_duration BETWEEN ? AND ?)",
		bufferStart, bufferEnd, bufferStart, bufferEnd)

	err := query.Preload("Object").Find(&conflicts).Error
	return conflicts, err
}

// RescheduleInstallation переносит монтаж на другое время
func (s *InstallationService) RescheduleInstallation(installationID uint, newScheduledAt time.Time, newInstallerID *uint) error {
	var installation models.Installation
	if err := s.DB.First(&installation, installationID).Error; err != nil {
		return errors.New("монтаж не найден")
	}

	if installation.Status == "completed" || installation.Status == "cancelled" {
		return errors.New("нельзя перенести завершенный или отмененный монтаж")
	}

	// Определяем ID монтажника
	installerID := installation.InstallerID
	if newInstallerID != nil {
		installerID = *newInstallerID
	}

	// Проверяем доступность монтажника
	var installer models.Installer
	if err := s.DB.First(&installer, installerID).Error; err != nil {
		return errors.New("монтажник не найден")
	}

	if !installer.IsAvailableOnDate(newScheduledAt) {
		return errors.New("монтажник недоступен на указанную дату")
	}

	// Проверяем конфликты (исключая текущий монтаж)
	conflicts, err := s.CheckScheduleConflicts(installerID, newScheduledAt, installation.EstimatedDuration, installation.ID)
	if err != nil {
		return fmt.Errorf("ошибка при проверке расписания: %v", err)
	}

	if len(conflicts) > 0 {
		return errors.New("у монтажника уже есть работы в это время")
	}

	// Обновляем монтаж
	oldScheduledAt := installation.ScheduledAt
	installation.ScheduledAt = newScheduledAt
	installation.Status = "planned" // Сбрасываем статус, если он был "postponed"

	if newInstallerID != nil {
		installation.InstallerID = *newInstallerID
	}

	if err := s.DB.Save(&installation).Error; err != nil {
		return fmt.Errorf("ошибка при переносе монтажа: %v", err)
	}

	// Отправляем уведомления о переносе
	if s.NotificationService != nil {
		go s.sendRescheduleNotifications(&installation, oldScheduledAt)
	}

	return nil
}

// GetInstallerWorkload возвращает загруженность монтажника на указанный период
func (s *InstallationService) GetInstallerWorkload(installerID uint, dateFrom, dateTo time.Time) (*InstallerWorkload, error) {
	var workload InstallerWorkload
	workload.InstallerID = installerID
	workload.DateFrom = dateFrom
	workload.DateTo = dateTo

	// Получаем все монтажи в указанном периоде
	var installations []models.Installation
	err := s.DB.Where("installer_id = ? AND scheduled_at BETWEEN ? AND ?",
		installerID, dateFrom, dateTo.Add(24*time.Hour)).
		Preload("Object").
		Order("scheduled_at ASC").
		Find(&installations).Error

	if err != nil {
		return nil, err
	}

	workload.Installations = installations
	workload.TotalInstallations = len(installations)

	// Подсчитываем статистику
	for _, installation := range installations {
		switch installation.Status {
		case "planned":
			workload.PlannedCount++
		case "in_progress":
			workload.InProgressCount++
		case "completed":
			workload.CompletedCount++
		case "cancelled":
			workload.CancelledCount++
		}

		workload.TotalEstimatedTime += installation.EstimatedDuration
		if installation.ActualDuration > 0 {
			workload.TotalActualTime += installation.ActualDuration
		}
	}

	// Рассчитываем загруженность по дням
	workload.DailyWorkload = s.calculateDailyWorkload(installations, dateFrom, dateTo)

	return &workload, nil
}

// GetAvailableInstallers возвращает доступных монтажников для указанных параметров
func (s *InstallationService) GetAvailableInstallers(date time.Time, locationID *uint, specialization string, duration int) ([]models.Installer, error) {
	query := s.DB.Where("is_active = true AND status = 'available'")

	// Фильтр по локации
	if locationID != nil {
		query = query.Where("? = ANY(location_ids)", *locationID)
	}

	// Фильтр по специализации
	if specialization != "" {
		query = query.Where("? = ANY(specialization)", specialization)
	}

	var installers []models.Installer
	if err := query.Find(&installers).Error; err != nil {
		return nil, err
	}

	// Фильтруем по доступности на дату и проверяем конфликты
	var availableInstallers []models.Installer
	for _, installer := range installers {
		if !installer.IsAvailableOnDate(date) {
			continue
		}

		// Проверяем конфликты в расписании
		conflicts, err := s.CheckScheduleConflicts(installer.ID, date, duration, 0)
		if err != nil {
			continue
		}

		if len(conflicts) == 0 {
			// Проверяем максимальное количество монтажей в день
			dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
			dayEnd := dayStart.Add(24 * time.Hour)

			var dailyCount int64
			s.DB.Model(&models.Installation{}).
				Where("installer_id = ? AND scheduled_at BETWEEN ? AND ? AND status IN ('planned', 'in_progress')",
					installer.ID, dayStart, dayEnd).
				Count(&dailyCount)

			if int(dailyCount) < installer.MaxDailyInstallations {
				availableInstallers = append(availableInstallers, installer)
			}
		}
	}

	return availableInstallers, nil
}

// SendReminders отправляет напоминания о предстоящих монтажах
func (s *InstallationService) SendReminders() error {
	// Получаем монтажи на завтра, по которым еще не отправлялись напоминания
	tomorrow := time.Now().Add(24 * time.Hour)
	tomorrowStart := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, tomorrow.Location())
	tomorrowEnd := tomorrowStart.Add(24 * time.Hour)

	var installations []models.Installation
	err := s.DB.Where("scheduled_at BETWEEN ? AND ? AND status = 'planned' AND reminder_sent = false",
		tomorrowStart, tomorrowEnd).
		Preload("Object").
		Preload("Installer").
		Find(&installations).Error

	if err != nil {
		return err
	}

	for _, installation := range installations {
		if s.NotificationService != nil {
			err := s.NotificationService.SendInstallationReminder(&installation)
			if err == nil {
				// Отмечаем, что напоминание отправлено
				now := time.Now()
				installation.ReminderSent = true
				installation.ReminderSentAt = &now
				s.DB.Save(&installation)
			}
		}
	}

	return nil
}

// GetOverdueInstallations возвращает просроченные монтажи
func (s *InstallationService) GetOverdueInstallations() ([]models.Installation, error) {
	var installations []models.Installation
	err := s.DB.Where("scheduled_at < ? AND status IN ('planned', 'in_progress')", time.Now()).
		Preload("Object").
		Preload("Installer").
		Order("scheduled_at ASC").
		Find(&installations).Error

	return installations, err
}

// Вспомогательные структуры и методы

type InstallerWorkload struct {
	InstallerID        uint                  `json:"installer_id"`
	DateFrom           time.Time             `json:"date_from"`
	DateTo             time.Time             `json:"date_to"`
	TotalInstallations int                   `json:"total_installations"`
	PlannedCount       int                   `json:"planned_count"`
	InProgressCount    int                   `json:"in_progress_count"`
	CompletedCount     int                   `json:"completed_count"`
	CancelledCount     int                   `json:"cancelled_count"`
	TotalEstimatedTime int                   `json:"total_estimated_time"`
	TotalActualTime    int                   `json:"total_actual_time"`
	DailyWorkload      []DailyWorkload       `json:"daily_workload"`
	Installations      []models.Installation `json:"installations"`
}

type DailyWorkload struct {
	Date              time.Time `json:"date"`
	InstallationCount int       `json:"installation_count"`
	EstimatedTime     int       `json:"estimated_time"`
	ActualTime        int       `json:"actual_time"`
	IsOverloaded      bool      `json:"is_overloaded"`
}

func (s *InstallationService) calculateDailyWorkload(installations []models.Installation, dateFrom, dateTo time.Time) []DailyWorkload {
	dailyMap := make(map[string]*DailyWorkload)

	// Инициализируем все дни в периоде
	for d := dateFrom; d.Before(dateTo) || d.Equal(dateTo); d = d.Add(24 * time.Hour) {
		dateKey := d.Format("2006-01-02")
		dailyMap[dateKey] = &DailyWorkload{
			Date:              d,
			InstallationCount: 0,
			EstimatedTime:     0,
			ActualTime:        0,
			IsOverloaded:      false,
		}
	}

	// Заполняем данными из монтажей
	for _, installation := range installations {
		dateKey := installation.ScheduledAt.Format("2006-01-02")
		if daily, exists := dailyMap[dateKey]; exists {
			daily.InstallationCount++
			daily.EstimatedTime += installation.EstimatedDuration
			if installation.ActualDuration > 0 {
				daily.ActualTime += installation.ActualDuration
			}

			// Считаем день перегруженным, если больше 8 часов работы
			if daily.EstimatedTime > 480 { // 8 часов = 480 минут
				daily.IsOverloaded = true
			}
		}
	}

	// Преобразуем в слайс
	var result []DailyWorkload
	for d := dateFrom; d.Before(dateTo) || d.Equal(dateTo); d = d.Add(24 * time.Hour) {
		dateKey := d.Format("2006-01-02")
		if daily, exists := dailyMap[dateKey]; exists {
			result = append(result, *daily)
		}
	}

	return result
}

func (s *InstallationService) sendInstallationNotifications(installation *models.Installation, action string) {
	if s.NotificationService == nil {
		return
	}

	switch action {
	case "created":
		s.NotificationService.SendInstallationCreated(installation)
	case "updated":
		s.NotificationService.SendInstallationUpdated(installation)
	case "completed":
		s.NotificationService.SendInstallationCompleted(installation)
	case "cancelled":
		s.NotificationService.SendInstallationCancelled(installation)
	}
}

func (s *InstallationService) sendRescheduleNotifications(installation *models.Installation, oldScheduledAt time.Time) {
	if s.NotificationService == nil {
		return
	}

	s.NotificationService.SendInstallationRescheduled(installation, oldScheduledAt)
}
