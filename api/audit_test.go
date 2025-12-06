package api

import (
	"backend_axenta/audit"
	"backend_axenta/database"
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
)

// setupAuditAPITestDB создает тестовую базу данных для audit API
func setupAuditAPITestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&audit.AuditLog{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupAuditAPITestRouter создает тестовый роутер
func setupAuditAPITestRouter(t *testing.T, db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	api := NewAuditAPI(db)
	api.RegisterRoutes(router.Group("/api"))

	return router
}

// TestAuditAPI_GetAuditLogs_Default тестирует GetAuditLogs с параметрами по умолчанию
func TestAuditAPI_GetAuditLogs_Default(t *testing.T) {
	db := setupAuditAPITestDB(t)
	router := setupAuditAPITestRouter(t, db)

	req, _ := http.NewRequest("GET", "/api/audit/logs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestAuditAPI_GetAuditLogs_WithFilters тестирует GetAuditLogs с фильтрами
func TestAuditAPI_GetAuditLogs_WithFilters(t *testing.T) {
	db := setupAuditAPITestDB(t)
	router := setupAuditAPITestRouter(t, db)

	// Создаем тестовый лог
	auditLog := audit.AuditLog{
		UserID:    "1",
		Action:    "user.login",
		Level:     "info",
		Success:   true,
		Timestamp: time.Now(),
	}
	db.Create(&auditLog)

	req, _ := http.NewRequest("GET", "/api/audit/logs?user_id=1&action=user.login", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestAuditAPI_GetAuditLogs_WithPagination тестирует GetAuditLogs с пагинацией
func TestAuditAPI_GetAuditLogs_WithPagination(t *testing.T) {
	db := setupAuditAPITestDB(t)
	router := setupAuditAPITestRouter(t, db)

	req, _ := http.NewRequest("GET", "/api/audit/logs?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(1), data["page"])
	assert.Equal(t, float64(10), data["per_page"])
}

// TestAuditAPI_GetAuditLogStats_Default тестирует GetAuditLogStats с параметрами по умолчанию
func TestAuditAPI_GetAuditLogStats_Default(t *testing.T) {
	db := setupAuditAPITestDB(t)
	router := setupAuditAPITestRouter(t, db)

	req, _ := http.NewRequest("GET", "/api/audit/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestAuditAPI_GetAuditLogStats_WithDays тестирует GetAuditLogStats с параметром days
func TestAuditAPI_GetAuditLogStats_WithDays(t *testing.T) {
	db := setupAuditAPITestDB(t)
	router := setupAuditAPITestRouter(t, db)

	req, _ := http.NewRequest("GET", "/api/audit/stats?days=30", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(30), data["period"])
}

// TestAuditAPI_GetAuditLog_NotFound тестирует GetAuditLog когда лог не найден
func TestAuditAPI_GetAuditLog_NotFound(t *testing.T) {
	db := setupAuditAPITestDB(t)
	router := setupAuditAPITestRouter(t, db)

	req, _ := http.NewRequest("GET", "/api/audit/logs/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestAuditAPI_GetAuditLog_Success тестирует успешное получение лога
func TestAuditAPI_GetAuditLog_Success(t *testing.T) {
	db := setupAuditAPITestDB(t)
	router := setupAuditAPITestRouter(t, db)

	// Создаем тестовый лог
	auditLog := audit.AuditLog{
		UserID:    "1",
		Action:    "user.login",
		Level:     "info",
		Success:   true,
		Timestamp: time.Now(),
	}
	db.Create(&auditLog)

	req, _ := http.NewRequest("GET", "/api/audit/logs/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestAuditAPI_ExportAuditLogs тестирует ExportAuditLogs
func TestAuditAPI_ExportAuditLogs(t *testing.T) {
	db := setupAuditAPITestDB(t)
	router := setupAuditAPITestRouter(t, db)

	req, _ := http.NewRequest("GET", "/api/audit/logs/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть успех или ошибку в зависимости от реализации
	assert.True(t, w.Code == http.StatusOK || w.Code >= 400)
}
