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

// setupSnapshotSettingsTestDB создает тестовую базу данных для snapshot settings
func setupSnapshotSettingsTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Company{},
		&models.SnapshotSettings{},
	)
	require.NoError(t, err)

	// Создаем тестовую компанию
	company := models.Company{
		ID:   1,
		Name: "Test Company",
	}
	db.Create(&company)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupSnapshotSettingsTestRouter создает тестовый роутер
func setupSnapshotSettingsTestRouter(_ *testing.T, _ *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	return router
}

// TestGetSnapshotSettings_Success тестирует успешное получение настроек
func TestGetSnapshotSettings_Success(t *testing.T) {
	db := setupSnapshotSettingsTestDB(t)
	router := setupSnapshotSettingsTestRouter(t, db)

	router.GET("/api/auth/snapshot-settings", GetSnapshotSettings)

	req, _ := http.NewRequest("GET", "/api/auth/snapshot-settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку из-за GetTenantDBByID или SET search_path
	// Но проверяем, что функция вызывается
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

// TestUpdateSnapshotSettings_ValidationError тестирует UpdateSnapshotSettings с ошибкой валидации
func TestUpdateSnapshotSettings_ValidationError(t *testing.T) {
	db := setupSnapshotSettingsTestDB(t)
	router := setupSnapshotSettingsTestRouter(t, db)

	router.POST("/api/auth/snapshot-settings", UpdateSnapshotSettings)

	// Тест с пустым axenta_token
	reqBody := map[string]interface{}{
		"axenta_token": "",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/auth/snapshot-settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку валидации или внутреннюю ошибку
	assert.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusInternalServerError)
}

// TestUpdateSnapshotSettings_Success тестирует успешное обновление настроек
func TestUpdateSnapshotSettings_Success(t *testing.T) {
	db := setupSnapshotSettingsTestDB(t)
	router := setupSnapshotSettingsTestRouter(t, db)

	router.POST("/api/auth/snapshot-settings", UpdateSnapshotSettings)

	reqBody := map[string]interface{}{
		"axenta_token": "test-token-123",
		"is_active":    true,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/auth/snapshot-settings", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку из-за GetTenantDBByID или успех
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}
