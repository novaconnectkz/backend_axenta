package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"backend_axenta/models"
)

func setupWarehouseServiceTest(t *testing.T) (*gorm.DB, *WarehouseService) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Автомиграция моделей
	err = db.AutoMigrate(
		&models.Equipment{},
		&models.EquipmentCategory{},
		&models.WarehouseOperation{},
		&models.StockAlert{},
		&models.Object{},
		&models.User{},
		&models.Role{},
	)
	assert.NoError(t, err)

	cache := NewCacheService(nil, nil) // Простой кэш для тестов
	notificationService := NewNotificationService(db, cache)
	warehouseService := NewWarehouseService(db, notificationService)

	return db, warehouseService
}

func TestWarehouseService_CheckLowStockLevels(t *testing.T) {
	db, ws := setupWarehouseServiceTest(t)

	// Создаем категорию с минимальным уровнем 5
	category := models.EquipmentCategory{
		Name:          "GPS Trackers",
		Code:          "GPS",
		MinStockLevel: 5,
		IsActive:      true,
	}
	db.Create(&category)

	// Создаем только 2 единицы оборудования (меньше минимума)
	for i := 0; i < 2; i++ {
		equipment := models.Equipment{
			Type:         "GPS-tracker",
			Model:        "GT06N",
			Brand:        "Concox",
			SerialNumber: "GT06N00" + string(rune(i+1)),
			Status:       "in_stock",
			CategoryID:   &category.ID,
		}
		db.Create(&equipment)
	}

	// Запускаем проверку
	err := ws.CheckLowStockLevels()
	assert.NoError(t, err)

	// Проверяем, что создано уведомление
	var alert models.StockAlert
	err = db.Where("equipment_category_id = ? AND type = 'low_stock'", category.ID).First(&alert).Error
	assert.NoError(t, err)
	assert.Equal(t, "active", alert.Status)
	assert.Contains(t, alert.Title, "GPS Trackers")
}

func TestWarehouseService_CheckExpiredWarranties(t *testing.T) {
	db, ws := setupWarehouseServiceTest(t)

	// Создаем оборудование с истекшей гарантией
	expiredDate := time.Now().AddDate(0, 0, -1) // Вчера
	equipment := models.Equipment{
		Type:          "GPS-tracker",
		Model:         "GT06N",
		Brand:         "Concox",
		SerialNumber:  "GT06N001",
		Status:        "in_stock",
		WarrantyUntil: &expiredDate,
	}
	db.Create(&equipment)

	// Запускаем проверку
	err := ws.CheckExpiredWarranties()
	assert.NoError(t, err)

	// Проверяем, что создано уведомление
	var alert models.StockAlert
	err = db.Where("equipment_id = ? AND type = 'expired_warranty'", equipment.ID).First(&alert).Error
	assert.NoError(t, err)
	assert.Equal(t, "active", alert.Status)
	assert.Contains(t, alert.Title, "Истекла гарантия")
}

func TestWarehouseService_ProcessEquipmentInstallation(t *testing.T) {
	db, ws := setupWarehouseServiceTest(t)

	// Создаем пользователя
	role := models.Role{Name: "installer", DisplayName: "Монтажник"}
	db.Create(&role)

	user := models.User{
		Username:  "installer",
		Email:     "installer@example.com",
		FirstName: "Иван",
		LastName:  "Монтажник",
		RoleID:    role.ID,
	}
	db.Create(&user)

	// Создаем объект
	object := models.Object{
		Name:      "Тестовый объект",
		Latitude:  func() *float64 { f := 55.7558; return &f }(),
		Longitude: func() *float64 { f := 37.6176; return &f }(),
		IMEI:      "123456789012345",
	}
	db.Create(&object)

	// Создаем доступное оборудование
	equipment := models.Equipment{
		Type:              "GPS-tracker",
		Model:             "GT06N",
		Brand:             "Concox",
		SerialNumber:      "GT06N001",
		Status:            "in_stock",
		Condition:         "new",
		WarehouseLocation: "A1-01",
	}
	db.Create(&equipment)

	// Обрабатываем установку
	err := ws.ProcessEquipmentInstallation(equipment.ID, object.ID, user.ID)
	assert.NoError(t, err)

	// Проверяем, что статус оборудования изменился
	var updatedEquipment models.Equipment
	db.First(&updatedEquipment, equipment.ID)
	assert.Equal(t, "installed", updatedEquipment.Status)
	assert.Equal(t, object.ID, *updatedEquipment.ObjectID)

	// Проверяем, что создана операция
	var operation models.WarehouseOperation
	err = db.Where("equipment_id = ? AND type = 'issue'", equipment.ID).First(&operation).Error
	assert.NoError(t, err)
	assert.Equal(t, "completed", operation.Status)
}

func TestWarehouseService_ProcessEquipmentReturn(t *testing.T) {
	db, ws := setupWarehouseServiceTest(t)

	// Создаем пользователя
	role := models.Role{Name: "installer", DisplayName: "Монтажник"}
	db.Create(&role)

	user := models.User{
		Username:  "installer",
		Email:     "installer@example.com",
		FirstName: "Иван",
		LastName:  "Монтажник",
		RoleID:    role.ID,
	}
	db.Create(&user)

	// Создаем объект
	object := models.Object{
		Name:      "Тестовый объект",
		Latitude:  func() *float64 { f := 55.7558; return &f }(),
		Longitude: func() *float64 { f := 37.6176; return &f }(),
		IMEI:      "123456789012345",
	}
	db.Create(&object)

	// Создаем установленное оборудование
	equipment := models.Equipment{
		Type:         "GPS-tracker",
		Model:        "GT06N",
		Brand:        "Concox",
		SerialNumber: "GT06N001",
		Status:       "installed",
		Condition:    "new",
		ObjectID:     &object.ID,
	}
	db.Create(&equipment)

	// Обрабатываем возврат
	err := ws.ProcessEquipmentReturn(equipment.ID, user.ID, "B2-05")
	assert.NoError(t, err)

	// Проверяем, что статус оборудования изменился
	var updatedEquipment models.Equipment
	db.First(&updatedEquipment, equipment.ID)
	assert.Equal(t, "in_stock", updatedEquipment.Status)
	assert.Nil(t, updatedEquipment.ObjectID)
	assert.Equal(t, "B2-05", updatedEquipment.WarehouseLocation)

	// Проверяем, что создана операция
	var operation models.WarehouseOperation
	err = db.Where("equipment_id = ? AND type = 'receive'", equipment.ID).First(&operation).Error
	assert.NoError(t, err)
	assert.Equal(t, "completed", operation.Status)
}

func TestWarehouseService_GenerateQRCode(t *testing.T) {
	db, ws := setupWarehouseServiceTest(t)

	// Создаем оборудование без QR кода
	equipment := models.Equipment{
		Type:         "GPS-tracker",
		Model:        "GT06N",
		Brand:        "Concox",
		SerialNumber: "GT06N001",
		Status:       "in_stock",
	}
	db.Create(&equipment)

	// Генерируем QR код
	qrCode, err := ws.GenerateQRCode(equipment.ID)
	assert.NoError(t, err)
	assert.NotEmpty(t, qrCode)
	assert.Contains(t, qrCode, "EQ-")
	assert.Contains(t, qrCode, equipment.SerialNumber)

	// Проверяем, что QR код сохранен
	var updatedEquipment models.Equipment
	db.First(&updatedEquipment, equipment.ID)
	assert.Equal(t, qrCode, updatedEquipment.QRCode)

	// Повторный вызов должен вернуть тот же QR код
	qrCode2, err := ws.GenerateQRCode(equipment.ID)
	assert.NoError(t, err)
	assert.Equal(t, qrCode, qrCode2)
}

func TestWarehouseService_ReserveEquipment(t *testing.T) {
	db, ws := setupWarehouseServiceTest(t)

	// Создаем пользователя
	role := models.Role{Name: "installer", DisplayName: "Монтажник"}
	db.Create(&role)

	user := models.User{
		Username:  "installer",
		Email:     "installer@example.com",
		FirstName: "Иван",
		LastName:  "Монтажник",
		RoleID:    role.ID,
	}
	db.Create(&user)

	// Создаем доступное оборудование
	equipment := models.Equipment{
		Type:         "GPS-tracker",
		Model:        "GT06N",
		Brand:        "Concox",
		SerialNumber: "GT06N001",
		Status:       "in_stock",
		Condition:    "new",
	}
	db.Create(&equipment)

	// Резервируем оборудование
	err := ws.ReserveEquipment(equipment.ID, user.ID, "Резерв для монтажа завтра")
	assert.NoError(t, err)

	// Проверяем, что статус изменился
	var updatedEquipment models.Equipment
	db.First(&updatedEquipment, equipment.ID)
	assert.Equal(t, "reserved", updatedEquipment.Status)

	// Проверяем, что создана операция
	var operation models.WarehouseOperation
	err = db.Where("equipment_id = ? AND type = 'reserve'", equipment.ID).First(&operation).Error
	assert.NoError(t, err)
	assert.Equal(t, "completed", operation.Status)

	// Пытаемся зарезервировать уже зарезервированное оборудование
	err = ws.ReserveEquipment(equipment.ID, user.ID, "Повторное резервирование")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "недоступно для резервирования")
}

func TestWarehouseService_UnreserveEquipment(t *testing.T) {
	db, ws := setupWarehouseServiceTest(t)

	// Создаем пользователя
	role := models.Role{Name: "installer", DisplayName: "Монтажник"}
	db.Create(&role)

	user := models.User{
		Username:  "installer",
		Email:     "installer@example.com",
		FirstName: "Иван",
		LastName:  "Монтажник",
		RoleID:    role.ID,
	}
	db.Create(&user)

	// Создаем зарезервированное оборудование
	equipment := models.Equipment{
		Type:         "GPS-tracker",
		Model:        "GT06N",
		Brand:        "Concox",
		SerialNumber: "GT06N001",
		Status:       "reserved",
		Condition:    "new",
	}
	db.Create(&equipment)

	// Снимаем резервирование
	err := ws.UnreserveEquipment(equipment.ID, user.ID)
	assert.NoError(t, err)

	// Проверяем, что статус изменился
	var updatedEquipment models.Equipment
	db.First(&updatedEquipment, equipment.ID)
	assert.Equal(t, "in_stock", updatedEquipment.Status)

	// Проверяем, что создана операция
	var operation models.WarehouseOperation
	err = db.Where("equipment_id = ? AND type = 'unreserve'", equipment.ID).First(&operation).Error
	assert.NoError(t, err)
	assert.Equal(t, "completed", operation.Status)
}

func TestWarehouseService_GetEquipmentHistory(t *testing.T) {
	db, ws := setupWarehouseServiceTest(t)

	// Создаем пользователя
	role := models.Role{Name: "installer", DisplayName: "Монтажник"}
	db.Create(&role)

	user := models.User{
		Username:  "installer",
		Email:     "installer@example.com",
		FirstName: "Иван",
		LastName:  "Монтажник",
		RoleID:    role.ID,
	}
	db.Create(&user)

	// Создаем оборудование
	equipment := models.Equipment{
		Type:         "GPS-tracker",
		Model:        "GT06N",
		Brand:        "Concox",
		SerialNumber: "GT06N001",
		Status:       "in_stock",
		Condition:    "new",
	}
	db.Create(&equipment)

	// Создаем несколько операций
	operations := []models.WarehouseOperation{
		{
			Type:        "receive",
			Description: "Поступление на склад",
			EquipmentID: equipment.ID,
			UserID:      user.ID,
			Status:      "completed",
		},
		{
			Type:        "reserve",
			Description: "Резервирование",
			EquipmentID: equipment.ID,
			UserID:      user.ID,
			Status:      "completed",
		},
		{
			Type:        "issue",
			Description: "Выдача для установки",
			EquipmentID: equipment.ID,
			UserID:      user.ID,
			Status:      "completed",
		},
	}

	for _, op := range operations {
		db.Create(&op)
	}

	// Получаем историю
	history, err := ws.GetEquipmentHistory(equipment.ID)
	assert.NoError(t, err)
	assert.Len(t, history, 3)

	// Проверяем порядок (должен быть от новых к старым)
	assert.Equal(t, "issue", history[0].Type)
	assert.Equal(t, "reserve", history[1].Type)
	assert.Equal(t, "receive", history[2].Type)
}
