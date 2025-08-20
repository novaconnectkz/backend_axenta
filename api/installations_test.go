package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"backend_axenta/models"
)

// Настройка тестовой базы данных для тестов монтажей
func setupInstallationTestDB() *gorm.DB {
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

// Создание тестовых данных для монтажей
func createInstallationTestData(db *gorm.DB) (models.Object, models.Installer, models.Location) {
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
		WorkingDays:           []int{1, 2, 3, 4, 5},
		WorkingHoursStart:     "09:00",
		WorkingHoursEnd:       "18:00",
		LocationIDs:           []uint{location.ID},
	}
	db.Create(&installer)

	return object, installer, location
}

func TestCreateInstallation(t *testing.T) {
	// Настройка
	db := setupInstallationTestDB()
	object, installer, _ := createInstallationTestData(db)

	api := NewInstallationAPI(db)
	router := gin.New()
	router.POST("/installations", api.CreateInstallation)

	// Тестовые данные для создания монтажа
	installation := models.Installation{
		Type:              "монтаж",
		ObjectID:          object.ID,
		InstallerID:       installer.ID,
		ScheduledAt:       time.Now().Add(24 * time.Hour), // завтра
		EstimatedDuration: 120,
		Description:       "Тестовый монтаж",
		Address:           "Тестовый адрес",
		ClientContact:     "+7900654321",
		Priority:          "normal",
	}

	jsonData, _ := json.Marshal(installation)
	req, _ := http.NewRequest("POST", "/installations", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	// Выполнение запроса
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Проверка результата
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	assert.Equal(t, "Монтаж успешно создан", response["message"])
	assert.NotNil(t, response["data"])

	// Проверяем, что монтаж сохранен в БД
	var savedInstallation models.Installation
	db.First(&savedInstallation)
	assert.Equal(t, "монтаж", savedInstallation.Type)
	assert.Equal(t, "planned", savedInstallation.Status)
}

func TestCreateInstallationWithConflict(t *testing.T) {
	// Настройка
	db := setupInstallationTestDB()
	object, installer, _ := createInstallationTestData(db)

	// Создаем существующий монтаж в то же время
	existingInstallation := models.Installation{
		Type:              "диагностика",
		ObjectID:          object.ID,
		InstallerID:       installer.ID,
		ScheduledAt:       time.Now().Add(24 * time.Hour),
		EstimatedDuration: 60,
		Status:            "planned",
	}
	db.Create(&existingInstallation)

	api := NewInstallationAPI(db)
	router := gin.New()
	router.POST("/installations", api.CreateInstallation)

	// Пытаемся создать конфликтующий монтаж
	newInstallation := models.Installation{
		Type:              "монтаж",
		ObjectID:          object.ID,
		InstallerID:       installer.ID,
		ScheduledAt:       time.Now().Add(24 * time.Hour), // то же время
		EstimatedDuration: 120,
	}

	jsonData, _ := json.Marshal(newInstallation)
	req, _ := http.NewRequest("POST", "/installations", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	// Выполнение запроса
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Проверка результата - должен быть конфликт
	assert.Equal(t, http.StatusConflict, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	assert.Contains(t, response["error"], "уже есть работы в это время")
}

func TestGetInstallations(t *testing.T) {
	// Настройка
	db := setupInstallationTestDB()
	object, installer, _ := createInstallationTestData(db)

	// Создаем несколько монтажей
	installations := []models.Installation{
		{
			Type:        "монтаж",
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			ScheduledAt: time.Now().Add(24 * time.Hour),
			Status:      "planned",
		},
		{
			Type:        "диагностика",
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			ScheduledAt: time.Now().Add(48 * time.Hour),
			Status:      "planned",
		},
	}

	for _, installation := range installations {
		db.Create(&installation)
	}

	api := NewInstallationAPI(db)
	router := gin.New()
	router.GET("/installations", api.GetInstallations)

	// Выполнение запроса
	req, _ := http.NewRequest("GET", "/installations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Проверка результата
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	data := response["data"].([]interface{})
	assert.Len(t, data, 2)

	pagination := response["pagination"].(map[string]interface{})
	assert.Equal(t, float64(2), pagination["total"])
}

func TestGetInstallationsWithFilters(t *testing.T) {
	// Настройка
	db := setupInstallationTestDB()
	object, installer, _ := createInstallationTestData(db)

	// Создаем монтажи с разными статусами
	installations := []models.Installation{
		{
			Type:        "монтаж",
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			ScheduledAt: time.Now().Add(24 * time.Hour),
			Status:      "planned",
		},
		{
			Type:        "диагностика",
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			ScheduledAt: time.Now().Add(48 * time.Hour),
			Status:      "completed",
		},
	}

	for _, installation := range installations {
		db.Create(&installation)
	}

	api := NewInstallationAPI(db)
	router := gin.New()
	router.GET("/installations", api.GetInstallations)

	// Тестируем фильтр по статусу
	req, _ := http.NewRequest("GET", "/installations?status=planned", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	data := response["data"].([]interface{})
	assert.Len(t, data, 1) // Только один запланированный монтаж
}

func TestStartInstallation(t *testing.T) {
	// Настройка
	db := setupInstallationTestDB()
	object, installer, _ := createInstallationTestData(db)

	// Создаем запланированный монтаж
	installation := models.Installation{
		Type:        "монтаж",
		ObjectID:    object.ID,
		InstallerID: installer.ID,
		ScheduledAt: time.Now().Add(-1 * time.Hour), // час назад
		Status:      "planned",
	}
	db.Create(&installation)

	api := NewInstallationAPI(db)
	router := gin.New()
	router.PUT("/installations/:id/start", api.StartInstallation)

	// Выполнение запроса
	req, _ := http.NewRequest("PUT", "/installations/1/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Проверка результата
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	assert.Equal(t, "Монтаж начат", response["message"])

	// Проверяем изменение статуса в БД
	var updatedInstallation models.Installation
	db.First(&updatedInstallation, installation.ID)
	assert.Equal(t, "in_progress", updatedInstallation.Status)
	assert.NotNil(t, updatedInstallation.StartedAt)
}

func TestCompleteInstallation(t *testing.T) {
	// Настройка
	db := setupInstallationTestDB()
	object, installer, _ := createInstallationTestData(db)

	// Создаем монтаж в процессе выполнения
	startTime := time.Now().Add(-2 * time.Hour)
	installation := models.Installation{
		Type:        "монтаж",
		ObjectID:    object.ID,
		InstallerID: installer.ID,
		ScheduledAt: time.Now().Add(-3 * time.Hour),
		Status:      "in_progress",
		StartedAt:   &startTime,
	}
	db.Create(&installation)

	api := NewInstallationAPI(db)
	router := gin.New()
	router.PUT("/installations/:id/complete", api.CompleteInstallation)

	// Данные для завершения
	completeData := map[string]interface{}{
		"result":          "Монтаж выполнен успешно",
		"notes":           "Все оборудование установлено и настроено",
		"actual_duration": 90,
		"materials_cost":  1500.0,
		"labor_cost":      2000.0,
	}

	jsonData, _ := json.Marshal(completeData)
	req, _ := http.NewRequest("PUT", "/installations/1/complete", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	// Выполнение запроса
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Проверка результата
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	assert.Equal(t, "Монтаж успешно завершен", response["message"])

	// Проверяем изменения в БД
	var updatedInstallation models.Installation
	db.First(&updatedInstallation, installation.ID)
	assert.Equal(t, "completed", updatedInstallation.Status)
	assert.NotNil(t, updatedInstallation.CompletedAt)
	assert.Equal(t, "Монтаж выполнен успешно", updatedInstallation.Result)
	assert.Equal(t, 90, updatedInstallation.ActualDuration)
}

func TestGetInstallerSchedule(t *testing.T) {
	// Настройка
	db := setupInstallationTestDB()
	object, installer, _ := createInstallationTestData(db)

	// Создаем монтажи на разные дни
	tomorrow := time.Now().Add(24 * time.Hour)
	dayAfterTomorrow := time.Now().Add(48 * time.Hour)

	installations := []models.Installation{
		{
			Type:        "монтаж",
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			ScheduledAt: tomorrow,
			Status:      "planned",
		},
		{
			Type:        "диагностика",
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			ScheduledAt: dayAfterTomorrow,
			Status:      "planned",
		},
	}

	for _, installation := range installations {
		db.Create(&installation)
	}

	api := NewInstallationAPI(db)
	router := gin.New()
	router.GET("/installers/:installer_id/schedule", api.GetInstallerSchedule)

	// Выполнение запроса
	dateFrom := time.Now().Format("2006-01-02")
	dateTo := time.Now().AddDate(0, 0, 7).Format("2006-01-02")

	req, _ := http.NewRequest("GET", "/installers/1/schedule?date_from="+dateFrom+"&date_to="+dateTo, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Проверка результата
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	data := response["data"].([]interface{})
	assert.Len(t, data, 2) // Два запланированных монтажа
}

func TestGetInstallationStatistics(t *testing.T) {
	// Настройка
	db := setupInstallationTestDB()
	object, installer, _ := createInstallationTestData(db)

	// Создаем монтажи с разными статусами
	installations := []models.Installation{
		{Type: "монтаж", ObjectID: object.ID, InstallerID: installer.ID, ScheduledAt: time.Now().Add(24 * time.Hour), Status: "planned"},
		{Type: "диагностика", ObjectID: object.ID, InstallerID: installer.ID, ScheduledAt: time.Now().Add(48 * time.Hour), Status: "completed"},
		{Type: "монтаж", ObjectID: object.ID, InstallerID: installer.ID, ScheduledAt: time.Now().Add(-24 * time.Hour), Status: "planned"}, // просроченный
	}

	for _, installation := range installations {
		db.Create(&installation)
	}

	api := NewInstallationAPI(db)
	router := gin.New()
	router.GET("/installations/statistics", api.GetInstallationStatistics)

	// Выполнение запроса
	req, _ := http.NewRequest("GET", "/installations/statistics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Проверка результата
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(3), data["total"])
	assert.Equal(t, float64(2), data["planned"])
	assert.Equal(t, float64(1), data["completed"])
	assert.Equal(t, float64(1), data["overdue"]) // один просроченный
}

// Benchmark тесты для производительности

func BenchmarkCreateInstallation(b *testing.B) {
	db := setupInstallationTestDB()
	object, installer, _ := createInstallationTestData(db)

	for i := 0; i < b.N; i++ {
		installation := models.Installation{
			Type:              "монтаж",
			ObjectID:          object.ID,
			InstallerID:       installer.ID,
			ScheduledAt:       time.Now().Add(time.Duration(i) * time.Hour), // разное время чтобы избежать конфликтов
			EstimatedDuration: 120,
		}

		db.Create(&installation)
	}
}

func BenchmarkGetInstallations(b *testing.B) {
	db := setupInstallationTestDB()
	object, installer, _ := createInstallationTestData(db)

	// Создаем много монтажей для тестирования производительности
	for i := 0; i < 1000; i++ {
		installation := models.Installation{
			Type:        "монтаж",
			ObjectID:    object.ID,
			InstallerID: installer.ID,
			ScheduledAt: time.Now().Add(time.Duration(i) * time.Hour),
			Status:      "planned",
		}
		db.Create(&installation)
	}

	api := NewInstallationAPI(db)
	router := gin.New()
	router.GET("/installations", api.GetInstallations)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "/installations?page=1&limit=20", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
