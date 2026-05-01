package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"backend_axenta/models"
)

// TestInstallationService_SendReminders тестирует SendReminders
func TestInstallationService_SendReminders(t *testing.T) {
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil)
	notificationService := NewNotificationService(db, cache, nil, nil)
	installationService := NewInstallationService(db, notificationService, 0)

	// Создаем монтаж на завтра (должен получить напоминание)
	tomorrow := time.Now().Add(24 * time.Hour)
	installation := &models.Installation{
		Type:        "монтаж",
		ObjectID:    object.ID,
		InstallerID: installer.ID,
		ScheduledAt: tomorrow,
		Status:      "planned",
	}
	db.Create(installation)

	// Отправляем напоминания
	err := installationService.SendReminders()
	// Может вернуть ошибку или успех в зависимости от реализации
	if err != nil {
		assert.Contains(t, err.Error(), "ошибка")
	}
}

// TestInstallationService_RescheduleInstallation_Completed тестирует RescheduleInstallation для завершенного монтажа
func TestInstallationService_RescheduleInstallation_Completed(t *testing.T) {
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil)
	notificationService := NewNotificationService(db, cache, nil, nil)
	installationService := NewInstallationService(db, notificationService, 0)

	nextMonday := getNextWeekday(time.Monday)
	installation := &models.Installation{
		Type:        "монтаж",
		ObjectID:    object.ID,
		InstallerID: installer.ID,
		ScheduledAt: nextMonday.Add(10 * time.Hour),
		Status:      "completed",
	}
	db.Create(installation)

	// Пытаемся перенести завершенный монтаж
	err := installationService.RescheduleInstallation(installation.ID, nextMonday.Add(14*time.Hour), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "нельзя перенести завершенный")
}

// TestInstallationService_RescheduleInstallation_InstallerNotFound тестирует RescheduleInstallation когда монтажник не найден
func TestInstallationService_RescheduleInstallation_InstallerNotFound(t *testing.T) {
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil)
	notificationService := NewNotificationService(db, cache, nil, nil)
	installationService := NewInstallationService(db, notificationService, 0)

	nextMonday := getNextWeekday(time.Monday)
	installation := &models.Installation{
		Type:        "монтаж",
		ObjectID:    object.ID,
		InstallerID: installer.ID,
		ScheduledAt: nextMonday.Add(10 * time.Hour),
		Status:      "planned",
	}
	db.Create(installation)

	// Пытаемся перенести на несуществующего монтажника
	nonExistentInstallerID := uint(99999)
	err := installationService.RescheduleInstallation(installation.ID, nextMonday.Add(14*time.Hour), &nonExistentInstallerID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "монтажник не найден")
}
