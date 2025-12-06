package api

import (
	"backend_axenta/database"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAxentaSyncTestDB создает тестовую базу данных
func setupAxentaSyncTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	database.DB = db
	return db
}

// setupAxentaSyncTestRouter создает тестовый роутер
func setupAxentaSyncTestRouter(_ *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	return router
}

// TestSyncAllAxentaUsers_NoTenantDB тестирует SyncAllAxentaUsers без tenant_db
func TestSyncAllAxentaUsers_NoTenantDB(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.POST("/api/axenta-users/sync", SyncAllAxentaUsers)

	req, _ := http.NewRequest("POST", "/api/axenta-users/sync", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestSyncAllAxentaUsers_NoToken тестирует SyncAllAxentaUsers без токена
func TestSyncAllAxentaUsers_NoToken(t *testing.T) {
	setupAxentaSyncTestDB(t)
	router := setupAxentaSyncTestRouter(t)

	// Middleware для установки tenant_db
	router.Use(func(c *gin.Context) {
		c.Set("tenant_db", database.DB)
		c.Next()
	})

	router.POST("/api/axenta-users/sync", SyncAllAxentaUsers)

	req, _ := http.NewRequest("POST", "/api/axenta-users/sync", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestGetSyncedUsersFromLocal_NoTenantDB тестирует GetSyncedUsersFromLocal без tenant_db
func TestGetSyncedUsersFromLocal_NoTenantDB(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/api/axenta-users/synced", GetSyncedUsersFromLocal)

	req, _ := http.NewRequest("GET", "/api/axenta-users/synced", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestGetSyncedUsersFromLocal_Success тестирует успешное получение синхронизированных пользователей
func TestGetSyncedUsersFromLocal_Success(t *testing.T) {
	db := setupAxentaSyncTestDB(t)
	router := setupAxentaSyncTestRouter(t)

	// Middleware для установки tenant_db
	router.Use(func(c *gin.Context) {
		c.Set("tenant_db", db)
		c.Next()
	})

	router.GET("/api/axenta-users/synced", GetSyncedUsersFromLocal)

	req, _ := http.NewRequest("GET", "/api/axenta-users/synced", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
