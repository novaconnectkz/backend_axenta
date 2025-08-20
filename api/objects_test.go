package api

import (
	"backend_axenta/models"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB создает тестовую базу данных в памяти
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Автомиграция всех моделей
	err = db.AutoMigrate(
		&models.Company{},
		&models.BillingPlan{},
		&models.Contract{},
		&models.ContractAppendix{},
		&models.Object{},
		&models.ObjectTemplate{},
		&models.Location{},
		&models.Equipment{},
		&models.Installation{},
		&models.User{},
		&models.Role{},
	)
	require.NoError(t, err)

	return db
}

// setupTestRouter создает тестовый роутер с middleware
func setupTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Создаем middleware для тестов
	router.Use(func(c *gin.Context) {
		c.Set("tenant_db", db)
		c.Next()
	})

	// Добавляем маршруты для объектов
	api := router.Group("/api")
	{
		api.GET("/objects", GetObjects)
		api.GET("/objects/:id", GetObject)
		api.POST("/objects", CreateObject)
		api.PUT("/objects/:id", UpdateObject)
		api.DELETE("/objects/:id", DeleteObject)
		api.PUT("/objects/:id/schedule-delete", ScheduleObjectDelete)
		api.PUT("/objects/:id/cancel-delete", CancelScheduledDelete)
		api.GET("/objects-trash", GetDeletedObjects)
		api.PUT("/objects/:id/restore", RestoreObject)
		api.DELETE("/objects/:id/permanent", PermanentDeleteObject)

		// Шаблоны объектов
		api.GET("/object-templates", GetObjectTemplates)
		api.GET("/object-templates/:id", GetObjectTemplate)
		api.POST("/object-templates", CreateObjectTemplate)
		api.PUT("/object-templates/:id", UpdateObjectTemplate)
		api.DELETE("/object-templates/:id", DeleteObjectTemplate)
	}

	return router
}

// createTestData создает тестовые данные
func createTestData(t *testing.T, db *gorm.DB) (models.Contract, models.ObjectTemplate, models.Location) {
	// Создаем тарифный план
	billingPlan := models.BillingPlan{
		Name:          "Тестовый план",
		Price:         decimal.NewFromFloat(1000.0),
		Currency:      "RUB",
		BillingPeriod: "monthly",
		IsActive:      true,
	}
	err := db.Create(&billingPlan).Error
	require.NoError(t, err)

	// Создаем договор
	contract := models.Contract{
		Number:       "TEST-001",
		Title:        "Тестовый договор",
		ClientName:   "ООО Тест",
		StartDate:    time.Now(),
		EndDate:      time.Now().AddDate(1, 0, 0),
		TariffPlanID: billingPlan.ID,
		TotalAmount:  decimal.NewFromFloat(12000.0),
		Status:       "active",
		IsActive:     true,
	}
	err = db.Create(&contract).Error
	require.NoError(t, err)

	// Создаем локацию
	location := models.Location{
		City:     "Москва",
		Region:   "Московская область",
		Country:  "Russia",
		Timezone: "Europe/Moscow",
		IsActive: true,
	}
	err = db.Create(&location).Error
	require.NoError(t, err)

	// Создаем шаблон объекта
	template := models.ObjectTemplate{
		Name:        "Тестовый шаблон",
		Description: "Шаблон для тестирования",
		Category:    "vehicle",
		IsActive:    true,
	}
	err = db.Create(&template).Error
	require.NoError(t, err)

	return contract, template, location
}

func TestGetObjects(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	contract, template, location := createTestData(t, db)

	// Создаем тестовый объект
	object := models.Object{
		Name:       "Тестовый объект",
		Type:       "vehicle",
		IMEI:       "123456789012345",
		ContractID: contract.ID,
		LocationID: location.ID,
		TemplateID: &template.ID,
		IsActive:   true,
	}
	err := db.Create(&object).Error
	require.NoError(t, err)

	// Тестируем получение списка объектов
	req, _ := http.NewRequest("GET", "/api/objects", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(1), data["total"])

	items := data["items"].([]interface{})
	assert.Len(t, items, 1)

	item := items[0].(map[string]interface{})
	assert.Equal(t, "Тестовый объект", item["name"])
	assert.Equal(t, "123456789012345", item["imei"])
}

func TestCreateObject(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	contract, template, location := createTestData(t, db)

	// Данные для создания объекта
	objectData := map[string]interface{}{
		"name":        "Новый объект",
		"type":        "vehicle",
		"imei":        "987654321098765",
		"contract_id": contract.ID,
		"location_id": location.ID,
		"template_id": template.ID,
		"description": "Тестовое описание",
	}

	jsonData, _ := json.Marshal(objectData)
	req, _ := http.NewRequest("POST", "/api/objects", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 201, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	data := response["data"].(map[string]interface{})
	assert.Equal(t, "Новый объект", data["name"])
	assert.Equal(t, "987654321098765", data["imei"])
	assert.Equal(t, "vehicle", data["type"])

	// Проверяем, что объект создался в базе
	var createdObject models.Object
	err = db.First(&createdObject, data["id"]).Error
	require.NoError(t, err)
	assert.Equal(t, "Новый объект", createdObject.Name)
}

func TestUpdateObject(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	contract, template, location := createTestData(t, db)

	// Создаем объект для обновления
	object := models.Object{
		Name:       "Исходный объект",
		Type:       "vehicle",
		IMEI:       "111111111111111",
		ContractID: contract.ID,
		LocationID: location.ID,
		TemplateID: &template.ID,
		IsActive:   true,
	}
	err := db.Create(&object).Error
	require.NoError(t, err)

	// Данные для обновления
	updateData := map[string]interface{}{
		"name":        "Обновленный объект",
		"description": "Новое описание",
	}

	jsonData, _ := json.Marshal(updateData)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/objects/%d", object.ID), bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	data := response["data"].(map[string]interface{})
	assert.Equal(t, "Обновленный объект", data["name"])
	assert.Equal(t, "Новое описание", data["description"])
}

func TestScheduleObjectDelete(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	contract, template, location := createTestData(t, db)

	// Создаем объект для планового удаления
	object := models.Object{
		Name:       "Объект для удаления",
		Type:       "vehicle",
		IMEI:       "222222222222222",
		ContractID: contract.ID,
		LocationID: location.ID,
		TemplateID: &template.ID,
		IsActive:   true,
	}
	err := db.Create(&object).Error
	require.NoError(t, err)

	// Планируем удаление на завтра
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	scheduleData := map[string]interface{}{
		"scheduled_delete_at": tomorrow,
	}

	jsonData, _ := json.Marshal(scheduleData)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/objects/%d/schedule-delete", object.ID), bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	data := response["data"].(map[string]interface{})
	assert.Equal(t, "scheduled_delete", data["status"])

	// Проверяем в базе
	var updatedObject models.Object
	err = db.First(&updatedObject, object.ID).Error
	require.NoError(t, err)
	assert.Equal(t, "scheduled_delete", updatedObject.Status)
	assert.NotNil(t, updatedObject.ScheduledDeleteAt)
	assert.False(t, updatedObject.IsActive)
}

func TestObjectTemplatesCRUD(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Тест создания шаблона
	templateData := map[string]interface{}{
		"name":        "Шаблон автомобиля",
		"description": "Шаблон для легковых автомобилей",
		"category":    "vehicle",
		"icon":        "car",
		"color":       "#FF5722",
	}

	jsonData, _ := json.Marshal(templateData)
	req, _ := http.NewRequest("POST", "/api/object-templates", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 201, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	data := response["data"].(map[string]interface{})
	templateID := uint(data["id"].(float64))
	assert.Equal(t, "Шаблон автомобиля", data["name"])
	assert.Equal(t, "vehicle", data["category"])

	// Тест получения списка шаблонов
	req, _ = http.NewRequest("GET", "/api/object-templates", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	listData := response["data"].(map[string]interface{})
	assert.Equal(t, float64(1), listData["total"])

	// Тест получения конкретного шаблона
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/object-templates/%d", templateID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	data = response["data"].(map[string]interface{})
	assert.Equal(t, "Шаблон автомобиля", data["name"])

	// Тест обновления шаблона
	updateData := map[string]interface{}{
		"name":        "Обновленный шаблон автомобиля",
		"description": "Обновленное описание",
	}

	jsonData, _ = json.Marshal(updateData)
	req, _ = http.NewRequest("PUT", fmt.Sprintf("/api/object-templates/%d", templateID), bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	data = response["data"].(map[string]interface{})
	assert.Equal(t, "Обновленный шаблон автомобиля", data["name"])
	assert.Equal(t, "Обновленное описание", data["description"])

	// Тест удаления шаблона
	req, _ = http.NewRequest("DELETE", fmt.Sprintf("/api/object-templates/%d", templateID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
}

func TestTrashBinOperations(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	contract, template, location := createTestData(t, db)

	// Создаем объект
	object := models.Object{
		Name:       "Объект для корзины",
		Type:       "vehicle",
		IMEI:       "333333333333333",
		ContractID: contract.ID,
		LocationID: location.ID,
		TemplateID: &template.ID,
		IsActive:   true,
	}
	err := db.Create(&object).Error
	require.NoError(t, err)

	// Удаляем объект (мягкое удаление)
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/objects/%d", object.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	// Проверяем корзину
	req, _ = http.NewRequest("GET", "/api/objects-trash", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(1), data["total"])

	items := data["items"].([]interface{})
	assert.Len(t, items, 1)
	item := items[0].(map[string]interface{})
	assert.Equal(t, "Объект для корзины", item["name"])

	// Восстанавливаем объект
	req, _ = http.NewRequest("PUT", fmt.Sprintf("/api/objects/%d/restore", object.ID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	// Проверяем, что объект восстановился
	var restoredObject models.Object
	err = db.First(&restoredObject, object.ID).Error
	require.NoError(t, err)
	assert.Equal(t, "active", restoredObject.Status)
	assert.True(t, restoredObject.IsActive)
}
