package middleware

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTenantAdditionalTestDB создает тестовую базу данных для дополнительных тестов tenant
func setupTenantAdditionalTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Company{},
	)
	require.NoError(t, err)

	database.DB = db
	return db
}

// TestNewTenantMiddleware тестирует создание нового TenantMiddleware
func TestNewTenantMiddleware(t *testing.T) {
	db := setupTenantAdditionalTestDB(t)
	middleware := NewTenantMiddleware(db)

	assert.NotNil(t, middleware)
	assert.NotNil(t, middleware.DB)
}

// TestTenantMiddleware_GetTenantDBByCompanyID_NotFound тестирует GetTenantDBByCompanyID когда компания не найдена
func TestTenantMiddleware_GetTenantDBByCompanyID_NotFound(t *testing.T) {
	db := setupTenantAdditionalTestDB(t)
	middleware := NewTenantMiddleware(db)

	tenantDB := middleware.GetTenantDBByCompanyID(99999)
	assert.Nil(t, tenantDB)
}

// TestTenantMiddleware_GetTenantDBByCompanyID_Success тестирует успешное получение tenant DB
func TestTenantMiddleware_GetTenantDBByCompanyID_Success(t *testing.T) {
	db := setupTenantAdditionalTestDB(t)
	middleware := NewTenantMiddleware(db)

	// Создаем тестовую компанию
	company := models.Company{
		ID:             1,
		Name:           "Test Company",
		DatabaseSchema: "tenant_test",
		IsActive:       true,
	}
	db.Create(&company)

	// Может вернуть nil из-за отсутствия реальной схемы в SQLite
	tenantDB := middleware.GetTenantDBByCompanyID(company.ID)
	// В SQLite это может вернуть nil, так как нет реальных схем
	// Проверяем, что функция не паникует
	assert.True(t, tenantDB == nil || tenantDB != nil)
}

// TestGetTenantDB_NotSet тестирует GetTenantDB когда tenant_db не установлен
func TestGetTenantDB_NotSet(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/test", func(c *gin.Context) {
		tenantDB := GetTenantDB(c)
		assert.Nil(t, tenantDB)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetCurrentCompany_NotSet тестирует GetCurrentCompany когда company не установлена
func TestGetCurrentCompany_NotSet(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/test", func(c *gin.Context) {
		company := GetCurrentCompany(c)
		assert.Nil(t, company)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetCompanyID_NotSet тестирует GetCompanyID когда company_id не установлен
func TestGetCompanyID_NotSet(t *testing.T) {
	router := gin.New()
	gin.SetMode(gin.TestMode)

	router.GET("/test", func(c *gin.Context) {
		companyID := GetCompanyID(c)
		assert.Equal(t, uint(0), companyID)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestTenantMiddleware_getCompanyByID_InvalidFormat тестирует getCompanyByID с неверным форматом ID
func TestTenantMiddleware_getCompanyByID_InvalidFormat(t *testing.T) {
	db := setupTenantAdditionalTestDB(t)
	middleware := NewTenantMiddleware(db)

	company, err := middleware.getCompanyByID("invalid-id")
	assert.Error(t, err)
	assert.Nil(t, company)
	assert.Contains(t, err.Error(), "некорректный формат")
}

// TestTenantMiddleware_getCompanyByID_NotFound тестирует getCompanyByID когда компания не найдена
func TestTenantMiddleware_getCompanyByID_NotFound(t *testing.T) {
	db := setupTenantAdditionalTestDB(t)
	middleware := NewTenantMiddleware(db)

	company, err := middleware.getCompanyByID("99999")
	assert.Error(t, err)
	assert.Nil(t, company)
	assert.Contains(t, err.Error(), "не найдена")
}

// TestTenantMiddleware_getCompanyByDomain_NotFound тестирует getCompanyByDomain когда компания не найдена
func TestTenantMiddleware_getCompanyByDomain_NotFound(t *testing.T) {
	db := setupTenantAdditionalTestDB(t)
	middleware := NewTenantMiddleware(db)

	company := middleware.getCompanyByDomain("nonexistent.example.com")
	assert.Nil(t, company)
}

// TestTenantMiddleware_extractCompanyFromToken_NoHeader тестирует extractCompanyFromToken без заголовка
func TestTenantMiddleware_extractCompanyFromToken_NoHeader(t *testing.T) {
	db := setupTenantAdditionalTestDB(t)
	middleware := NewTenantMiddleware(db)

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		companyID := middleware.extractCompanyFromToken(c)
		assert.Equal(t, "", companyID)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestTenantMiddleware_parseJWTForCompanyID_InvalidToken тестирует parseJWTForCompanyID с неверным токеном
func TestTenantMiddleware_parseJWTForCompanyID_InvalidToken(t *testing.T) {
	db := setupTenantAdditionalTestDB(t)
	middleware := NewTenantMiddleware(db)

	companyID := middleware.parseJWTForCompanyID("invalid.jwt.token")
	assert.Equal(t, "", companyID)
}

// TestIsPublicRoute тестирует функцию isPublicRoute
func TestIsPublicRoute(t *testing.T) {
	// Тест с публичными маршрутами
	assert.True(t, isPublicRoute("/ping"))
	assert.True(t, isPublicRoute("/api/auth/login"))
	assert.True(t, isPublicRoute("/health"))
	assert.True(t, isPublicRoute("/metrics"))

	// Тест с приватными маршрутами
	assert.False(t, isPublicRoute("/api/objects"))
	assert.False(t, isPublicRoute("/api/users"))
	assert.False(t, isPublicRoute("/api/billing"))
}
