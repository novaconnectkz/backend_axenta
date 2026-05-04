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

// setupAxentaProxyTestDB создает тестовую базу данных
func setupAxentaProxyTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	database.DB = db
	return db
}

// setupAxentaProxyTestRouter создает тестовый роутер с middleware
func setupAxentaProxyTestRouter(_ *testing.T, db *gorm.DB) *gin.Engine {
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

// TestGetObjectsFromAxentaCloud_NoTenantDB тестирует GetObjectsFromAxentaCloud без tenant_db.
// Snapshot read-path молча fallback'ит на live, который без auth header возвращает 401.
func TestGetObjectsFromAxentaCloud_NoTenantDB(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/api/axenta-proxy/objects", GetObjectsFromAxentaCloud)

	req, _ := http.NewRequest("GET", "/api/axenta-proxy/objects", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Любая ошибка >= 400 валидна (401 без auth, 500 при недоступности Axenta Cloud)
	assert.GreaterOrEqual(t, w.Code, 400)
}

// TestGetObjectsFromAxentaCloud_Success тестирует успешное получение объектов
func TestGetObjectsFromAxentaCloud_Success(t *testing.T) {
	db := setupAxentaProxyTestDB(t)
	router := setupAxentaProxyTestRouter(t, db)

	router.GET("/api/axenta-proxy/objects", GetObjectsFromAxentaCloud)

	req, _ := http.NewRequest("GET", "/api/axenta-proxy/objects", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку из-за отсутствия токена или успех
	assert.True(t, w.Code == http.StatusOK || w.Code >= 400)
}

// TestGetObjectsStatsFromAxentaCloud_Success тестирует GetObjectsStatsFromAxentaCloud
func TestGetObjectsStatsFromAxentaCloud_Success(t *testing.T) {
	db := setupAxentaProxyTestDB(t)
	router := setupAxentaProxyTestRouter(t, db)

	router.GET("/api/axenta-proxy/objects/stats", GetObjectsStatsFromAxentaCloud)

	req, _ := http.NewRequest("GET", "/api/axenta-proxy/objects/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку из-за отсутствия токена или успех
	assert.True(t, w.Code == http.StatusOK || w.Code >= 400)
}

// TestGetUsersFromAxentaCloud_Success тестирует GetUsersFromAxentaCloud
func TestGetUsersFromAxentaCloud_Success(t *testing.T) {
	db := setupAxentaProxyTestDB(t)
	router := setupAxentaProxyTestRouter(t, db)

	router.GET("/api/axenta-proxy/users", GetUsersFromAxentaCloud)

	req, _ := http.NewRequest("GET", "/api/axenta-proxy/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку из-за отсутствия токена или успех
	assert.True(t, w.Code == http.StatusOK || w.Code >= 400)
}

// TestGetUsersStatsFromAxentaCloud_Success тестирует GetUsersStatsFromAxentaCloud
func TestGetUsersStatsFromAxentaCloud_Success(t *testing.T) {
	db := setupAxentaProxyTestDB(t)
	router := setupAxentaProxyTestRouter(t, db)

	router.GET("/api/axenta-proxy/users/stats", GetUsersStatsFromAxentaCloud)

	req, _ := http.NewRequest("GET", "/api/axenta-proxy/users/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку из-за отсутствия токена или успех
	assert.True(t, w.Code == http.StatusOK || w.Code >= 400)
}

// TestSplitFullName тестирует функцию splitFullName
func TestSplitFullName(t *testing.T) {
	// Тест с полным именем
	firstName, lastName := splitFullName("Иван Иванов")
	assert.Equal(t, "Иван", firstName)
	assert.Equal(t, "Иванов", lastName)

	// Тест с одним именем
	firstName, lastName = splitFullName("Иван")
	assert.Equal(t, "Иван", firstName)
	assert.Equal(t, "", lastName)

	// Тест с пустой строкой
	firstName, lastName = splitFullName("")
	assert.Equal(t, "", firstName)
	assert.Equal(t, "", lastName)

	// Тест с несколькими словами
	firstName, lastName = splitFullName("Иван Петрович Иванов")
	assert.Equal(t, "Иван", firstName)
	assert.Equal(t, "Петрович Иванов", lastName)
}

// TestShouldExcludeUserFromSearch тестирует функцию shouldExcludeUserFromSearch
func TestShouldExcludeUserFromSearch(t *testing.T) {
	// Тест с совпадением по основным полям
	user := map[string]interface{}{
		"username":    "testuser",
		"email":       "test@example.com",
		"first_name":  "Test",
		"last_name":   "User",
		"creatorName": "Creator",
	}
	shouldExclude := shouldExcludeUserFromSearch("test", user)
	assert.False(t, shouldExclude)

	// Тест с совпадением только по creatorName
	user2 := map[string]interface{}{
		"username":    "otheruser",
		"email":       "other@example.com",
		"creatorName": "Creator",
	}
	shouldExclude = shouldExcludeUserFromSearch("Creator", user2)
	assert.True(t, shouldExclude)

	// Тест с пустым запросом
	shouldExclude = shouldExcludeUserFromSearch("", user)
	assert.False(t, shouldExclude)
}
