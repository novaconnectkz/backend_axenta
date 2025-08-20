package services

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"backend_axenta/models"
)

// Настройка тестовой базы данных для тестов сервиса монтажей
func setupServiceTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	// Автоматическая миграция
	db.AutoMigrate(
		&models.Company{},
		&models.User{},
		&models.Object{},
		&models.Location{},
		&models.Installer{},
		&models.Installation{},
		&models.Equipment{},
		&models.Contract{},
		&models.BillingPlan{},
	)

	return db
}

// Создание тестовых данных для сервиса монтажей
func createServiceTestData(db *gorm.DB) (models.Object, models.Installer, models.Location) {
	// Создаем компанию
	company := models.Company{
		Name:           "Test Company",
		DatabaseSchema: "test_schema",
		AxetnaLogin:    "test_login",
		AxetnaPassword: "test_password",
	}
	db.Create(&company)

	// Создаем локацию
	location := models.Location{
		City:   "Москва",
		Region: "Московская область",
	}
	db.Create(&location)

	// Создаем тарифный план
	billingPlan := models.BillingPlan{
		Name:     "Тестовый план",
		Price:    decimal.NewFromInt(1000),
		Currency: "RUB",
	}
	db.Create(&billingPlan)

	// Создаем договор
	contract := models.Contract{
		Number:       "TEST-001",
		Title:        "Тестовый договор",
		ClientName:   "Тестовый клиент",
		StartDate:    time.Now(),
		EndDate:      time.Now().AddDate(1, 0, 0),
		TariffPlanID: billingPlan.ID,
		CompanyID:    company.ID,
	}
	db.Create(&contract)

	// Создаем объект
	object := models.Object{
		Name:       "Тестовый объект",
		Type:       "vehicle",
		IMEI:       "123456789012345",
		ContractID: contract.ID,
		LocationID: location.ID,
	}
	db.Create(&object)

	// Создаем монтажника
	installer := models.Installer{
		FirstName:             "Иван",
		LastName:              "Иванов",
		Email:                 "installer@test.com",
		Phone:                 "+7900123456",
		Type:                  "штатный",
		Specialization:        []string{"GPS-трекер", "сигнализация"},
		MaxDailyInstallations: 3,
		WorkingDays:           []int{1, 2, 3, 4, 5}, // понедельник-пятница
		WorkingHoursStart:     "09:00",
		WorkingHoursEnd:       "18:00",
		LocationIDs:           []uint{location.ID},
	}
	db.Create(&installer)

	return object, installer, location
}

func TestScheduleInstallation(t *testing.T) {
	// Настройка
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	installationService := NewInstallationService(db, notificationService)

	// Создаем монтаж на следующий рабочий день (понедельник)
	nextMonday := getNextWeekday(time.Monday)

	installation := &models.Installation{
		Type:              "монтаж",
		ObjectID:          object.ID,
		InstallerID:       installer.ID,
		ScheduledAt:       nextMonday.Add(10 * time.Hour), // 10:00
		EstimatedDuration: 120,
		Description:       "Тестовый монтаж",
		Address:           "Тестовый адрес",
		ClientContact:     "+7900654321",
		Priority:          "normal",
	}

	// Выполнение
	err := installationService.ScheduleInstallation(installation)

	// Проверка
	assert.NoError(t, err)
	assert.NotZero(t, installation.ID)
	assert.Equal(t, "planned", installation.Status)

	// Проверяем, что монтаж сохранен в БД
	var savedInstallation models.Installation
	db.First(&savedInstallation, installation.ID)
	assert.Equal(t, "монтаж", savedInstallation.Type)
}

func TestScheduleInstallationWithConflict(t *testing.T) {
	// Настройка
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	installationService := NewInstallationService(db, notificationService)

	// Создаем существующий монтаж
	nextMonday := getNextWeekday(time.Monday)
	scheduledTime := nextMonday.Add(10 * time.Hour)

	existingInstallation := &models.Installation{
		Type:              "диагностика",
		ObjectID:          object.ID,
		InstallerID:       installer.ID,
		ScheduledAt:       scheduledTime,
		EstimatedDuration: 60,
		Status:            "planned",
	}
	db.Create(existingInstallation)

	// Пытаемся создать конфликтующий монтаж
	newInstallation := &models.Installation{
		Type:              "монтаж",
		ObjectID:          object.ID,
		InstallerID:       installer.ID,
		ScheduledAt:       scheduledTime.Add(30 * time.Minute), // через 30 минут - должен быть конфликт
		EstimatedDuration: 120,
	}

	// Выполнение
	err := installationService.ScheduleInstallation(newInstallation)

	// Проверка - должна быть ошибка конфликта
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "уже есть работы в это время")
}

func TestScheduleInstallationOnWeekend(t *testing.T) {
	// Настройка
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	installationService := NewInstallationService(db, notificationService)

	// Пытаемся создать монтаж на выходные (суббота)
	nextSaturday := getNextWeekday(time.Saturday)

	installation := &models.Installation{
		Type:              "монтаж",
		ObjectID:          object.ID,
		InstallerID:       installer.ID,
		ScheduledAt:       nextSaturday.Add(10 * time.Hour),
		EstimatedDuration: 120,
	}

	// Выполнение
	err := installationService.ScheduleInstallation(installation)

	// Проверка - должна быть ошибка, так как монтажник не работает в выходные
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "недоступен на указанную дату")
}

func TestScheduleInstallationExceedsMaxDaily(t *testing.T) {
	// Настройка
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	installationService := NewInstallationService(db, notificationService)

	nextMonday := getNextWeekday(time.Monday)

	// Создаем максимальное количество монтажей на день (3)
	for i := 0; i < 3; i++ {
		installation := &models.Installation{
			Type:              "монтаж",
			ObjectID:          object.ID,
			InstallerID:       installer.ID,
			ScheduledAt:       nextMonday.Add(time.Duration(9+i*3) * time.Hour), // 9:00, 12:00, 15:00
			EstimatedDuration: 120,
			Status:            "planned",
		}
		db.Create(installation)
	}

	// Пытаемся создать четвертый монтаж
	fourthInstallation := &models.Installation{
		Type:              "диагностика",
		ObjectID:          object.ID,
		InstallerID:       installer.ID,
		ScheduledAt:       nextMonday.Add(18 * time.Hour), // 18:00
		EstimatedDuration: 60,
	}

	// Выполнение
	err := installationService.ScheduleInstallation(fourthInstallation)

	// Проверка - должна быть ошибка превышения лимита
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "превышено максимальное количество монтажей в день")
}

func TestCheckScheduleConflicts(t *testing.T) {
	// Настройка
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	installationService := NewInstallationService(db, notificationService)

	nextMonday := getNextWeekday(time.Monday)
	scheduledTime := nextMonday.Add(10 * time.Hour) // 10:00

	// Создаем существующий монтаж
	existingInstallation := &models.Installation{
		Type:              "диагностика",
		ObjectID:          object.ID,
		InstallerID:       installer.ID,
		ScheduledAt:       scheduledTime,
		EstimatedDuration: 60,
		Status:            "planned",
	}
	db.Create(existingInstallation)

	// Тестируем различные сценарии конфликтов
	testCases := []struct {
		name           string
		scheduledAt    time.Time
		duration       int
		shouldConflict bool
	}{
		{
			name:           "Конфликт - точно в то же время",
			scheduledAt:    scheduledTime,
			duration:       60,
			shouldConflict: true,
		},
		{
			name:           "Конфликт - перекрытие начала",
			scheduledAt:    scheduledTime.Add(-30 * time.Minute),
			duration:       60,
			shouldConflict: true,
		},
		{
			name:           "Конфликт - перекрытие конца",
			scheduledAt:    scheduledTime.Add(30 * time.Minute),
			duration:       60,
			shouldConflict: true,
		},
		{
			name:           "Нет конфликта - достаточный интервал",
			scheduledAt:    scheduledTime.Add(2 * time.Hour),
			duration:       60,
			shouldConflict: false,
		},
		{
			name:           "Нет конфликта - до существующего монтажа",
			scheduledAt:    scheduledTime.Add(-2 * time.Hour),
			duration:       60,
			shouldConflict: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conflicts, err := installationService.CheckScheduleConflicts(
				installer.ID, tc.scheduledAt, tc.duration, 0)

			assert.NoError(t, err)

			if tc.shouldConflict {
				assert.NotEmpty(t, conflicts, "Должен быть конфликт")
			} else {
				assert.Empty(t, conflicts, "Не должно быть конфликтов")
			}
		})
	}
}

func TestRescheduleInstallation(t *testing.T) {
	// Настройка
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	installationService := NewInstallationService(db, notificationService)

	nextMonday := getNextWeekday(time.Monday)
	originalTime := nextMonday.Add(10 * time.Hour)
	newTime := nextMonday.Add(14 * time.Hour) // 14:00

	// Создаем монтаж
	installation := &models.Installation{
		Type:              "монтаж",
		ObjectID:          object.ID,
		InstallerID:       installer.ID,
		ScheduledAt:       originalTime,
		EstimatedDuration: 120,
		Status:            "planned",
	}
	db.Create(installation)

	// Выполнение переноса
	err := installationService.RescheduleInstallation(installation.ID, newTime, nil)

	// Проверка
	assert.NoError(t, err)

	// Проверяем изменения в БД
	var updatedInstallation models.Installation
	db.First(&updatedInstallation, installation.ID)
	assert.Equal(t, newTime.Unix(), updatedInstallation.ScheduledAt.Unix())
	assert.Equal(t, "planned", updatedInstallation.Status)
}

func TestRescheduleInstallationWithNewInstaller(t *testing.T) {
	// Настройка
	db := setupServiceTestDB()
	object, installer1, location := createServiceTestData(db)

	// Создаем второго монтажника
	installer2 := models.Installer{
		FirstName:             "Петр",
		LastName:              "Петров",
		Email:                 "installer2@test.com",
		Phone:                 "+7900123457",
		Type:                  "штатный",
		Specialization:        []string{"GPS-трекер"},
		MaxDailyInstallations: 3,
		WorkingDays:           []int{1, 2, 3, 4, 5},
		WorkingHoursStart:     "09:00",
		WorkingHoursEnd:       "18:00",
		LocationIDs:           []uint{location.ID},
	}
	db.Create(&installer2)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	installationService := NewInstallationService(db, notificationService)

	nextMonday := getNextWeekday(time.Monday)
	newTime := nextMonday.Add(14 * time.Hour)

	// Создаем монтаж для первого монтажника
	installation := &models.Installation{
		Type:              "монтаж",
		ObjectID:          object.ID,
		InstallerID:       installer1.ID,
		ScheduledAt:       nextMonday.Add(10 * time.Hour),
		EstimatedDuration: 120,
		Status:            "planned",
	}
	db.Create(installation)

	// Переносим на другого монтажника
	err := installationService.RescheduleInstallation(installation.ID, newTime, &installer2.ID)

	// Проверка
	assert.NoError(t, err)

	// Проверяем изменения в БД
	var updatedInstallation models.Installation
	db.First(&updatedInstallation, installation.ID)
	assert.Equal(t, installer2.ID, updatedInstallation.InstallerID)
	assert.Equal(t, newTime.Unix(), updatedInstallation.ScheduledAt.Unix())
}

func TestGetAvailableInstallers(t *testing.T) {
	// Настройка
	db := setupServiceTestDB()
	_, installer1, location := createServiceTestData(db)

	// Создаем второго монтажника с другой специализацией
	installer2 := models.Installer{
		FirstName:             "Петр",
		LastName:              "Петров",
		Email:                 "installer2@test.com",
		Phone:                 "+7900123457",
		Type:                  "штатный",
		Specialization:        []string{"видеонаблюдение"},
		MaxDailyInstallations: 3,
		WorkingDays:           []int{1, 2, 3, 4, 5},
		WorkingHoursStart:     "09:00",
		WorkingHoursEnd:       "18:00",
		LocationIDs:           []uint{location.ID},
	}
	db.Create(&installer2)

	// Создаем неактивного монтажника
	installer3 := models.Installer{
		FirstName:             "Сидор",
		LastName:              "Сидоров",
		Email:                 "installer3@test.com",
		Phone:                 "+7900123458",
		Type:                  "штатный",
		Specialization:        []string{"GPS-трекер"},
		MaxDailyInstallations: 3,
		WorkingDays:           []int{1, 2, 3, 4, 5},
		WorkingHoursStart:     "09:00",
		WorkingHoursEnd:       "18:00",
		LocationIDs:           []uint{location.ID},
		IsActive:              false, // неактивен
	}
	db.Create(&installer3)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	installationService := NewInstallationService(db, notificationService)

	nextMonday := getNextWeekday(time.Monday)

	// Тестируем поиск доступных монтажников
	testCases := []struct {
		name           string
		specialization string
		expectedCount  int
		expectedIDs    []uint
	}{
		{
			name:           "Поиск по специализации GPS-трекер",
			specialization: "GPS-трекер",
			expectedCount:  1,
			expectedIDs:    []uint{installer1.ID},
		},
		{
			name:           "Поиск по специализации видеонаблюдение",
			specialization: "видеонаблюдение",
			expectedCount:  1,
			expectedIDs:    []uint{installer2.ID},
		},
		{
			name:           "Поиск без специализации",
			specialization: "",
			expectedCount:  2,
			expectedIDs:    []uint{installer1.ID, installer2.ID},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			installers, err := installationService.GetAvailableInstallers(
				nextMonday.Add(10*time.Hour), &location.ID, tc.specialization, 120)

			assert.NoError(t, err)
			assert.Len(t, installers, tc.expectedCount)

			if tc.expectedCount > 0 {
				var foundIDs []uint
				for _, installer := range installers {
					foundIDs = append(foundIDs, installer.ID)
				}

				for _, expectedID := range tc.expectedIDs {
					assert.Contains(t, foundIDs, expectedID)
				}
			}
		})
	}
}

func TestGetInstallerWorkload(t *testing.T) {
	// Настройка
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	installationService := NewInstallationService(db, notificationService)

	nextMonday := getNextWeekday(time.Monday)
	nextTuesday := nextMonday.Add(24 * time.Hour)

	// Создаем несколько монтажей с разными статусами
	installations := []models.Installation{
		{
			Type:              "монтаж",
			ObjectID:          object.ID,
			InstallerID:       installer.ID,
			ScheduledAt:       nextMonday.Add(10 * time.Hour),
			EstimatedDuration: 120,
			Status:            "planned",
		},
		{
			Type:              "диагностика",
			ObjectID:          object.ID,
			InstallerID:       installer.ID,
			ScheduledAt:       nextMonday.Add(14 * time.Hour),
			EstimatedDuration: 60,
			Status:            "completed",
			ActualDuration:    45,
		},
		{
			Type:              "монтаж",
			ObjectID:          object.ID,
			InstallerID:       installer.ID,
			ScheduledAt:       nextTuesday.Add(10 * time.Hour),
			EstimatedDuration: 90,
			Status:            "in_progress",
		},
	}

	for _, installation := range installations {
		db.Create(&installation)
	}

	// Выполнение
	dateFrom := nextMonday.Truncate(24 * time.Hour)
	dateTo := dateFrom.Add(7 * 24 * time.Hour)

	workload, err := installationService.GetInstallerWorkload(installer.ID, dateFrom, dateTo)

	// Проверка
	assert.NoError(t, err)
	assert.NotNil(t, workload)
	assert.Equal(t, installer.ID, workload.InstallerID)
	assert.Equal(t, 3, workload.TotalInstallations)
	assert.Equal(t, 1, workload.PlannedCount)
	assert.Equal(t, 1, workload.CompletedCount)
	assert.Equal(t, 1, workload.InProgressCount)
	assert.Equal(t, 270, workload.TotalEstimatedTime) // 120 + 60 + 90
	assert.Equal(t, 45, workload.TotalActualTime)     // только завершенный
	assert.NotEmpty(t, workload.DailyWorkload)
}

func TestGetOverdueInstallations(t *testing.T) {
	// Настройка
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	installationService := NewInstallationService(db, notificationService)

	// Создаем просроченные и непросроченные монтажи
	installations := []models.Installation{
		{
			Type:        "монтаж",
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			ScheduledAt: time.Now().Add(-2 * time.Hour), // 2 часа назад - просрочен
			Status:      "planned",
		},
		{
			Type:        "диагностика",
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			ScheduledAt: time.Now().Add(-1 * time.Hour), // час назад - просрочен
			Status:      "in_progress",
		},
		{
			Type:        "монтаж",
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			ScheduledAt: time.Now().Add(2 * time.Hour), // через 2 часа - не просрочен
			Status:      "planned",
		},
		{
			Type:        "диагностика",
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			ScheduledAt: time.Now().Add(-3 * time.Hour), // 3 часа назад но завершен
			Status:      "completed",
		},
	}

	for _, installation := range installations {
		db.Create(&installation)
	}

	// Выполнение
	overdueInstallations, err := installationService.GetOverdueInstallations()

	// Проверка
	assert.NoError(t, err)
	assert.Len(t, overdueInstallations, 2) // только 2 просроченных (planned и in_progress)

	for _, installation := range overdueInstallations {
		assert.True(t, installation.ScheduledAt.Before(time.Now()))
		assert.Contains(t, []string{"planned", "in_progress"}, installation.Status)
	}
}

// Вспомогательная функция для получения следующего дня недели
func getNextWeekday(weekday time.Weekday) time.Time {
	now := time.Now()
	daysUntil := int(weekday - now.Weekday())
	if daysUntil <= 0 {
		daysUntil += 7
	}
	return now.AddDate(0, 0, daysUntil)
}

// Benchmark тесты

func BenchmarkScheduleInstallation(b *testing.B) {
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	installationService := NewInstallationService(db, notificationService)

	nextMonday := getNextWeekday(time.Monday)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		installation := &models.Installation{
			Type:              "монтаж",
			ObjectID:          object.ID,
			InstallerID:       installer.ID,
			ScheduledAt:       nextMonday.Add(time.Duration(i) * time.Hour), // разное время
			EstimatedDuration: 120,
		}

		installationService.ScheduleInstallation(installation)
	}
}

func BenchmarkCheckScheduleConflicts(b *testing.B) {
	db := setupServiceTestDB()
	object, installer, _ := createServiceTestData(db)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	installationService := NewInstallationService(db, notificationService)

	// Создаем много монтажей для тестирования производительности
	nextMonday := getNextWeekday(time.Monday)
	for i := 0; i < 100; i++ {
		installation := &models.Installation{
			Type:        "монтаж",
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			ScheduledAt: nextMonday.Add(time.Duration(i) * time.Hour),
			Status:      "planned",
		}
		db.Create(installation)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		installationService.CheckScheduleConflicts(
			installer.ID, nextMonday.Add(time.Duration(i)*time.Minute), 120, 0)
	}
}
