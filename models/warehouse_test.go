package models

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEquipmentModel тестирует модель Equipment
func TestEquipmentModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание оборудования", func(t *testing.T) {
		purchaseDate := time.Now().AddDate(0, -3, 0) // 3 месяца назад
		warrantyUntil := time.Now().AddDate(2, 0, 0) // Гарантия на 2 года

		equipment := Equipment{
			Type:              "GPS-tracker",
			Model:             "GT06N",
			Brand:             "Concox",
			SerialNumber:      "GT06N123456789",
			IMEI:              "123456789012345",
			PhoneNumber:       "+7-900-123-45-67",
			MACAddress:        "AA:BB:CC:DD:EE:FF",
			Status:            "in_stock",
			Condition:         "new",
			WarehouseLocation: "A1-B2-C3",
			PurchasePrice:     decimal.NewFromFloat(2500.0),
			PurchaseDate:      &purchaseDate,
			WarrantyUntil:     &warrantyUntil,
			Specifications:    `{"voltage": "12V", "frequency": "GSM 900/1800", "gps_accuracy": "5m"}`,
			Notes:             "Новое оборудование для тестирования",
		}

		err := db.Create(&equipment).Error
		require.NoError(t, err)
		assert.NotZero(t, equipment.ID)
		assert.Equal(t, "GPS-tracker", equipment.Type)
		assert.Equal(t, "GT06N123456789", equipment.SerialNumber)
		assert.Equal(t, "123456789012345", equipment.IMEI)
	})

	t.Run("Уникальность серийного номера и IMEI", func(t *testing.T) {
		// Создаем первое оборудование
		equipment1 := Equipment{
			Type:         "GPS-tracker",
			Model:        "Model1",
			SerialNumber: "UNIQUE_SERIAL_001",
			IMEI:         "unique123456789",
			Status:       "in_stock",
			Condition:    "new",
		}
		err := db.Create(&equipment1).Error
		require.NoError(t, err)

		// Попытка создать оборудование с тем же серийным номером
		equipment2 := Equipment{
			Type:         "GPS-tracker",
			Model:        "Model2",
			SerialNumber: "UNIQUE_SERIAL_001",
			IMEI:         "different123456789",
			Status:       "in_stock",
			Condition:    "new",
		}
		err = db.Create(&equipment2).Error
		assert.Error(t, err, "Должна быть ошибка из-за дублирования серийного номера")

		// Попытка создать оборудование с тем же IMEI
		equipment3 := Equipment{
			Type:         "GPS-tracker",
			Model:        "Model3",
			SerialNumber: "DIFFERENT_SERIAL_001",
			IMEI:         "unique123456789",
			Status:       "in_stock",
			Condition:    "new",
		}
		err = db.Create(&equipment3).Error
		assert.Error(t, err, "Должна быть ошибка из-за дублирования IMEI")
	})

	t.Run("Метод IsAvailable", func(t *testing.T) {
		// Доступное оборудование
		availableEquipment := Equipment{
			Type:         "sensor",
			Model:        "AvailableModel",
			SerialNumber: "AVAILABLE_001",
			Status:       "in_stock",
			Condition:    "new",
		}
		err := db.Create(&availableEquipment).Error
		require.NoError(t, err)
		assert.True(t, availableEquipment.IsAvailable())

		// Недоступное оборудование - зарезервировано
		reservedEquipment := Equipment{
			Type:         "sensor",
			Model:        "ReservedModel",
			SerialNumber: "RESERVED_001",
			Status:       "reserved",
			Condition:    "new",
		}
		err = db.Create(&reservedEquipment).Error
		require.NoError(t, err)
		assert.False(t, reservedEquipment.IsAvailable())

		// Недоступное оборудование - повреждено
		damagedEquipment := Equipment{
			Type:         "sensor",
			Model:        "DamagedModel",
			SerialNumber: "DAMAGED_001",
			Status:       "in_stock",
			Condition:    "damaged",
		}
		err = db.Create(&damagedEquipment).Error
		require.NoError(t, err)
		assert.False(t, damagedEquipment.IsAvailable())
	})

	t.Run("Метод NeedsAttention", func(t *testing.T) {
		// Оборудование в обслуживании
		maintenanceEquipment := Equipment{
			Type:         "camera",
			Model:        "MaintenanceModel",
			SerialNumber: "MAINTENANCE_001",
			Status:       "maintenance",
			Condition:    "used",
		}
		err := db.Create(&maintenanceEquipment).Error
		require.NoError(t, err)
		assert.True(t, maintenanceEquipment.NeedsAttention())

		// Поврежденное оборудование
		damagedEquipment := Equipment{
			Type:         "camera",
			Model:        "DamagedModel",
			SerialNumber: "DAMAGED_ATTENTION_001",
			Status:       "in_stock",
			Condition:    "damaged",
		}
		err = db.Create(&damagedEquipment).Error
		require.NoError(t, err)
		assert.True(t, damagedEquipment.NeedsAttention())

		// Оборудование с истекшей гарантией
		expiredWarranty := time.Now().AddDate(-1, 0, 0) // Гарантия истекла год назад
		expiredWarrantyEquipment := Equipment{
			Type:          "tracker",
			Model:         "ExpiredModel",
			SerialNumber:  "EXPIRED_WARRANTY_001",
			Status:        "in_stock",
			Condition:     "used",
			WarrantyUntil: &expiredWarranty,
		}
		err = db.Create(&expiredWarrantyEquipment).Error
		require.NoError(t, err)
		assert.True(t, expiredWarrantyEquipment.NeedsAttention())

		// Нормальное оборудование
		normalEquipment := Equipment{
			Type:         "tracker",
			Model:        "NormalModel",
			SerialNumber: "NORMAL_001",
			Status:       "in_stock",
			Condition:    "new",
		}
		err = db.Create(&normalEquipment).Error
		require.NoError(t, err)
		assert.False(t, normalEquipment.NeedsAttention())
	})

	t.Run("Связь с объектом", func(t *testing.T) {
		// Создаем необходимые связанные сущности для объекта
		billingPlan := BillingPlan{
			Name:          "План для оборудования",
			Price:         1000.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "EQUIPMENT-CONTRACT-001",
			Title:        "Договор для оборудования",
			ClientName:   "ООО Оборудование",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(12000.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		object := Object{
			Name:       "Объект с оборудованием",
			Type:       "vehicle",
			IMEI:       "object123456789",
			ContractID: contract.ID,
			IsActive:   true,
		}
		err = db.Create(&object).Error
		require.NoError(t, err)

		// Создаем оборудование, привязанное к объекту
		equipment := Equipment{
			Type:         "GPS-tracker",
			Model:        "InstalledModel",
			SerialNumber: "INSTALLED_001",
			IMEI:         "installed123456",
			Status:       "installed",
			Condition:    "new",
			ObjectID:     &object.ID,
		}
		err = db.Create(&equipment).Error
		require.NoError(t, err)

		// Загружаем оборудование с объектом
		var equipmentWithObject Equipment
		err = db.Preload("Object").First(&equipmentWithObject, equipment.ID).Error
		require.NoError(t, err)
		assert.NotNil(t, equipmentWithObject.Object)
		assert.Equal(t, "Объект с оборудованием", equipmentWithObject.Object.Name)
	})

	t.Run("Обновление времени последнего обслуживания", func(t *testing.T) {
		equipment := Equipment{
			Type:         "maintenance-tracker",
			Model:        "MaintenanceTestModel",
			SerialNumber: "MAINTENANCE_TEST_001",
			Status:       "in_stock",
			Condition:    "used",
		}
		err := db.Create(&equipment).Error
		require.NoError(t, err)

		// Обновляем время последнего обслуживания
		lastMaintenance := time.Now()
		equipment.LastMaintenanceAt = &lastMaintenance

		err = db.Save(&equipment).Error
		require.NoError(t, err)

		// Проверяем обновление
		var updatedEquipment Equipment
		err = db.First(&updatedEquipment, equipment.ID).Error
		require.NoError(t, err)
		assert.NotNil(t, updatedEquipment.LastMaintenanceAt)
	})
}

// TestLocationModel тестирует модель Location
func TestLocationModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание локации", func(t *testing.T) {
		location := Location{
			City:      "Екатеринбург",
			Region:    "Свердловская область",
			Country:   "Russia",
			Latitude:  floatPtr(56.8431),
			Longitude: floatPtr(60.6454),
			Timezone:  "Asia/Yekaterinburg",
			IsActive:  true,
			Notes:     "Крупный промышленный центр Урала",
		}

		err := db.Create(&location).Error
		require.NoError(t, err)
		assert.NotZero(t, location.ID)
		assert.Equal(t, "Екатеринбург", location.City)
		assert.Equal(t, "Asia/Yekaterinburg", location.Timezone)
		assert.NotNil(t, location.Latitude)
		assert.NotNil(t, location.Longitude)
	})

	t.Run("Метод GetFullName", func(t *testing.T) {
		// Локация с регионом
		locationWithRegion := Location{
			City:     "Новосибирск",
			Region:   "Новосибирская область",
			Country:  "Russia",
			IsActive: true,
		}
		err := db.Create(&locationWithRegion).Error
		require.NoError(t, err)
		assert.Equal(t, "Новосибирск, Новосибирская область", locationWithRegion.GetFullName())

		// Локация без региона
		locationWithoutRegion := Location{
			City:     "Сочи",
			Country:  "Russia",
			IsActive: true,
		}
		err = db.Create(&locationWithoutRegion).Error
		require.NoError(t, err)
		assert.Equal(t, "Сочи", locationWithoutRegion.GetFullName())
	})

	t.Run("Связь с объектами", func(t *testing.T) {
		// Создаем локацию
		location := Location{
			City:     "Казань",
			Region:   "Республика Татарстан",
			Country:  "Russia",
			Timezone: "Europe/Moscow",
			IsActive: true,
		}
		err := db.Create(&location).Error
		require.NoError(t, err)

		// Создаем необходимые сущности для объектов
		billingPlan := BillingPlan{
			Name:          "План для локации",
			Price:         800.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err = db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "LOCATION-CONTRACT-001",
			Title:        "Договор для локации",
			ClientName:   "ООО Локация",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(9600.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		// Создаем объекты в этой локации
		object1 := Object{
			Name:       "Объект в Казани 1",
			Type:       "vehicle",
			IMEI:       "kazan123456789",
			ContractID: contract.ID,
			LocationID: location.ID,
			IsActive:   true,
		}
		object2 := Object{
			Name:       "Объект в Казани 2",
			Type:       "equipment",
			IMEI:       "kazan987654321",
			ContractID: contract.ID,
			LocationID: location.ID,
			IsActive:   true,
		}

		err = db.Create(&object1).Error
		require.NoError(t, err)
		err = db.Create(&object2).Error
		require.NoError(t, err)

		// Загружаем локацию с объектами
		var locationWithObjects Location
		err = db.Preload("Objects").First(&locationWithObjects, location.ID).Error
		require.NoError(t, err)
		assert.Len(t, locationWithObjects.Objects, 2)
	})

	t.Run("Часовые пояса", func(t *testing.T) {
		locations := []Location{
			{
				City:     "Москва",
				Region:   "Московская область",
				Country:  "Russia",
				Timezone: "Europe/Moscow",
				IsActive: true,
			},
			{
				City:     "Владивосток",
				Region:   "Приморский край",
				Country:  "Russia",
				Timezone: "Asia/Vladivostok",
				IsActive: true,
			},
			{
				City:     "Калининград",
				Region:   "Калининградская область",
				Country:  "Russia",
				Timezone: "Europe/Kaliningrad",
				IsActive: true,
			},
		}

		for _, loc := range locations {
			err := db.Create(&loc).Error
			require.NoError(t, err)
			assert.Contains(t, loc.Timezone, "/")
		}
	})
}

// TestInstallerModel тестирует модель Installer
func TestInstallerModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание монтажника", func(t *testing.T) {
		installer := Installer{
			FirstName:      "Иван",
			LastName:       "Иванов",
			Type:           "staff",
			Phone:          "+7-900-123-45-67",
			Email:          "ivan.ivanov@example.com",
			TelegramID:     "@ivan_installer",
			Specialization: []string{"GPS-trackers", "sensors", "cameras"},
			SkillLevel:     "senior",
			LocationIDs:    []uint{1, 2, 3},
			WorkingDays:    []int{1, 2, 3, 4, 5},
			HourlyRate:     decimal.NewFromFloat(1500.0),
			IsActive:       true,
			Rating:         4.8,
			CompletedJobs:  127,
			Notes:          "Опытный монтажник с большим стажем работы",
		}

		err := db.Create(&installer).Error
		require.NoError(t, err)
		assert.NotZero(t, installer.ID)
		assert.Equal(t, "Иван Иванов", installer.GetFullName())
		assert.Equal(t, "staff", installer.Type)
		assert.Equal(t, "senior", installer.SkillLevel)
		assert.Equal(t, float32(4.8), installer.Rating)
	})

	t.Run("Метод CanWorkInLocation", func(t *testing.T) {
		installer := Installer{
			FirstName:   "Петр",
			LastName:    "Петров",
			Type:        "contractor",
			Phone:       "+7-900-000-00-01",
			LocationIDs: []uint{10, 20, 30},
			IsActive:    true,
		}
		err := db.Create(&installer).Error
		require.NoError(t, err)

		// Тестируем метод CanWorkInLocation
		assert.True(t, installer.CanWorkInLocation(10))
		assert.True(t, installer.CanWorkInLocation(20))
		assert.True(t, installer.CanWorkInLocation(30))
		assert.False(t, installer.CanWorkInLocation(40))
		assert.False(t, installer.CanWorkInLocation(1))
	})

	t.Run("Метод HasSpecialization", func(t *testing.T) {
		installer := Installer{
			FirstName:      "Сергей",
			LastName:       "Сергеев",
			Type:           "staff",
			Phone:          "+7-900-000-00-02",
			Specialization: []string{"GPS-trackers", "fuel-sensors", "temperature-sensors"},
			IsActive:       true,
		}
		err := db.Create(&installer).Error
		require.NoError(t, err)

		// Тестируем метод HasSpecialization
		assert.True(t, installer.HasSpecialization("GPS-trackers"))
		assert.True(t, installer.HasSpecialization("fuel-sensors"))
		assert.True(t, installer.HasSpecialization("temperature-sensors"))
		assert.False(t, installer.HasSpecialization("cameras"))
		assert.False(t, installer.HasSpecialization("alarms"))
	})

	t.Run("Связь с локациями", func(t *testing.T) {
		// Создаем локации
		location1 := Location{
			City:     "Ростов-на-Дону",
			Region:   "Ростовская область",
			Country:  "Russia",
			IsActive: true,
		}
		location2 := Location{
			City:     "Краснодар",
			Region:   "Краснодарский край",
			Country:  "Russia",
			IsActive: true,
		}

		err := db.Create(&location1).Error
		require.NoError(t, err)
		err = db.Create(&location2).Error
		require.NoError(t, err)

		// Создаем монтажника
		installer := Installer{
			FirstName:   "Александр",
			LastName:    "Александров",
			Type:        "contractor",
			Phone:       "+7-900-000-00-03",
			LocationIDs: []uint{location1.ID, location2.ID},
			IsActive:    true,
		}
		err = db.Create(&installer).Error
		require.NoError(t, err)

		// Связываем монтажника с локациями через many2many
		err = db.Model(&installer).Association("Locations").Append(&location1, &location2)
		require.NoError(t, err)

		// Загружаем монтажника с локациями
		var installerWithLocations Installer
		err = db.Preload("Locations").First(&installerWithLocations, installer.ID).Error
		require.NoError(t, err)
		assert.Len(t, installerWithLocations.Locations, 2)
	})

	t.Run("Различные типы монтажников", func(t *testing.T) {
		// Штатный монтажник
		staffInstaller := Installer{
			FirstName:  "Штатный",
			LastName:   "Сотрудник",
			Type:       "staff",
			Phone:      "+7-900-000-00-04",
			SkillLevel: "middle",
			HourlyRate: decimal.Zero, // Штатные без почасовой оплаты
			IsActive:   true,
			Rating:     5.0,
		}
		err := db.Create(&staffInstaller).Error
		require.NoError(t, err)

		// Наемный монтажник
		contractorInstaller := Installer{
			FirstName:  "Наемный",
			LastName:   "Подрядчик",
			Type:       "contractor",
			Phone:      "+7-900-000-00-05",
			SkillLevel: "junior",
			HourlyRate: decimal.NewFromFloat(800.0),
			IsActive:   true,
			Rating:     4.2,
		}
		err = db.Create(&contractorInstaller).Error
		require.NoError(t, err)

		// Проверяем создание
		var staff, contractor Installer
		err = db.First(&staff, staffInstaller.ID).Error
		require.NoError(t, err)
		assert.Equal(t, "staff", staff.Type)
		assert.True(t, staff.HourlyRate.IsZero())

		err = db.First(&contractor, contractorInstaller.ID).Error
		require.NoError(t, err)
		assert.Equal(t, "contractor", contractor.Type)
		assert.Equal(t, decimal.NewFromFloat(800.0), contractor.HourlyRate)
	})
}

// TestInstallationModel тестирует модель Installation
func TestInstallationModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание монтажа", func(t *testing.T) {
		// Создаем необходимые связанные сущности
		billingPlan := BillingPlan{
			Name:          "План для монтажа",
			Price:         1200.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "INSTALL-CONTRACT-001",
			Title:        "Договор для монтажа",
			ClientName:   "ООО Монтаж",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(14400.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		object := Object{
			Name:       "Объект для монтажа",
			Type:       "vehicle",
			IMEI:       "install123456789",
			ContractID: contract.ID,
			IsActive:   true,
		}
		err = db.Create(&object).Error
		require.NoError(t, err)

		installer := Installer{
			FirstName: "Монтажник",
			LastName:  "Тестовый",
			Type:      "staff",
			Phone:     "+7-900-000-00-06",
			IsActive:  true,
		}
		err = db.Create(&installer).Error
		require.NoError(t, err)

		// Создаем монтаж
		scheduledAt := time.Now().Add(time.Hour * 24) // Завтра
		installation := Installation{
			ObjectID:          object.ID,
			InstallerID:       installer.ID,
			Type:              "installation",
			Priority:          "normal",
			ScheduledAt:       scheduledAt,
			EstimatedDuration: 120, // 2 часа
			Status:            "planned",
			Notes:             "Установка GPS-трекера",
			Cost:              decimal.NewFromFloat(3000.0),
			IsBillable:        true,
		}

		err = db.Create(&installation).Error
		require.NoError(t, err)
		assert.NotZero(t, installation.ID)
		assert.Equal(t, "installation", installation.Type)
		assert.Equal(t, "planned", installation.Status)
		assert.Equal(t, 120, installation.EstimatedDuration)
	})

	t.Run("Методы проверки статуса", func(t *testing.T) {
		// Создаем минимальные связанные сущности
		billingPlan := BillingPlan{
			Name:          "План статуса",
			Price:         1000.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "STATUS-CONTRACT-001",
			Title:        "Договор статуса",
			ClientName:   "ООО Статус",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(12000.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		object := Object{
			Name:       "Объект статуса",
			Type:       "equipment",
			IMEI:       "status123456789",
			ContractID: contract.ID,
			IsActive:   true,
		}
		err = db.Create(&object).Error
		require.NoError(t, err)

		installer := Installer{
			FirstName: "Статусный",
			LastName:  "Монтажник",
			Type:      "contractor",
			Phone:     "+7-900-000-00-07",
			IsActive:  true,
		}
		err = db.Create(&installer).Error
		require.NoError(t, err)

		// Просроченный монтаж
		overdueInstallation := Installation{
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			Type:        "maintenance",
			ScheduledAt: time.Now().Add(-time.Hour * 24), // Вчера
			Status:      "planned",
		}
		err = db.Create(&overdueInstallation).Error
		require.NoError(t, err)
		assert.True(t, overdueInstallation.IsOverdue())

		// Завершенный монтаж
		startedAt := time.Now().Add(-time.Hour * 3)
		completedAt := time.Now().Add(-time.Hour * 1)
		completedInstallation := Installation{
			ObjectID:       object.ID,
			InstallerID:    installer.ID,
			Type:           "installation",
			ScheduledAt:    time.Now().Add(-time.Hour * 4),
			StartedAt:      &startedAt,
			CompletedAt:    &completedAt,
			ActualDuration: 120,
			Status:         "completed",
		}
		err = db.Create(&completedInstallation).Error
		require.NoError(t, err)
		assert.False(t, completedInstallation.IsOverdue())
		assert.True(t, completedInstallation.IsCompleted())
		assert.Equal(t, time.Hour*2, completedInstallation.GetDuration())

		// Запланированный монтаж
		plannedInstallation := Installation{
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			Type:        "diagnostics",
			ScheduledAt: time.Now().Add(time.Hour * 24), // Завтра
			Status:      "planned",
		}
		err = db.Create(&plannedInstallation).Error
		require.NoError(t, err)
		assert.False(t, plannedInstallation.IsOverdue())
		assert.False(t, plannedInstallation.IsCompleted())
	})

	t.Run("Связи монтажа", func(t *testing.T) {
		// Создаем все необходимые связанные сущности
		billingPlan := BillingPlan{
			Name:          "План связей",
			Price:         1500.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "RELATIONS-INSTALL-001",
			Title:        "Договор связей монтажа",
			ClientName:   "ООО Связи Монтажа",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(18000.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		object := Object{
			Name:       "Объект связей монтажа",
			Type:       "asset",
			IMEI:       "relations123456",
			ContractID: contract.ID,
			IsActive:   true,
		}
		err = db.Create(&object).Error
		require.NoError(t, err)

		installer := Installer{
			FirstName: "Связной",
			LastName:  "Монтажник",
			Type:      "staff",
			Phone:     "+7-900-000-00-00",
			IsActive:  true,
		}
		err = db.Create(&installer).Error
		require.NoError(t, err)

		// Создаем оборудование для монтажа
		equipment1 := Equipment{
			Type:         "GPS-tracker",
			Model:        "InstallModel1",
			SerialNumber: "INSTALL_EQ_001",
			Status:       "reserved",
			Condition:    "new",
		}
		equipment2 := Equipment{
			Type:         "sensor",
			Model:        "InstallModel2",
			SerialNumber: "INSTALL_EQ_002",
			Status:       "reserved",
			Condition:    "new",
		}

		err = db.Create(&equipment1).Error
		require.NoError(t, err)
		err = db.Create(&equipment2).Error
		require.NoError(t, err)

		// Создаем монтаж
		installation := Installation{
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			Type:        "installation",
			ScheduledAt: time.Now().Add(time.Hour * 48),
			Status:      "planned",
			Notes:       "Установка комплекса оборудования",
			Issues:      "",
			Photos:      []string{},
			Cost:        decimal.NewFromFloat(5000.0),
			IsBillable:  true,
		}
		err = db.Create(&installation).Error
		require.NoError(t, err)

		// Связываем монтаж с оборудованием
		err = db.Model(&installation).Association("Equipment").Append(&equipment1, &equipment2)
		require.NoError(t, err)

		// Загружаем монтаж со всеми связями
		var installationWithRelations Installation
		err = db.Preload("Object").Preload("Installer").Preload("Equipment").First(&installationWithRelations, installation.ID).Error
		require.NoError(t, err)

		assert.NotNil(t, installationWithRelations.Object)
		assert.Equal(t, "Объект связей монтажа", installationWithRelations.Object.Name)

		assert.NotNil(t, installationWithRelations.Installer)
		assert.Equal(t, "Связной Монтажник", installationWithRelations.Installer.GetFullName())

		assert.Len(t, installationWithRelations.Equipment, 2)
	})

	t.Run("Качество работы и обратная связь", func(t *testing.T) {
		// Создаем минимальные связанные сущности
		billingPlan := BillingPlan{
			Name:          "План качества",
			Price:         900.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "QUALITY-CONTRACT-001",
			Title:        "Договор качества",
			ClientName:   "ООО Качество",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(10800.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		object := Object{
			Name:       "Объект качества",
			Type:       "vehicle",
			IMEI:       "quality123456789",
			ContractID: contract.ID,
			IsActive:   true,
		}
		err = db.Create(&object).Error
		require.NoError(t, err)

		installer := Installer{
			FirstName: "Качественный",
			LastName:  "Монтажник",
			Type:      "staff",
			Phone:     "+7-900-000-00-08",
			IsActive:  true,
		}
		err = db.Create(&installer).Error
		require.NoError(t, err)

		// Создаем завершенный монтаж с оценкой качества
		rating := float32(4.5)
		installation := Installation{
			ObjectID:       object.ID,
			InstallerID:    installer.ID,
			Type:           "installation",
			ScheduledAt:    time.Now().Add(-time.Hour * 48),
			Status:         "completed",
			Notes:          "Установка прошла успешно",
			Issues:         "Небольшие сложности с креплением",
			Photos:         []string{"photo1.jpg", "photo2.jpg", "photo3.jpg"},
			QualityRating:  &rating,
			ClientFeedback: "Работа выполнена качественно, монтажник профессионал",
			Cost:           decimal.NewFromFloat(4000.0),
			IsBillable:     true,
		}

		err = db.Create(&installation).Error
		require.NoError(t, err)
		assert.NotNil(t, installation.QualityRating)
		assert.Equal(t, float32(4.5), *installation.QualityRating)
		assert.Len(t, installation.Photos, 3)
		assert.Contains(t, installation.ClientFeedback, "профессионал")
	})
}
