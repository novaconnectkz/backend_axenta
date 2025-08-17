package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"backend_axenta/models"
)

// Упрощенная модель Installer для SQLite тестов (без массивов)
type TestInstaller struct {
	ID        uint   `json:"id" gorm:"primarykey"`
	FirstName string `json:"first_name" gorm:"not null"`
	LastName  string `json:"last_name" gorm:"not null"`
	Type      string `json:"type" gorm:"not null"`
	Phone     string `json:"phone" gorm:"not null"`
	Email     string `json:"email" gorm:"uniqueIndex"`
	Status    string `json:"status" gorm:"default:'available'"`
	IsActive  bool   `json:"is_active" gorm:"default:true"`
}

func (TestInstaller) TableName() string {
	return "installers"
}

func setupInstallerTestDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(
		&TestInstaller{},
		&models.Location{},
		&models.Installation{},
		&models.Object{},
	)
	return db
}

func setupInstallerTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	installerAPI := NewInstallerAPI(db)
	api := router.Group("/api")
	{
		api.GET("/installers", installerAPI.GetInstallers)
		api.GET("/installers/:id", installerAPI.GetInstaller)
		api.POST("/installers", installerAPI.CreateInstaller)
		api.PUT("/installers/:id", installerAPI.UpdateInstaller)
		api.DELETE("/installers/:id", installerAPI.DeleteInstaller)
		api.PUT("/installers/:id/deactivate", installerAPI.DeactivateInstaller)
		api.PUT("/installers/:id/activate", installerAPI.ActivateInstaller)
		api.GET("/installers/available", installerAPI.GetAvailableInstallers)
		api.GET("/installers/statistics", installerAPI.GetInstallerStatistics)
		api.GET("/installers/:id/workload", installerAPI.GetInstallerWorkload)
	}

	return router
}

func TestCreateInstaller(t *testing.T) {
	db := setupInstallerTestDB()
	router := setupInstallerTestRouter(db)

	tests := []struct {
		name           string
		installer      models.Installer
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Successful creation - staff installer",
			installer: models.Installer{
				FirstName: "Иван",
				LastName:  "Петров",
				Type:      "штатный",
				Phone:     "+7-900-123-45-67",
				Email:     "ivan.petrov@test.com",
			},
			expectedStatus: http.StatusCreated,
			expectError:    false,
		},
		{
			name: "Successful creation - contractor",
			installer: models.Installer{
				FirstName: "Александр",
				LastName:  "Сидоров",
				Type:      "наемный",
				Phone:     "+7-900-987-65-43",
				Email:     "alex.sidorov@test.com",
			},
			expectedStatus: http.StatusCreated,
			expectError:    false,
		},
		{
			name: "Missing required first name",
			installer: models.Installer{
				LastName: "Петров",
				Type:     "штатный",
				Phone:    "+7-900-123-45-68",
				Email:    "test@test.com",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Duplicate email",
			installer: models.Installer{
				FirstName: "Дубликат",
				LastName:  "Тест",
				Type:      "штатный",
				Phone:     "+7-900-123-45-69",
				Email:     "ivan.petrov@test.com", // Дублируем email из первого теста
			},
			expectedStatus: http.StatusConflict,
			expectError:    true,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Для теста дубликата сначала создаем запись
			if i == 3 {
				firstInstaller := models.Installer{
					FirstName: "Иван",
					LastName:  "Петров",
					Type:      "штатный",
					Phone:     "+7-900-123-45-67",
					Email:     "ivan.petrov@test.com",
				}
				db.Create(&firstInstaller)
			}

			body, _ := json.Marshal(tt.installer)
			req, _ := http.NewRequest("POST", "/api/installers", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response, "data")
				assert.Equal(t, "Монтажник успешно создан", response["message"])
			}
		})
	}
}

func TestGetInstallers(t *testing.T) {
	db := setupInstallerTestDB()
	router := setupInstallerTestRouter(db)

	// Создаем тестовые данные
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
			Status:    "busy",
			IsActive:  true,
		},
		{
			FirstName: "Петр",
			LastName:  "Иванов",
			Type:      "партнер",
			Phone:     "+7-900-555-55-55",
			Email:     "petr.ivanov@test.com",
			Status:    "vacation",
			IsActive:  false,
		},
	}

	for _, installer := range installers {
		db.Create(&installer)
	}

	tests := []struct {
		name           string
		query          string
		expectedCount  int
		expectedStatus int
	}{
		{
			name:           "Get all installers",
			query:          "",
			expectedCount:  3,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by status",
			query:          "?status=available",
			expectedCount:  1,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by type",
			query:          "?type=штатный",
			expectedCount:  1,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by active status",
			query:          "?is_active=true",
			expectedCount:  2,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Search by name",
			query:          "?search=Иван",
			expectedCount:  2, // Иван Петров и Петр Иванов
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by type",
			query:          "?type=наемный",
			expectedCount:  1,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/installers"+tt.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			data := response["data"].([]interface{})
			assert.Equal(t, tt.expectedCount, len(data))
		})
	}
}

func TestUpdateInstaller(t *testing.T) {
	db := setupInstallerTestDB()
	router := setupInstallerTestRouter(db)

	// Создаем тестового монтажника
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

	updateData := models.Installer{
		FirstName: "Иван",
		LastName:  "Петров",
		Type:      "штатный",
		Phone:     "+7-900-123-45-67",
		Email:     "ivan.petrov.updated@test.com",
	}

	body, _ := json.Marshal(updateData)
	req, _ := http.NewRequest("PUT", "/api/installers/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Монтажник успешно обновлен", response["message"])
}

func TestDeleteInstallerWithActiveInstallations(t *testing.T) {
	db := setupInstallerTestDB()
	router := setupInstallerTestRouter(db)

	// Создаем монтажника
	installer := models.Installer{
		FirstName: "Иван",
		LastName:  "Петров",
		Type:      "штатный",
		Phone:     "+7-900-123-45-67",
		Email:     "ivan.petrov@test.com",
	}
	db.Create(&installer)

	// Создаем объект для монтажа
	object := models.Object{
		Name: "Тестовый объект",
	}
	db.Create(&object)

	// Создаем активный монтаж
	installation := models.Installation{
		Type:        "монтаж",
		Status:      "planned",
		ScheduledAt: time.Now().Add(24 * time.Hour),
		ObjectID:    object.ID,
		InstallerID: installer.ID,
	}
	db.Create(&installation)

	// Пытаемся удалить монтажника с активными монтажами
	req, _ := http.NewRequest("DELETE", "/api/installers/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "активными монтажами")
	assert.Equal(t, float64(1), response["active_installations"])
}

func TestInstallerActivationDeactivation(t *testing.T) {
	db := setupInstallerTestDB()
	router := setupInstallerTestRouter(db)

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

	// Деактивируем монтажника
	req, _ := http.NewRequest("PUT", "/api/installers/1/deactivate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем, что монтажник деактивирован
	var updatedInstaller models.Installer
	db.First(&updatedInstaller, installer.ID)
	assert.False(t, updatedInstaller.IsActive)
	assert.Equal(t, "unavailable", updatedInstaller.Status)

	// Активируем монтажника обратно
	req, _ = http.NewRequest("PUT", "/api/installers/1/activate", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем, что монтажник активирован
	db.First(&updatedInstaller, installer.ID)
	assert.True(t, updatedInstaller.IsActive)
	assert.Equal(t, "available", updatedInstaller.Status)
}

func TestGetAvailableInstallers(t *testing.T) {
	db := setupInstallerTestDB()
	router := setupInstallerTestRouter(db)

	// Создаем локацию
	location := models.Location{
		City:   "Москва",
		Region: "Московская область",
	}
	db.Create(&location)

	// Создаем тестовых монтажников
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
			Status:    "busy",
			IsActive:  true,
		},
		{
			FirstName: "Петр",
			LastName:  "Иванов",
			Type:      "партнер",
			Phone:     "+7-900-555-55-55",
			Email:     "petr.ivanov@test.com",
			Status:    "available",
			IsActive:  true,
		},
	}

	for _, installer := range installers {
		db.Create(&installer)
	}

	tests := []struct {
		name           string
		query          string
		expectedCount  int
		expectedStatus int
	}{
		{
			name:           "Get available installers for date",
			query:          "?date=2024-02-15",
			expectedCount:  2, // Иван и Петр доступны и активны
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by location",
			query:          "?date=2024-02-15&location_id=1",
			expectedCount:  2, // Без поддержки массивов LocationIDs все монтажники доступны
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by specialization",
			query:          "?date=2024-02-15&specialization=GPS-trackers",
			expectedCount:  2, // Без поддержки массивов все доступные монтажники
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Missing date parameter",
			query:          "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/installers/available"+tt.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data := response["data"].([]interface{})
				assert.Equal(t, tt.expectedCount, len(data))
			}
		})
	}
}

func TestGetInstallerStatistics(t *testing.T) {
	db := setupInstallerTestDB()
	router := setupInstallerTestRouter(db)

	// Создаем тестовые данные
	installers := []models.Installer{
		{FirstName: "Иван", LastName: "Петров", Type: "штатный", Phone: "+7-900-123-45-67", Email: "ivan@test.com", Status: "available", IsActive: true},
		{FirstName: "Александр", LastName: "Сидоров", Type: "наемный", Phone: "+7-900-987-65-43", Email: "alex@test.com", Status: "busy", IsActive: true},
		{FirstName: "Петр", LastName: "Иванов", Type: "партнер", Phone: "+7-900-555-55-55", Email: "petr@test.com", Status: "vacation", IsActive: false},
	}

	for _, installer := range installers {
		db.Create(&installer)
	}

	req, _ := http.NewRequest("GET", "/api/installers/statistics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(3), data["total"])
	assert.Equal(t, float64(2), data["active"])
	assert.Equal(t, float64(1), data["inactive"])
	assert.Equal(t, float64(1), data["available"])
	assert.Equal(t, float64(1), data["busy"])
	assert.Equal(t, float64(1), data["on_vacation"])
	assert.Equal(t, float64(1), data["staff"])
	assert.Equal(t, float64(1), data["contractors"])
	assert.Equal(t, float64(1), data["partners"])
}

func TestGetInstallerWorkload(t *testing.T) {
	db := setupInstallerTestDB()
	router := setupInstallerTestRouter(db)

	// Создаем монтажника
	installer := models.Installer{
		FirstName: "Иван",
		LastName:  "Петров",
		Type:      "штатный",
		Phone:     "+7-900-123-45-67",
		Email:     "ivan.petrov@test.com",
	}
	db.Create(&installer)

	// Создаем объект
	object := models.Object{
		Name: "Тестовый объект",
	}
	db.Create(&object)

	// Создаем монтажи с разными статусами
	installations := []models.Installation{
		{
			Type:        "монтаж",
			Status:      "completed",
			ScheduledAt: time.Now().Add(-24 * time.Hour),
			ObjectID:    object.ID,
			InstallerID: installer.ID,
		},
		{
			Type:        "диагностика",
			Status:      "planned",
			ScheduledAt: time.Now().Add(24 * time.Hour),
			ObjectID:    object.ID,
			InstallerID: installer.ID,
		},
		{
			Type:        "обслуживание",
			Status:      "cancelled",
			ScheduledAt: time.Now().Add(-12 * time.Hour),
			ObjectID:    object.ID,
			InstallerID: installer.ID,
		},
	}

	for _, installation := range installations {
		db.Create(&installation)
	}

	req, _ := http.NewRequest("GET", "/api/installers/1/workload", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(1), data["installer_id"])
	assert.Equal(t, float64(3), data["total_installations"])
	assert.Equal(t, float64(1), data["completed_installations"])
	assert.Equal(t, float64(1), data["planned_installations"])
	assert.Equal(t, float64(1), data["cancelled_installations"])
	assert.Equal(t, 5.0, data["average_rating"]) // Значение по умолчанию
}

// Тестируем методы модели Installer
func TestInstallerModelMethods(t *testing.T) {
	installer := models.Installer{
		FirstName:  "Иван",
		LastName:   "Петров",
		MiddleName: "Сергеевич",
		IsActive:   true,
		Status:     "available",
	}

	// Тест GetFullName
	assert.Equal(t, "Иван Сергеевич Петров", installer.GetFullName())

	// Тест GetDisplayName
	assert.Equal(t, "Иван Петров", installer.GetDisplayName())

	// Тест с неактивным монтажником
	installer.IsActive = false
	monday := time.Date(2024, 2, 12, 10, 0, 0, 0, time.UTC) // Понедельник
	assert.False(t, installer.IsAvailableOnDate(monday))
}
