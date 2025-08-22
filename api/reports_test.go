package api

import (
	"backend_axenta/models"
	"backend_axenta/services"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupReportsTestDB создает тестовую базу данных
func setupReportsTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Company{},
		&models.User{},
		&models.Role{},
		&models.Object{},
		&models.Contract{},
		&models.Location{},
		&models.Installation{},
		&models.Installer{},
		&models.Equipment{},
		&models.EquipmentCategory{},
		&models.Invoice{},
		&models.Report{},
		&models.ReportTemplate{},
		&models.ReportSchedule{},
		&models.ReportExecution{},
	)
	require.NoError(t, err)

	return db
}

// createReportsTestData создает тестовые данные
func createReportsTestData(t *testing.T, db *gorm.DB) {
	// Создаем роль
	role := models.Role{
		Name:        "Admin",
		DisplayName: "Administrator",
		Description: "Administrator role",
		IsActive:    true,
	}
	require.NoError(t, db.Create(&role).Error)

	// Создаем пользователя
	user := models.User{
		Username:  "testuser",
		Email:     "test@example.com",
		Password:  "hashedpassword",
		FirstName: "Test",
		LastName:  "User",
		IsActive:  true,
		RoleID:    role.ID,
		CompanyID: uuid.New(),
	}
	require.NoError(t, db.Create(&user).Error)

	// Создаем отчеты
	reports := []models.Report{
		{
			Name:        "Test Report 1",
			Type:        models.ReportTypeObjects,
			Status:      models.ReportStatusCompleted,
			Format:      models.ReportFormatCSV,
			CreatedByID: user.ID,
			CompanyID:   1,
			RecordCount: 10,
			FileSize:    1024,
			FilePath:    "/tmp/report1.csv",
		},
		{
			Name:        "Test Report 2",
			Type:        models.ReportTypeUsers,
			Status:      models.ReportStatusPending,
			Format:      models.ReportFormatExcel,
			CreatedByID: user.ID,
			CompanyID:   1,
		},
	}

	for _, report := range reports {
		require.NoError(t, db.Create(&report).Error)
	}

	// Создаем шаблоны отчетов
	templates := []models.ReportTemplate{
		{
			Name:        "Objects Template",
			Type:        models.ReportTypeObjects,
			Config:      `{"columns": ["id", "name", "status"]}`,
			Parameters:  `{"default_status": "active"}`,
			IsActive:    true,
			IsPublic:    true,
			CreatedByID: user.ID,
			CompanyID:   1,
		},
		{
			Name:        "Users Template",
			Type:        models.ReportTypeUsers,
			Config:      `{"columns": ["id", "username", "email"]}`,
			Parameters:  `{"include_inactive": false}`,
			IsActive:    true,
			IsPublic:    false,
			CreatedByID: user.ID,
			CompanyID:   1,
		},
	}

	for _, template := range templates {
		require.NoError(t, db.Create(&template).Error)
	}

	// Создаем расписания
	schedules := []models.ReportSchedule{
		{
			Name:        "Daily Objects Report",
			Type:        models.ScheduleTypeDaily,
			TemplateID:  templates[0].ID,
			TimeOfDay:   "09:00",
			Parameters:  `{"status": "active"}`,
			Format:      models.ReportFormatCSV,
			Recipients:  `["admin@test.com"]`,
			IsActive:    true,
			CreatedByID: user.ID,
			CompanyID:   1,
		},
	}

	for _, schedule := range schedules {
		require.NoError(t, db.Create(&schedule).Error)
	}
}

// setupReportsTestRouter настраивает тестовый роутер
func setupReportsTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки company_id и user_id
	router.Use(func(c *gin.Context) {
		c.Set("company_id", uint(1))
		c.Set("user_id", uint(1))
		c.Next()
	})

	reportService := services.NewReportService(db)
	schedulerService := services.NewReportSchedulerService(db, reportService, nil)
	api := NewReportsAPI(db, reportService, schedulerService)

	v1 := router.Group("/api")
	api.RegisterRoutes(v1)

	return router
}

func TestReportsAPI_GetReports(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	tests := []struct {
		name           string
		query          string
		expectedCount  int
		expectedStatus int
	}{
		{
			name:           "Get all reports",
			query:          "",
			expectedCount:  2,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by type",
			query:          "?type=objects",
			expectedCount:  1,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by status",
			query:          "?status=completed",
			expectedCount:  1,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Pagination",
			query:          "?page=1&limit=1",
			expectedCount:  1,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/reports"+tt.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				reports := response["reports"].([]interface{})
				assert.Equal(t, tt.expectedCount, len(reports))
			}
		})
	}
}

func TestReportsAPI_CreateReport(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	tests := []struct {
		name           string
		payload        CreateReportRequest
		expectedStatus int
	}{
		{
			name: "Valid report creation",
			payload: CreateReportRequest{
				Name:        "New Test Report",
				Description: "Test description",
				Type:        models.ReportTypeObjects,
				Format:      models.ReportFormatCSV,
				Parameters:  map[string]interface{}{"status": "active"},
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "Missing required fields",
			payload: CreateReportRequest{
				Description: "Test description",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonPayload, _ := json.Marshal(tt.payload)
			req, _ := http.NewRequest("POST", "/api/reports", bytes.NewBuffer(jsonPayload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == http.StatusCreated {
				var report models.Report
				err := json.Unmarshal(w.Body.Bytes(), &report)
				require.NoError(t, err)
				assert.Equal(t, tt.payload.Name, report.Name)
				assert.Equal(t, tt.payload.Type, report.Type)
				assert.Equal(t, models.ReportStatusPending, report.Status)
			}
		})
	}
}

func TestReportsAPI_GetReport(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	tests := []struct {
		name           string
		reportID       string
		expectedStatus int
	}{
		{
			name:           "Valid report ID",
			reportID:       "1",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid report ID",
			reportID:       "abc",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Non-existent report",
			reportID:       "999",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/reports/"+tt.reportID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == http.StatusOK {
				var report models.Report
				err := json.Unmarshal(w.Body.Bytes(), &report)
				require.NoError(t, err)
				assert.NotZero(t, report.ID)
			}
		})
	}
}

func TestReportsAPI_UpdateReport(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	payload := CreateReportRequest{
		Name:        "Updated Report Name",
		Description: "Updated description",
		Type:        models.ReportTypeUsers,
		Format:      models.ReportFormatExcel,
	}

	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PUT", "/api/reports/1", bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var report models.Report
	err := json.Unmarshal(w.Body.Bytes(), &report)
	require.NoError(t, err)
	assert.Equal(t, payload.Name, report.Name)
	assert.Equal(t, payload.Description, report.Description)
}

func TestReportsAPI_DeleteReport(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	req, _ := http.NewRequest("DELETE", "/api/reports/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем, что отчет удален
	var report models.Report
	err := db.First(&report, 1).Error
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

func TestReportsAPI_GenerateReport(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	tests := []struct {
		name           string
		reportID       string
		expectedStatus int
	}{
		{
			name:           "Generate pending report",
			reportID:       "2", // Pending report
			expectedStatus: http.StatusAccepted,
		},
		{
			name:           "Generate completed report",
			reportID:       "1",                 // Completed report
			expectedStatus: http.StatusAccepted, // Можно перегенерировать
		},
		{
			name:           "Non-existent report",
			reportID:       "999",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/api/reports/"+tt.reportID+"/generate", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == http.StatusAccepted {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Equal(t, "Report generation started", response["message"])
			}
		})
	}
}

func TestReportsAPI_GetReportStatus(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	req, _ := http.NewRequest("GET", "/api/reports/1/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var status map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &status)
	require.NoError(t, err)

	assert.Equal(t, float64(1), status["id"])
	assert.Equal(t, string(models.ReportStatusCompleted), status["status"])
	assert.Equal(t, float64(10), status["record_count"])
	assert.NotNil(t, status["download_url"])
}

func TestReportsAPI_GetReportTemplates(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	tests := []struct {
		name           string
		query          string
		expectedCount  int
		expectedStatus int
	}{
		{
			name:           "Get all templates",
			query:          "",
			expectedCount:  2, // Один публичный + один созданный пользователем
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Filter by type",
			query:          "?type=objects",
			expectedCount:  1,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/reports/templates"+tt.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == http.StatusOK {
				var templates []models.ReportTemplate
				err := json.Unmarshal(w.Body.Bytes(), &templates)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedCount, len(templates))
			}
		})
	}
}

func TestReportsAPI_CreateReportTemplate(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	payload := CreateReportTemplateRequest{
		Name:        "New Template",
		Description: "Test template",
		Type:        models.ReportTypeBilling,
		Config:      map[string]interface{}{"columns": []string{"id", "amount"}},
		Parameters:  map[string]interface{}{"status": "paid"},
		Headers:     []string{"ID", "Amount"},
		IsPublic:    true,
	}

	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/api/reports/templates", bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var template models.ReportTemplate
	err := json.Unmarshal(w.Body.Bytes(), &template)
	require.NoError(t, err)
	assert.Equal(t, payload.Name, template.Name)
	assert.Equal(t, payload.Type, template.Type)
	assert.Equal(t, payload.IsPublic, template.IsPublic)
}

func TestReportsAPI_GetReportSchedules(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	req, _ := http.NewRequest("GET", "/api/reports/schedules", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var schedules []models.ReportSchedule
	err := json.Unmarshal(w.Body.Bytes(), &schedules)
	require.NoError(t, err)
	assert.Equal(t, 1, len(schedules))
	assert.Equal(t, "Daily Objects Report", schedules[0].Name)
}

func TestReportsAPI_CreateReportSchedule(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	payload := CreateReportScheduleRequest{
		Name:        "Weekly Report",
		Description: "Weekly objects report",
		TemplateID:  1, // Существующий шаблон
		Type:        models.ScheduleTypeWeekly,
		TimeOfDay:   "10:00",
		DayOfWeek:   1, // Понедельник
		Parameters:  map[string]interface{}{"status": "all"},
		Format:      models.ReportFormatExcel,
		Recipients:  []string{"manager@test.com"},
	}

	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/api/reports/schedules", bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var schedule models.ReportSchedule
	err := json.Unmarshal(w.Body.Bytes(), &schedule)
	require.NoError(t, err)
	assert.Equal(t, payload.Name, schedule.Name)
	assert.Equal(t, payload.Type, schedule.Type)
	assert.Equal(t, payload.TemplateID, schedule.TemplateID)
}

func TestReportsAPI_GetReportsStats(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	req, _ := http.NewRequest("GET", "/api/reports/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var stats map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &stats)
	require.NoError(t, err)

	assert.Equal(t, float64(2), stats["total_reports"])
	assert.Equal(t, float64(1), stats["completed_reports"])
	assert.Equal(t, float64(2), stats["total_templates"])
	assert.Equal(t, float64(1), stats["active_schedules"])
}

func TestReportsAPI_ValidationErrors(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)
	router := setupReportsTestRouter(db)

	tests := []struct {
		name           string
		method         string
		url            string
		payload        interface{}
		expectedStatus int
	}{
		{
			name:   "Create report without name",
			method: "POST",
			url:    "/api/reports",
			payload: map[string]interface{}{
				"type":   "objects",
				"format": "csv",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Create template without name",
			method: "POST",
			url:    "/api/reports/templates",
			payload: map[string]interface{}{
				"type": "users",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Create schedule with non-existent template",
			method: "POST",
			url:    "/api/reports/schedules",
			payload: map[string]interface{}{
				"name":        "Test Schedule",
				"template_id": 999,
				"type":        "daily",
				"format":      "csv",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonPayload, _ := json.Marshal(tt.payload)
			req, _ := http.NewRequest(tt.method, tt.url, bytes.NewBuffer(jsonPayload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestReportsAPI_CompanyIsolation(t *testing.T) {
	db := setupReportsTestDB(t)
	createReportsTestData(t, db)

	// Создаем отчет для другой компании
	otherCompanyReport := models.Report{
		Name:        "Other Company Report",
		Type:        models.ReportTypeObjects,
		Status:      models.ReportStatusCompleted,
		Format:      models.ReportFormatCSV,
		CreatedByID: 1,
		CompanyID:   2, // Другая компания
	}
	require.NoError(t, db.Create(&otherCompanyReport).Error)

	router := setupReportsTestRouter(db)

	// Пытаемся получить отчет другой компании
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/reports/%d", otherCompanyReport.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Должны получить 404, так как отчет принадлежит другой компании
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Бенчмарк тесты

func BenchmarkReportsAPI_GetReports(b *testing.B) {
	db := setupReportsTestDB(&testing.T{})
	createReportsTestData(&testing.T{}, db)
	router := setupReportsTestRouter(db)

	// Создаем много отчетов для тестирования производительности
	for i := 0; i < 100; i++ {
		report := models.Report{
			Name:        fmt.Sprintf("Benchmark Report %d", i),
			Type:        models.ReportTypeObjects,
			Status:      models.ReportStatusCompleted,
			Format:      models.ReportFormatCSV,
			CreatedByID: 1,
			CompanyID:   1,
		}
		db.Create(&report)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "/api/reports", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			b.Fatal("Expected status OK")
		}
	}
}

func BenchmarkReportsAPI_CreateReport(b *testing.B) {
	db := setupReportsTestDB(&testing.T{})
	createReportsTestData(&testing.T{}, db)
	router := setupReportsTestRouter(db)

	payload := CreateReportRequest{
		Name:        "Benchmark Report",
		Description: "Test description",
		Type:        models.ReportTypeObjects,
		Format:      models.ReportFormatCSV,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload.Name = fmt.Sprintf("Benchmark Report %d", i)
		jsonPayload, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", "/api/reports", bytes.NewBuffer(jsonPayload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			b.Fatal("Expected status Created")
		}
	}
}
