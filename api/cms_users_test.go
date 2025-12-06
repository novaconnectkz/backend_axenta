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

// setupCmsUsersTestDB создает тестовую базу данных для CMS users
func setupCmsUsersTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.DB{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.User{},
		&models.Company{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupCmsUsersTestRouter создает тестовый роутер с middleware
func setupCmsUsersTestRouter(_ *testing.T, db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки tenant_db
	router.Use(func(c *gin.Context) {
		c.Set("tenant_db", db)
		c.Set("user", map[string]interface{}{
			"id":        1,
			"username":  "testuser",
			"accountId": float64(123),
		})
		c.Next()
	})

	return router
}

// TestCreateCmsUser_ValidationError тестирует CreateCmsUser с ошибкой валидации
func TestCreateCmsUser_ValidationError(t *testing.T) {
	db := setupCmsUsersTestDB(t)
	router := setupCmsUsersTestRouter(t, db)

	router.POST("/api/cms-users", CreateCmsUser)

	// Тест с пустым email
	reqBody := map[string]interface{}{
		"name":     "Test User",
		"username": "testuser",
		"email":    "",
		"password": "password123",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/cms-users", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestCreateCmsUser_InvalidEmail тестирует CreateCmsUser с неверным email
func TestCreateCmsUser_InvalidEmail(t *testing.T) {
	db := setupCmsUsersTestDB(t)
	router := setupCmsUsersTestRouter(t, db)

	router.POST("/api/cms-users", CreateCmsUser)

	reqBody := map[string]interface{}{
		"name":     "Test User",
		"username": "testuser",
		"email":    "invalid-email",
		"password": "password123",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/cms-users", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestCreateCmsUser_ShortPassword тестирует CreateCmsUser с коротким паролем
func TestCreateCmsUser_ShortPassword(t *testing.T) {
	db := setupCmsUsersTestDB(t)
	router := setupCmsUsersTestRouter(t, db)

	router.POST("/api/cms-users", CreateCmsUser)

	reqBody := map[string]interface{}{
		"name":     "Test User",
		"username": "testuser",
		"email":    "test@example.com",
		"password": "12345", // Меньше 6 символов
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/cms-users", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetCmsUser_NotFound тестирует GetCmsUser когда пользователь не найден
func TestGetCmsUser_NotFound(t *testing.T) {
	db := setupCmsUsersTestDB(t)
	router := setupCmsUsersTestRouter(t, db)

	router.GET("/api/cms-users/:id", GetCmsUser)

	req, _ := http.NewRequest("GET", "/api/cms-users/99999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
