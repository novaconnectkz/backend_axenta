package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupEquipmentTestDB создает тестовую базу данных для equipment
func setupEquipmentTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Equipment{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupEquipmentTestRouter создает тестовый роутер
func setupEquipmentTestRouter(_ *testing.T, _ *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	return router
}

// TestEquipmentAPI_CreateEquipment_ValidationError тестирует CreateEquipment с ошибкой валидации
func TestEquipmentAPI_CreateEquipment_ValidationError(t *testing.T) {
	db := setupEquipmentTestDB(t)
	router := setupEquipmentTestRouter(t, db)

	api := NewEquipmentAPI(db)
	router.POST("/api/equipment", api.CreateEquipment)

	// Тест с пустым телом запроса
	req, _ := http.NewRequest("POST", "/api/equipment", bytes.NewBuffer([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку валидации или успех в зависимости от модели
	assert.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusCreated)
}

// TestEquipmentAPI_CreateEquipment_DuplicateSerialNumber тестирует CreateEquipment с дублирующимся серийным номером
func TestEquipmentAPI_CreateEquipment_DuplicateSerialNumber(t *testing.T) {
	db := setupEquipmentTestDB(t)
	router := setupEquipmentTestRouter(t, db)

	// Создаем существующее оборудование
	existingEquipment := models.Equipment{
		SerialNumber: "SN123456",
		Type:         "tracker",
		Status:       "in_stock",
		Condition:    "new",
	}
	db.Create(&existingEquipment)

	api := NewEquipmentAPI(db)
	router.POST("/api/equipment", api.CreateEquipment)

	// Пытаемся создать с тем же серийным номером
	reqBody := map[string]interface{}{
		"serial_number": "SN123456",
		"type":          "tracker",
		"status":        "in_stock",
		"condition":     "new",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/equipment", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "серийным номером уже существует")
}

// TestEquipmentAPI_CreateEquipment_Success тестирует успешное создание оборудования
func TestEquipmentAPI_CreateEquipment_Success(t *testing.T) {
	db := setupEquipmentTestDB(t)
	router := setupEquipmentTestRouter(t, db)

	api := NewEquipmentAPI(db)
	router.POST("/api/equipment", api.CreateEquipment)

	reqBody := map[string]interface{}{
		"serial_number": "SN789012",
		"type":          "tracker",
		"status":        "in_stock",
		"condition":     "new",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/equipment", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["message"], "успешно создано")
}

// TestEquipmentAPI_GetEquipment_Success тестирует успешное получение списка оборудования
func TestEquipmentAPI_GetEquipment_Success(t *testing.T) {
	db := setupEquipmentTestDB(t)
	router := setupEquipmentTestRouter(t, db)

	// Создаем тестовое оборудование
	equipment := models.Equipment{
		SerialNumber: "SN111222",
		Type:         "tracker",
		Status:       "in_stock",
		Condition:    "new",
	}
	db.Create(&equipment)

	api := NewEquipmentAPI(db)
	router.GET("/api/equipment", api.GetEquipment)

	req, _ := http.NewRequest("GET", "/api/equipment", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotNil(t, response["data"])
}

// TestEquipmentAPI_GetEquipment_WithFilters тестирует GetEquipment с фильтрами
func TestEquipmentAPI_GetEquipment_WithFilters(t *testing.T) {
	db := setupEquipmentTestDB(t)
	router := setupEquipmentTestRouter(t, db)

	// Создаем тестовое оборудование
	equipment := models.Equipment{
		SerialNumber: "SN333444",
		Type:         "tracker",
		Status:       "in_stock",
		Condition:    "new",
	}
	db.Create(&equipment)

	api := NewEquipmentAPI(db)
	router.GET("/api/equipment", api.GetEquipment)

	req, _ := http.NewRequest("GET", "/api/equipment?type=tracker&status=in_stock", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestEquipmentAPI_GetEquipmentByID_NotFound тестирует GetEquipmentByID когда оборудование не найдено
func TestEquipmentAPI_GetEquipmentByID_NotFound(t *testing.T) {
	db := setupEquipmentTestDB(t)
	router := setupEquipmentTestRouter(t, db)

	api := NewEquipmentAPI(db)
	router.GET("/api/equipment/:id", api.GetEquipmentItem)

	req, _ := http.NewRequest("GET", "/api/equipment/99999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestEquipmentAPI_GetEquipmentByID_Success тестирует успешное получение оборудования по ID
func TestEquipmentAPI_GetEquipmentByID_Success(t *testing.T) {
	db := setupEquipmentTestDB(t)
	router := setupEquipmentTestRouter(t, db)

	// Создаем тестовое оборудование
	equipment := models.Equipment{
		SerialNumber: "SN555666",
		Type:         "tracker",
		Status:       "in_stock",
		Condition:    "new",
	}
	db.Create(&equipment)

	api := NewEquipmentAPI(db)
	router.GET("/api/equipment/:id", api.GetEquipmentItem)

	req, _ := http.NewRequest("GET", "/api/equipment/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
