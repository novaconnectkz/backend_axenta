package api

import (
	"backend_axenta/database"
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
// Ф3-B: snapshot-only, без live-proxy — нет БД → 200 + degraded:true (не 4xx/5xx).
func TestGetObjectsFromAxentaCloud_NoTenantDB(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/api/axenta-proxy/objects", GetObjectsFromAxentaCloud)

	req, _ := http.NewRequest("GET", "/api/axenta-proxy/objects", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Degraded bool          `json:"degraded"`
			Items    []interface{} `json:"items"`
		} `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.True(t, resp.Data.Degraded, "нет БД → degraded:true")
	assert.Empty(t, resp.Data.Items, "нет БД → пустой список")
}

// TestGetDeletedObjectsFromAxentaCloud_NoTenantDB — Ф3-B7: корзина snapshot-only,
// без live-proxy в axenta.cloud. Нет БД → 200 + degraded:true (не 4xx/5xx).
func TestGetDeletedObjectsFromAxentaCloud_NoTenantDB(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/api/auth/cms/trash", GetDeletedObjectsFromAxentaCloud)

	req, _ := http.NewRequest("GET", "/api/auth/cms/trash", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Degraded bool          `json:"degraded"`
			Items    []interface{} `json:"items"`
		} `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.True(t, resp.Data.Degraded, "нет БД → degraded:true")
	assert.Empty(t, resp.Data.Items, "нет БД → пустой список")
}

// TestGetObjectsFromAxentaCloud_Success тестирует успешное получение объектов
func TestGetObjectsFromAxentaCloud_Success(t *testing.T) {
	db := setupAxentaProxyTestDB(t)
	router := setupAxentaProxyTestRouter(t, db)

	router.GET("/api/axenta-proxy/objects", GetObjectsFromAxentaCloud)

	req, _ := http.NewRequest("GET", "/api/axenta-proxy/objects", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Ф3-B: snapshot-only — всегда 200 (пустой snapshot → degraded), не 4xx/5xx.
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetObjectsStatsFromAxentaCloud_Success тестирует GetObjectsStatsFromAxentaCloud
func TestGetObjectsStatsFromAxentaCloud_Success(t *testing.T) {
	db := setupAxentaProxyTestDB(t)
	router := setupAxentaProxyTestRouter(t, db)

	router.GET("/api/axenta-proxy/objects/stats", GetObjectsStatsFromAxentaCloud)

	req, _ := http.NewRequest("GET", "/api/axenta-proxy/objects/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Ф3-B: snapshot-only — без токена/без snapshot всё равно 200 + degraded:true
	// (никакого live-proxy в axenta.cloud по request-токену, см. local-auth-ph1 грабля #2).
	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Degraded bool `json:"degraded"`
		} `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.True(t, resp.Data.Degraded, "пустой snapshot → degraded:true")
}

// TestGetUsersFromAxentaCloud_Success тестирует GetUsersFromAxentaCloud
func TestGetUsersFromAxentaCloud_Success(t *testing.T) {
	db := setupAxentaProxyTestDB(t)
	router := setupAxentaProxyTestRouter(t, db)

	router.GET("/api/axenta-proxy/users", GetUsersFromAxentaCloud)

	req, _ := http.NewRequest("GET", "/api/axenta-proxy/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Ф3-B: snapshot-only — всегда 200 (пустой snapshot → degraded), не 4xx/5xx.
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetUsersStatsFromAxentaCloud_Success тестирует GetUsersStatsFromAxentaCloud
func TestGetUsersStatsFromAxentaCloud_Success(t *testing.T) {
	db := setupAxentaProxyTestDB(t)
	router := setupAxentaProxyTestRouter(t, db)

	router.GET("/api/axenta-proxy/users/stats", GetUsersStatsFromAxentaCloud)

	req, _ := http.NewRequest("GET", "/api/axenta-proxy/users/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Ф3-B: snapshot-only — без токена/без snapshot всё равно 200 + degraded:true
	// (никакого live-proxy в axenta.cloud по request-токену).
	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Degraded bool `json:"degraded"`
		} `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.True(t, resp.Data.Degraded, "пустой snapshot → degraded:true")
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
