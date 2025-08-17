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

func setupLocationTestDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(
		&models.Location{},
		&models.Object{},
		&models.Installation{},
	)
	return db
}

func setupLocationTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	locationAPI := NewLocationAPI(db)
	api := router.Group("/api")
	{
		api.GET("/locations", locationAPI.GetLocations)
		api.GET("/locations/:id", locationAPI.GetLocation)
		api.POST("/locations", locationAPI.CreateLocation)
		api.PUT("/locations/:id", locationAPI.UpdateLocation)
		api.DELETE("/locations/:id", locationAPI.DeleteLocation)
		api.PUT("/locations/:id/deactivate", locationAPI.DeactivateLocation)
		api.PUT("/locations/:id/activate", locationAPI.ActivateLocation)
		api.GET("/locations/statistics", locationAPI.GetLocationStatistics)
		api.GET("/locations/by-region", locationAPI.GetLocationsByRegion)
		api.GET("/locations/search", locationAPI.SearchLocations)
	}

	return router
}

func TestCreateLocation(t *testing.T) {
	db := setupLocationTestDB()
	router := setupLocationTestRouter(db)

	tests := []struct {
		name           string
		location       models.Location
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Successful creation",
			location: models.Location{
				City:      "Москва",
				Region:    "Московская область",
				Country:   "Россия",
				Latitude:  func() *float64 { v := 55.7558; return &v }(),
				Longitude: func() *float64 { v := 37.6176; return &v }(),
				Timezone:  "Europe/Moscow",
			},
			expectedStatus: http.StatusCreated,
			expectError:    false,
		},
		{
			name: "Missing required city",
			location: models.Location{
				Region:  "Московская область",
				Country: "Россия",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Duplicate city+region",
			location: models.Location{
				City:    "Москва",
				Region:  "Московская область",
				Country: "Россия",
			},
			expectedStatus: http.StatusConflict,
			expectError:    true,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Для теста дубликата сначала создаем запись
			if i == 2 {
				firstLocation := models.Location{
					City:    "Москва",
					Region:  "Московская область",
					Country: "Россия",
				}
				db.Create(&firstLocation)
			}

			body, _ := json.Marshal(tt.location)
			req, _ := http.NewRequest("POST", "/api/locations", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response, "data")
				assert.Equal(t, "Локация успешно создана", response["message"])
			}
		})
	}
}

func TestGetLocations(t *testing.T) {
	db := setupLocationTestDB()
	router := setupLocationTestRouter(db)

	// Создаем тестовые данные
	locations := []models.Location{
		{City: "Москва", Region: "Московская область", Country: "Россия", IsActive: true},
		{City: "Санкт-Петербург", Region: "Ленинградская область", Country: "Россия", IsActive: true},
		{City: "Казань", Region: "Республика Татарстан", Country: "Россия", IsActive: false},
	}

	for _, location := range locations {
		db.Create(&location)
	}

	tests := []struct {
		name           string
		query          string
		expectedCount  int
		expectedStatus int
	}{
		{
			name:           "Get all locations",
			query:          "",
			expectedCount:  3,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by active status",
			query:          "?is_active=true",
			expectedCount:  2,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Search by city",
			query:          "?search=Москва",
			expectedCount:  1,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by region",
			query:          "?region=Московская",
			expectedCount:  1,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/locations"+tt.query, nil)
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

func TestUpdateLocation(t *testing.T) {
	db := setupLocationTestDB()
	router := setupLocationTestRouter(db)

	// Создаем тестовую локацию
	location := models.Location{
		City:    "Москва",
		Region:  "Московская область",
		Country: "Россия",
	}
	db.Create(&location)

	updateData := models.Location{
		City:      "Москва",
		Region:    "г. Москва",
		Country:   "Россия",
		Latitude:  func() *float64 { v := 55.7558; return &v }(),
		Longitude: func() *float64 { v := 37.6176; return &v }(),
	}

	body, _ := json.Marshal(updateData)
	req, _ := http.NewRequest("PUT", "/api/locations/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Локация успешно обновлена", response["message"])
}

func TestDeleteLocationWithConstraints(t *testing.T) {
	db := setupLocationTestDB()
	router := setupLocationTestRouter(db)

	// Создаем локацию
	location := models.Location{
		City:    "Москва",
		Region:  "Московская область",
		Country: "Россия",
	}
	db.Create(&location)

	// Создаем объект, привязанный к локации
	object := models.Object{
		Name:       "Тестовый объект",
		LocationID: location.ID,
	}
	db.Create(&object)

	// Пытаемся удалить локацию с привязанными объектами
	req, _ := http.NewRequest("DELETE", "/api/locations/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"], "привязаны объекты")
}

func TestLocationActivationDeactivation(t *testing.T) {
	db := setupLocationTestDB()
	router := setupLocationTestRouter(db)

	// Создаем локацию
	location := models.Location{
		City:     "Москва",
		Region:   "Московская область",
		Country:  "Россия",
		IsActive: true,
	}
	db.Create(&location)

	// Деактивируем локацию
	req, _ := http.NewRequest("PUT", "/api/locations/1/deactivate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем, что локация деактивирована
	var updatedLocation models.Location
	db.First(&updatedLocation, location.ID)
	assert.False(t, updatedLocation.IsActive)

	// Активируем локацию обратно
	req, _ = http.NewRequest("PUT", "/api/locations/1/activate", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем, что локация активирована
	db.First(&updatedLocation, location.ID)
	assert.True(t, updatedLocation.IsActive)
}

func TestGetLocationStatistics(t *testing.T) {
	db := setupLocationTestDB()
	router := setupLocationTestRouter(db)

	// Создаем тестовые данные
	locations := []models.Location{
		{City: "Москва", Region: "Московская область", Country: "Россия", IsActive: true, Latitude: func() *float64 { v := 55.7558; return &v }(), Longitude: func() *float64 { v := 37.6176; return &v }()},
		{City: "Санкт-Петербург", Region: "Ленинградская область", Country: "Россия", IsActive: true},
		{City: "Казань", Region: "Республика Татарстан", Country: "Россия", IsActive: false},
	}

	for _, location := range locations {
		db.Create(&location)
	}

	req, _ := http.NewRequest("GET", "/api/locations/statistics", nil)
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
	assert.Equal(t, float64(1), data["with_coordinates"])
	assert.Equal(t, float64(2), data["without_coordinates"])
}

func TestSearchLocations(t *testing.T) {
	db := setupLocationTestDB()
	router := setupLocationTestRouter(db)

	// Создаем тестовые данные
	locations := []models.Location{
		{City: "Москва", Region: "Московская область", Country: "Россия", IsActive: true},
		{City: "Мытищи", Region: "Московская область", Country: "Россия", IsActive: true},
		{City: "Санкт-Петербург", Region: "Ленинградская область", Country: "Россия", IsActive: true},
	}

	for _, location := range locations {
		db.Create(&location)
	}

	tests := []struct {
		name           string
		query          string
		expectedCount  int
		expectedStatus int
	}{
		{
			name:           "Search by city prefix",
			query:          "?q=Мо",
			expectedCount:  1,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Search by region",
			query:          "?q=Московская",
			expectedCount:  2,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "No query parameter",
			query:          "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/locations/search"+tt.query, nil)
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
