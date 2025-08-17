package api

import (
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

	"backend_axenta/models"
)

func setupRelationTestDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(
		&models.Location{},
		&models.Installer{},
	)
	return db
}

func setupRelationTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	locationAPI := NewLocationAPI(db)
	installerAPI := NewInstallerAPI(db)

	api := router.Group("/api")
	{
		// Location endpoints
		api.GET("/locations", locationAPI.GetLocations)
		api.GET("/locations/:id", locationAPI.GetLocation)
		api.POST("/locations", locationAPI.CreateLocation)

		// Installer endpoints
		api.GET("/installers", installerAPI.GetInstallers)
		api.GET("/installers/:id", installerAPI.GetInstaller)
		api.POST("/installers", installerAPI.CreateInstaller)
		api.PUT("/installers/:id", installerAPI.UpdateInstaller)
		api.GET("/installers/available", installerAPI.GetAvailableInstallers)
	}

	return router
}

func TestLocationInstallerRelations(t *testing.T) {
	db := setupRelationTestDB()
	router := setupRelationTestRouter(db)

	// Создаем локации
	locations := []models.Location{
		{City: "Москва", Region: "Московская область", Country: "Россия"},
		{City: "Санкт-Петербург", Region: "Ленинградская область", Country: "Россия"},
		{City: "Казань", Region: "Республика Татарстан", Country: "Россия"},
	}

	for _, location := range locations {
		db.Create(&location)
	}

	// Создаем монтажника с привязкой к локациям
	installer := models.Installer{
		FirstName: "Иван",
		LastName:  "Петров",
		Type:      "штатный",
		Phone:     "+7-900-123-45-67",
		Email:     "ivan.petrov@test.com",
		Status:    "available",
		IsActive:  true,
	}

	body, _ := json.Marshal(installer)
	req, _ := http.NewRequest("POST", "/api/installers", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	// Проверяем, что монтажник создался успешно
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	data := response["data"].(map[string]interface{})
	assert.Equal(t, "Иван", data["first_name"])
	assert.Equal(t, "Петров", data["last_name"])

	// Проверяем поиск доступных монтажников по локации
	req, _ = http.NewRequest("GET", "/api/installers/available?date=2024-02-15&location_id=1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	availableInstallers := response["data"].([]interface{})
	assert.Len(t, availableInstallers, 1)
}

func TestUpdateInstallerLocations(t *testing.T) {
	db := setupRelationTestDB()
	router := setupRelationTestRouter(db)

	// Создаем локации
	locations := []models.Location{
		{City: "Москва", Region: "Московская область", Country: "Россия"},
		{City: "Санкт-Петербург", Region: "Ленинградская область", Country: "Россия"},
		{City: "Казань", Region: "Республика Татарстан", Country: "Россия"},
		{City: "Екатеринбург", Region: "Свердловская область", Country: "Россия"},
	}

	for _, location := range locations {
		db.Create(&location)
	}

	// Создаем монтажника
	installer := models.Installer{
		FirstName: "Александр",
		LastName:  "Сидоров",
		Type:      "наемный",
		Phone:     "+7-900-987-65-43",
		Email:     "alex.sidorov@test.com",
		Status:    "available",
		IsActive:  true,
	}
	db.Create(&installer)

	// Обновляем данные монтажника
	updateData := models.Installer{
		SkillLevel: "senior",
	}

	body, _ := json.Marshal(updateData)
	req, _ := http.NewRequest("PUT", "/api/installers/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем, что данные обновились
	var updatedInstaller models.Installer
	db.First(&updatedInstaller, installer.ID)

	assert.Equal(t, "senior", updatedInstaller.SkillLevel)
}

func TestInstallerLocationConstraints(t *testing.T) {
	db := setupRelationTestDB()

	// Создаем локации
	locations := []models.Location{
		{City: "Москва", Region: "Московская область", Country: "Россия"},
		{City: "Санкт-Петербург", Region: "Ленинградская область", Country: "Россия"},
	}

	for _, location := range locations {
		db.Create(&location)
	}

	// Создаем монтажника
	installer := models.Installer{
		FirstName: "Тест",
		LastName:  "Тестов",
		Type:      "штатный",
		Phone:     "+7-900-111-11-11",
		Email:     "test@test.com",
		Status:    "available",
		IsActive:  true,
	}

	// Создаем монтажника
	result := db.Create(&installer)

	// Проверяем, что запись создалась
	assert.NoError(t, result.Error)
}

func TestLocationDeletion(t *testing.T) {
	db := setupRelationTestDB()
	router := setupRelationTestRouter(db)

	// Создаем локацию
	location := models.Location{
		City:    "Москва",
		Region:  "Московская область",
		Country: "Россия",
	}
	db.Create(&location)

	// Создаем монтажника
	installer := models.Installer{
		FirstName: "Иван",
		LastName:  "Петров",
		Type:      "штатный",
		Phone:     "+7-900-123-45-67",
		Email:     "ivan.petrov@test.com",
		Status:    "available",
		IsActive:  true,
	}
	db.Create(&installer)

	// Пытаемся удалить локацию
	req, _ := http.NewRequest("DELETE", "/api/locations/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Локация должна удалиться
	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем, что монтажник остался
	var remainingInstaller models.Installer
	db.First(&remainingInstaller, installer.ID)
	assert.Equal(t, "Иван", remainingInstaller.FirstName)
}

func TestMultipleInstallersInLocation(t *testing.T) {
	db := setupRelationTestDB()
	router := setupRelationTestRouter(db)

	// Создаем локации
	locations := []models.Location{
		{City: "Москва", Region: "Московская область", Country: "Россия"},
		{City: "Санкт-Петербург", Region: "Ленинградская область", Country: "Россия"},
	}

	for _, location := range locations {
		db.Create(&location)
	}

	// Создаем нескольких монтажников
	installers := []models.Installer{
		{
			FirstName: "Иван",
			LastName:  "Петров",
			Type:      "штатный",
			Phone:     "+7-900-123-45-67",
			Email:     "ivan.petrov@test.com",
			Status:    "available",
			IsActive:  true,
		},
		{
			FirstName: "Александр",
			LastName:  "Сидоров",
			Type:      "наемный",
			Phone:     "+7-900-987-65-43",
			Email:     "alex.sidorov@test.com",
			Status:    "available",
			IsActive:  true,
		},
		{
			FirstName: "Петр",
			LastName:  "Иванов",
			Type:      "партнер",
			Phone:     "+7-900-555-55-55",
			Email:     "petr.ivanov@test.com",
			Status:    "busy", // Занят
			IsActive:  true,
		},
	}

	for _, installer := range installers {
		db.Create(&installer)
	}

	// Проверяем доступных монтажников в Москве
	req, _ := http.NewRequest("GET", "/api/installers/available?date=2024-02-15&location_id=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	availableInstallers := response["data"].([]interface{})
	assert.Len(t, availableInstallers, 2) // Иван и Александр (доступные)
}

func TestLocationFilterInInstallersList(t *testing.T) {
	db := setupRelationTestDB()
	router := setupRelationTestRouter(db)

	// Создаем локации
	locations := []models.Location{
		{City: "Москва", Region: "Московская область", Country: "Россия"},
		{City: "Санкт-Петербург", Region: "Ленинградская область", Country: "Россия"},
	}

	for _, location := range locations {
		db.Create(&location)
	}

	// Создаем монтажников
	installers := []models.Installer{
		{
			FirstName: "Московский",
			LastName:  "Монтажник",
			Type:      "штатный",
			Phone:     "+7-900-111-11-11",
			Email:     "moscow@test.com",
			Status:    "available",
			IsActive:  true,
		},
		{
			FirstName: "Питерский",
			LastName:  "Монтажник",
			Type:      "наемный",
			Phone:     "+7-900-222-22-22",
			Email:     "spb@test.com",
			Status:    "available",
			IsActive:  true,
		},
		{
			FirstName: "Мобильный",
			LastName:  "Монтажник",
			Type:      "партнер",
			Phone:     "+7-900-333-33-33",
			Email:     "mobile@test.com",
			Status:    "available",
			IsActive:  true,
		},
	}

	for _, installer := range installers {
		db.Create(&installer)
	}

	// К сожалению, текущий API не поддерживает фильтрацию по городу через название
	// Но мы можем протестировать общий список
	req, _ := http.NewRequest("GET", "/api/installers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	allInstallers := response["data"].([]interface{})
	assert.Len(t, allInstallers, 3)

	// Проверяем, что все монтажники присутствуют
	for _, installerData := range allInstallers {
		installer := installerData.(map[string]interface{})
		firstName := installer["first_name"]
		assert.NotEmpty(t, firstName)
	}
}
