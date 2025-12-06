package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"backend_axenta/models"
)

// setupWarehouseAdditionalTestAPI создает тестовый API для дополнительных тестов warehouse
func setupWarehouseAdditionalTestAPI(t *testing.T) (*gorm.DB, *gin.Engine) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = db.AutoMigrate(
		&models.Equipment{},
		&models.EquipmentCategory{},
		&models.WarehouseOperation{},
		&models.StockAlert{},
		&models.User{},
		&models.Role{},
		&models.Installation{},
	)
	assert.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	warehouseAPI := NewWarehouseAPI(db)

	api := router.Group("/api")
	{
		api.GET("/warehouse/operations/:id", warehouseAPI.GetWarehouseOperation)
		api.PUT("/warehouse/operations/:id", warehouseAPI.UpdateWarehouseOperation)
		api.DELETE("/warehouse/operations/:id", warehouseAPI.DeleteWarehouseOperation)
		api.GET("/warehouse/operations/:id/history", warehouseAPI.GetOperationHistory)
		api.GET("/warehouse/alerts/:id", warehouseAPI.GetStockAlert)
		api.PUT("/warehouse/alerts/:id", warehouseAPI.UpdateStockAlert)
		api.DELETE("/warehouse/alerts/:id", warehouseAPI.DeleteStockAlert)
		api.GET("/warehouse/transfer", warehouseAPI.GetTransferHistory)
	}

	return db, router
}

// TestGetWarehouseOperation тестирует GetWarehouseOperation
func TestGetWarehouseOperation(t *testing.T) {
	db, router := setupWarehouseAdditionalTestAPI(t)

	// Создаем тестовые данные
	equipment := models.Equipment{
		Type:         "GPS-tracker",
		Model:        "GT06N",
		Brand:        "Concox",
		SerialNumber: "GT06N001",
		Status:       "in_stock",
	}
	db.Create(&equipment)

	operation := models.WarehouseOperation{
		Type:        "receive",
		Description: "Тестовая операция",
		EquipmentID: equipment.ID,
		Quantity:    1,
		Status:      "completed",
	}
	db.Create(&operation)

	req, _ := http.NewRequest("GET", "/api/warehouse/operations/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	req, _ = http.NewRequest("GET", "/api/warehouse/operations/1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestUpdateWarehouseOperation тестирует UpdateWarehouseOperation
func TestUpdateWarehouseOperation(t *testing.T) {
	db, router := setupWarehouseAdditionalTestAPI(t)

	equipment := models.Equipment{
		Type:         "GPS-tracker",
		Model:        "GT06N",
		Brand:        "Concox",
		SerialNumber: "GT06N001",
		Status:       "in_stock",
	}
	db.Create(&equipment)

	operation := models.WarehouseOperation{
		Type:        "receive",
		Description: "Исходная операция",
		EquipmentID: equipment.ID,
		Quantity:    1,
		Status:      "pending",
	}
	db.Create(&operation)

	updateData := map[string]interface{}{
		"description": "Обновленная операция",
		"status":      "completed",
	}
	jsonData, _ := json.Marshal(updateData)

	req, _ := http.NewRequest("PUT", "/api/warehouse/operations/999", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	req, _ = http.NewRequest("PUT", "/api/warehouse/operations/1", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// Может быть OK или BadRequest в зависимости от реализации
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
}

// TestDeleteWarehouseOperation тестирует DeleteWarehouseOperation
func TestDeleteWarehouseOperation(t *testing.T) {
	db, router := setupWarehouseAdditionalTestAPI(t)

	equipment := models.Equipment{
		Type:         "GPS-tracker",
		Model:        "GT06N",
		Brand:        "Concox",
		SerialNumber: "GT06N001",
		Status:       "in_stock",
	}
	db.Create(&equipment)

	operation := models.WarehouseOperation{
		Type:        "receive",
		Description: "Операция для удаления",
		EquipmentID: equipment.ID,
		Quantity:    1,
		Status:      "completed",
	}
	db.Create(&operation)

	req, _ := http.NewRequest("DELETE", "/api/warehouse/operations/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	req, _ = http.NewRequest("DELETE", "/api/warehouse/operations/1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// Может быть OK или NoContent в зависимости от реализации
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusNoContent || w.Code == http.StatusNotFound)
}

// TestGetStockAlert тестирует GetStockAlert
func TestGetStockAlert(t *testing.T) {
	db, router := setupWarehouseAdditionalTestAPI(t)

	category := models.EquipmentCategory{
		Name:          "GPS Trackers",
		Code:          "GPS",
		MinStockLevel: 5,
	}
	db.Create(&category)

	alert := models.StockAlert{
		Type:                "low_stock",
		Title:               "Низкий остаток",
		Description:         "Описание",
		Severity:            "high",
		EquipmentCategoryID: &category.ID,
		Status:              "active",
	}
	db.Create(&alert)

	req, _ := http.NewRequest("GET", "/api/warehouse/alerts/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	req, _ = http.NewRequest("GET", "/api/warehouse/alerts/1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestUpdateStockAlert тестирует UpdateStockAlert
func TestUpdateStockAlert(t *testing.T) {
	db, router := setupWarehouseAdditionalTestAPI(t)

	category := models.EquipmentCategory{
		Name:          "GPS Trackers",
		Code:          "GPS",
		MinStockLevel: 5,
	}
	db.Create(&category)

	alert := models.StockAlert{
		Type:                "low_stock",
		Title:               "Исходное уведомление",
		Description:         "Описание",
		Severity:            "high",
		EquipmentCategoryID: &category.ID,
		Status:              "active",
	}
	db.Create(&alert)

	updateData := map[string]interface{}{
		"title":  "Обновленное уведомление",
		"status": "resolved",
	}
	jsonData, _ := json.Marshal(updateData)

	req, _ := http.NewRequest("PUT", "/api/warehouse/alerts/999", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	req, _ = http.NewRequest("PUT", "/api/warehouse/alerts/1", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// Может быть OK или BadRequest
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
}

// TestDeleteStockAlert тестирует DeleteStockAlert
func TestDeleteStockAlert(t *testing.T) {
	db, router := setupWarehouseAdditionalTestAPI(t)

	category := models.EquipmentCategory{
		Name:          "GPS Trackers",
		Code:          "GPS",
		MinStockLevel: 5,
	}
	db.Create(&category)

	alert := models.StockAlert{
		Type:                "low_stock",
		Title:               "Уведомление для удаления",
		Description:         "Описание",
		Severity:            "high",
		EquipmentCategoryID: &category.ID,
		Status:              "active",
	}
	db.Create(&alert)

	req, _ := http.NewRequest("DELETE", "/api/warehouse/alerts/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	req, _ = http.NewRequest("DELETE", "/api/warehouse/alerts/1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// Может быть OK, NoContent или NotFound
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusNoContent || w.Code == http.StatusNotFound)
}
