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

// setupAxentaUsersTestDB создает тестовую базу данных
func setupAxentaUsersTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.User{},
		&models.Role{},
	)
	require.NoError(t, err)

	// Создаем тестовую роль
	role := models.Role{
		ID:   1,
		Name: "Test Role",
	}
	db.Create(&role)

	database.DB = db
	return db
}

// setupAxentaUsersTestRouter создает тестовый роутер с middleware
func setupAxentaUsersTestRouter(_ *testing.T, db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки tenant_db
	router.Use(func(c *gin.Context) {
		c.Set("tenant_db", db)
		c.Next()
	})

	return router
}

// TestGetAxentaUsers_NoTenantDB тестирует GetAxentaUsers без tenant_db
func TestGetAxentaUsers_NoTenantDB(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/api/axenta-users", GetAxentaUsers)

	req, _ := http.NewRequest("GET", "/api/axenta-users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestGetAxentaUsers_Success тестирует успешное получение пользователей
func TestGetAxentaUsers_Success(t *testing.T) {
	db := setupAxentaUsersTestDB(t)
	router := setupAxentaUsersTestRouter(t, db)

	router.GET("/api/axenta-users", GetAxentaUsers)

	req, _ := http.NewRequest("GET", "/api/axenta-users?type=all", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestCreateLocalUser_ValidationError тестирует CreateLocalUser с ошибкой валидации
func TestCreateLocalUser_ValidationError(t *testing.T) {
	db := setupAxentaUsersTestDB(t)
	router := setupAxentaUsersTestRouter(t, db)

	router.POST("/api/axenta-users/local", CreateLocalUser)

	// Тест с пустым username
	reqBody := map[string]interface{}{
		"username": "",
		"email":    "test@example.com",
		"password": "password123",
		"role_id":  1,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/axenta-users/local", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestCreateLocalUser_InvalidEmail тестирует CreateLocalUser с неверным email
func TestCreateLocalUser_InvalidEmail(t *testing.T) {
	db := setupAxentaUsersTestDB(t)
	router := setupAxentaUsersTestRouter(t, db)

	router.POST("/api/axenta-users/local", CreateLocalUser)

	reqBody := map[string]interface{}{
		"username": "testuser",
		"email":    "invalid-email",
		"password": "password123",
		"role_id":  1,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/axenta-users/local", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestCreateLocalUser_RoleNotFound тестирует CreateLocalUser когда роль не найдена
func TestCreateLocalUser_RoleNotFound(t *testing.T) {
	db := setupAxentaUsersTestDB(t)
	router := setupAxentaUsersTestRouter(t, db)

	router.POST("/api/axenta-users/local", CreateLocalUser)

	reqBody := map[string]interface{}{
		"username": "testuser",
		"email":    "test@example.com",
		"password": "password123",
		"role_id":  99999, // Несуществующая роль
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/axenta-users/local", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetUsersByAxentaType_Success тестирует GetUsersByAxentaType
func TestGetUsersByAxentaType_Success(t *testing.T) {
	db := setupAxentaUsersTestDB(t)
	router := setupAxentaUsersTestRouter(t, db)

	router.GET("/api/axenta-users/by-type", GetUsersByAxentaType)

	req, _ := http.NewRequest("GET", "/api/axenta-users/by-type?type=partner", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
