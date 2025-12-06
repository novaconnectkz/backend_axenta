package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
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

// setupTrashTestDB создает тестовую базу данных для trash
func setupTrashTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.DeletedItem{},
	)
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// setupTrashTestRouter создает тестовый роутер с middleware
func setupTrashTestRouter(_ *testing.T, db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки tenant_db и user_id
	router.Use(func(c *gin.Context) {
		c.Set("tenant_db", db)
		c.Set("user_id", uint(1))
		c.Set("user", map[string]interface{}{
			"id":        1,
			"username":  "testuser",
			"accountId": float64(123),
		})
		c.Next()
	})

	return router
}

// TestGetTrashItems_Success тестирует успешное получение списка элементов корзины
func TestGetTrashItems_Success(t *testing.T) {
	db := setupTrashTestDB(t)
	router := setupTrashTestRouter(t, db)

	router.GET("/api/trash", GetTrashItems)

	req, _ := http.NewRequest("GET", "/api/trash", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestGetTrashItems_WithFilters тестирует GetTrashItems с фильтрами
func TestGetTrashItems_WithFilters(t *testing.T) {
	db := setupTrashTestDB(t)
	router := setupTrashTestRouter(t, db)

	router.GET("/api/trash", GetTrashItems)

	req, _ := http.NewRequest("GET", "/api/trash?type=object&limit=10&offset=0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetTrashStats_Success тестирует успешное получение статистики корзины
func TestGetTrashStats_Success(t *testing.T) {
	db := setupTrashTestDB(t)
	router := setupTrashTestRouter(t, db)

	router.GET("/api/trash/stats", GetTrashStats)

	req, _ := http.NewRequest("GET", "/api/trash/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
}

// TestRestoreItem_NotFound тестирует RestoreItem когда элемент не найден
func TestRestoreItem_NotFound(t *testing.T) {
	db := setupTrashTestDB(t)
	router := setupTrashTestRouter(t, db)

	router.POST("/api/trash/:id/restore", RestoreItem)

	req, _ := http.NewRequest("POST", "/api/trash/99999/restore", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestPermanentlyDeleteItem_NotFound тестирует PermanentlyDeleteItem когда элемент не найден
func TestPermanentlyDeleteItem_NotFound(t *testing.T) {
	db := setupTrashTestDB(t)
	router := setupTrashTestRouter(t, db)

	router.DELETE("/api/trash/:id", PermanentlyDeleteItem)

	req, _ := http.NewRequest("DELETE", "/api/trash/99999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
