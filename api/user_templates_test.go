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

// setupUserTemplatesTestDB создает тестовую базу данных для user templates
func setupUserTemplatesTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.UserTemplate{},
		&models.Role{},
	)
	require.NoError(t, err)

	// Создаем тестовую роль
	role := models.Role{
		ID:   1,
		Name: "Test Role",
	}
	db.Create(&role)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupUserTemplatesTestRouter создает тестовый роутер с middleware
func setupUserTemplatesTestRouter(_ *testing.T, db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки tenant_db
	router.Use(func(c *gin.Context) {
		c.Set("tenant_db", db)
		c.Next()
	})

	return router
}

// TestGetUserTemplates_Success тестирует успешное получение списка шаблонов
func TestGetUserTemplates_Success(t *testing.T) {
	db := setupUserTemplatesTestDB(t)
	router := setupUserTemplatesTestRouter(t, db)

	router.GET("/api/user-templates", GetUserTemplates)

	req, _ := http.NewRequest("GET", "/api/user-templates", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestGetUserTemplates_WithFilters тестирует GetUserTemplates с фильтрами
func TestGetUserTemplates_WithFilters(t *testing.T) {
	db := setupUserTemplatesTestDB(t)
	router := setupUserTemplatesTestRouter(t, db)

	router.GET("/api/user-templates", GetUserTemplates)

	req, _ := http.NewRequest("GET", "/api/user-templates?active_only=true&search=test&page=1&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetUserTemplate_NotFound тестирует GetUserTemplate когда шаблон не найден
func TestGetUserTemplate_NotFound(t *testing.T) {
	db := setupUserTemplatesTestDB(t)
	router := setupUserTemplatesTestRouter(t, db)

	router.GET("/api/user-templates/:id", GetUserTemplate)

	req, _ := http.NewRequest("GET", "/api/user-templates/99999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestCreateUserTemplate_ValidationError тестирует CreateUserTemplate с ошибкой валидации
func TestCreateUserTemplate_ValidationError(t *testing.T) {
	db := setupUserTemplatesTestDB(t)
	router := setupUserTemplatesTestRouter(t, db)

	router.POST("/api/user-templates", CreateUserTemplate)

	// Тест с пустым name
	reqBody := map[string]interface{}{
		"name":    "",
		"role_id": 1,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/user-templates", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestCreateUserTemplate_Success тестирует успешное создание шаблона
func TestCreateUserTemplate_Success(t *testing.T) {
	db := setupUserTemplatesTestDB(t)
	router := setupUserTemplatesTestRouter(t, db)

	router.POST("/api/user-templates", CreateUserTemplate)

	reqBody := map[string]interface{}{
		"name":        "Test Template",
		"description": "Test Description",
		"role_id":     1,
		"is_active":   true,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/user-templates", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestUpdateUserTemplate_NotFound тестирует UpdateUserTemplate когда шаблон не найден
func TestUpdateUserTemplate_NotFound(t *testing.T) {
	db := setupUserTemplatesTestDB(t)
	router := setupUserTemplatesTestRouter(t, db)

	router.PUT("/api/user-templates/:id", UpdateUserTemplate)

	reqBody := map[string]interface{}{
		"name": "Updated Template",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/api/user-templates/99999", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestDeleteUserTemplate_NotFound тестирует DeleteUserTemplate когда шаблон не найден
func TestDeleteUserTemplate_NotFound(t *testing.T) {
	db := setupUserTemplatesTestDB(t)
	router := setupUserTemplatesTestRouter(t, db)

	router.DELETE("/api/user-templates/:id", DeleteUserTemplate)

	req, _ := http.NewRequest("DELETE", "/api/user-templates/99999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
