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

func setupWarehouseTestAPI(t *testing.T) (*gorm.DB, *gin.Engine) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Автомиграция моделей
	err = db.AutoMigrate(
		&models.Equipment{},
		&models.EquipmentCategory{},
		&models.WarehouseOperation{},
		&models.StockAlert{},
		&models.User{},
		&models.Role{},
	)
	assert.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	warehouseAPI := NewWarehouseAPI(db)

	// Настройка маршрутов
	api := router.Group("/api")
	{
		api.POST("/warehouse/operations", warehouseAPI.CreateWarehouseOperation)
		api.GET("/warehouse/operations", warehouseAPI.GetWarehouseOperations)
		api.POST("/equipment/categories", warehouseAPI.CreateEquipmentCategory)
		api.GET("/equipment/categories", warehouseAPI.GetEquipmentCategories)
		api.PUT("/equipment/categories/:id", warehouseAPI.UpdateEquipmentCategory)
		api.DELETE("/equipment/categories/:id", warehouseAPI.DeleteEquipmentCategory)
		api.GET("/warehouse/alerts", warehouseAPI.GetStockAlerts)
		api.POST("/warehouse/alerts", warehouseAPI.CreateStockAlert)
		api.PUT("/warehouse/alerts/:id/acknowledge", warehouseAPI.AcknowledgeStockAlert)
		api.PUT("/warehouse/alerts/:id/resolve", warehouseAPI.ResolveStockAlert)
		api.GET("/warehouse/statistics", warehouseAPI.GetWarehouseStatistics)
		api.POST("/warehouse/transfer", warehouseAPI.TransferEquipment)
	}

	return db, router
}

func TestCreateEquipmentCategory(t *testing.T) {
	db, router := setupWarehouseTestAPI(t)

	category := models.EquipmentCategory{
		Name:          "GPS Trackers",
		Description:   "GPS трекеры для мониторинга",
		Code:          "GPS",
		MinStockLevel: 5,
		IsActive:      true,
	}

	jsonData, _ := json.Marshal(category)
	req, _ := http.NewRequest("POST", "/api/equipment/categories", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Категория успешно создана", response["message"])

	// Проверяем, что категория создана в базе
	var createdCategory models.EquipmentCategory
	err = db.Where("name = ?", category.Name).First(&createdCategory).Error
	assert.NoError(t, err)
	assert.Equal(t, category.Name, createdCategory.Name)
	assert.Equal(t, category.Code, createdCategory.Code)
}

func TestGetEquipmentCategories(t *testing.T) {
	db, router := setupWarehouseTestAPI(t)

	// Создаем тестовые категории
	categories := []models.EquipmentCategory{
		{Name: "GPS Trackers", Code: "GPS", IsActive: true},
		{Name: "Sensors", Code: "SNS", IsActive: true},
		{Name: "Cameras", Code: "CAM", IsActive: false},
	}

	for _, cat := range categories {
		db.Create(&cat)
	}

	t.Run("Get all categories", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/equipment/categories", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		data := response["data"].([]interface{})
		assert.Len(t, data, 3)
	})

	t.Run("Get only active categories", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/equipment/categories?active=true", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		data := response["data"].([]interface{})
		assert.Len(t, data, 2) // Только активные категории
	})
}

func TestCreateWarehouseOperation(t *testing.T) {
	db, router := setupWarehouseTestAPI(t)

	// Создаем пользователя и роль
	role := models.Role{
		Name:        "warehouse_manager",
		DisplayName: "Менеджер склада",
	}
	db.Create(&role)

	user := models.User{
		Username:  "warehouse_user",
		Email:     "warehouse@example.com",
		FirstName: "Иван",
		LastName:  "Складской",
		RoleID:    role.ID,
		IsActive:  true,
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

	operation := models.WarehouseOperation{
		Type:        "receive",
		Description: "Поступление нового оборудования",
		EquipmentID: equipment.ID,
		UserID:      user.ID,
		Quantity:    1,
		ToLocation:  "A1-01",
		Status:      "completed",
	}

	jsonData, _ := json.Marshal(operation)
	req, _ := http.NewRequest("POST", "/api/warehouse/operations", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Операция успешно создана", response["message"])

	// Проверяем, что операция создана в базе
	var createdOperation models.WarehouseOperation
	err = db.Where("equipment_id = ? AND user_id = ?", equipment.ID, user.ID).First(&createdOperation).Error
	assert.NoError(t, err)
	assert.Equal(t, operation.Type, createdOperation.Type)
	assert.Equal(t, operation.Description, createdOperation.Description)
}

func TestCreateStockAlert(t *testing.T) {
	db, router := setupWarehouseTestAPI(t)

	// Создаем категорию
	category := models.EquipmentCategory{
		Name:          "GPS Trackers",
		Code:          "GPS",
		MinStockLevel: 5,
	}
	db.Create(&category)

	alert := models.StockAlert{
		Type:                "low_stock",
		Title:               "Низкий остаток GPS трекеров",
		Description:         "Остаток GPS трекеров ниже минимального уровня",
		Severity:            "high",
		EquipmentCategoryID: &category.ID,
		Status:              "active",
	}

	jsonData, _ := json.Marshal(alert)
	req, _ := http.NewRequest("POST", "/api/warehouse/alerts", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Уведомление успешно создано", response["message"])

	// Проверяем, что уведомление создано в базе
	var createdAlert models.StockAlert
	err = db.Where("title = ?", alert.Title).First(&createdAlert).Error
	assert.NoError(t, err)
	assert.Equal(t, alert.Type, createdAlert.Type)
	assert.Equal(t, alert.Severity, createdAlert.Severity)
}

func TestGetWarehouseStatistics(t *testing.T) {
	db, router := setupWarehouseTestAPI(t)

	// Создаем тестовые данные
	category := models.EquipmentCategory{Name: "GPS Trackers", Code: "GPS"}
	db.Create(&category)

	equipment := []models.Equipment{
		{Type: "GPS-tracker", Model: "GT06N", Brand: "Concox", SerialNumber: "001", Status: "in_stock", CategoryID: &category.ID},
		{Type: "GPS-tracker", Model: "GT06N", Brand: "Concox", SerialNumber: "002", Status: "installed", CategoryID: &category.ID},
		{Type: "GPS-tracker", Model: "GT06N", Brand: "Concox", SerialNumber: "003", Status: "maintenance", CategoryID: &category.ID},
	}

	for _, eq := range equipment {
		db.Create(&eq)
	}

	req, _ := http.NewRequest("GET", "/api/warehouse/statistics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(3), data["total_equipment"])
	assert.Equal(t, float64(1), data["in_stock"])
	assert.Equal(t, float64(1), data["installed"])
	assert.Equal(t, float64(1), data["maintenance"])
}
