package services

import (
	"backend_axenta/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestWarehouseService_CheckMaintenanceDue тестирует CheckMaintenanceDue
func TestWarehouseService_CheckMaintenanceDue(t *testing.T) {
	db, ws := setupWarehouseServiceTest(t)

	// Создаем оборудование, требующее обслуживания (более 6 месяцев назад)
	sixMonthsAgo := time.Now().AddDate(0, -7, 0)
	equipment := models.Equipment{
		Type:              "GPS-tracker",
		Model:             "GT06N",
		Brand:             "Concox",
		SerialNumber:      "GT06N001",
		Status:            "in_stock",
		LastMaintenanceAt: &sixMonthsAgo,
	}
	db.Create(&equipment)

	// Запускаем проверку
	err := ws.CheckMaintenanceDue()
	assert.NoError(t, err)

	// Проверяем, что создано уведомление
	var alert models.StockAlert
	err = db.Where("equipment_id = ? AND type = 'maintenance_due'", equipment.ID).First(&alert).Error
	assert.NoError(t, err)
	assert.Equal(t, "active", alert.Status)
	assert.Contains(t, alert.Title, "Требуется обслуживание")
}

// TestWarehouseService_determineSeverity тестирует determineSeverity
func TestWarehouseService_determineSeverity(t *testing.T) {
	_, ws := setupWarehouseServiceTest(t)

	// Тест с нулевым остатком
	severity := ws.determineSeverity(0, 10)
	assert.Equal(t, "critical", severity)

	// Тест с остатком меньше половины минимума
	severity = ws.determineSeverity(3, 10)
	assert.Equal(t, "high", severity)

	// Тест с остатком меньше минимума
	severity = ws.determineSeverity(7, 10)
	assert.Equal(t, "medium", severity)

	// Тест с остатком больше минимума
	severity = ws.determineSeverity(15, 10)
	assert.Equal(t, "low", severity)
}

// TestWarehouseService_RunPeriodicChecks тестирует RunPeriodicChecks
func TestWarehouseService_RunPeriodicChecks(t *testing.T) {
	db, ws := setupWarehouseServiceTest(t)

	// Создаем тестовые данные
	category := models.EquipmentCategory{
		Name:          "Test Category",
		MinStockLevel: 5,
		IsActive:      true,
	}
	db.Create(&category)

	// Создаем оборудование с низким остатком
	for i := 0; i < 2; i++ {
		equipment := models.Equipment{
			Type:         "GPS-tracker",
			SerialNumber: "SN00" + string(rune(i+1)),
			Status:       "in_stock",
			CategoryID:   &category.ID,
		}
		db.Create(&equipment)
	}

	// Запускаем периодические проверки
	ws.RunPeriodicChecks()
	// Не должно паниковать
}
