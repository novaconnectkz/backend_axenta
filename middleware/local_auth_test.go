package middleware

import (
	"backend_axenta/models"
	"backend_axenta/services"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMockJWTService создает мок JWT сервиса для тестов
func setupMockJWTService(t *testing.T) *services.JWTService {
	// Сохраняем оригинальное значение
	originalSecret := os.Getenv("JWT_SECRET")
	defer os.Setenv("JWT_SECRET", originalSecret)

	// Устанавливаем тестовый секрет
	os.Setenv("JWT_SECRET", "test-secret-key-for-testing-only")

	// Используем реальный JWT сервис
	// Для тестов используем nil DB, так как мы не используем refresh токены
	jwtService := services.NewJWTService(nil)
	return jwtService
}

// TestNewLocalAuthMiddleware тестирует создание нового middleware
func TestNewLocalAuthMiddleware(t *testing.T) {
	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	assert.NotNil(t, middleware)
	assert.NotNil(t, middleware.jwtService)
}

// TestLocalAuthMiddleware_RequireAuth_NoHeader тестирует RequireAuth без заголовка
func TestLocalAuthMiddleware_RequireAuth_NoHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	router.GET("/test", middleware.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestLocalAuthMiddleware_RequireAuth_InvalidFormat тестирует RequireAuth с неверным форматом
func TestLocalAuthMiddleware_RequireAuth_InvalidFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	router.GET("/test", middleware.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "InvalidFormat token123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestLocalAuthMiddleware_RequireAuth_InvalidToken тестирует RequireAuth с неверным токеном
func TestLocalAuthMiddleware_RequireAuth_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	router.GET("/test", middleware.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestLocalAuthMiddleware_RequireAuth_ValidToken тестирует RequireAuth с валидным токеном
func TestLocalAuthMiddleware_RequireAuth_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	// Создаем тестового пользователя
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	// Генерируем валидный токен
	token, err := jwtService.GenerateAccessToken(user)
	require.NoError(t, err)

	router.GET("/test", middleware.RequireAuth(), func(c *gin.Context) {
		userID, _ := GetCurrentUserID(c)
		companyID, _ := GetCurrentCompanyID(c)
		role, _ := GetCurrentUserRole(c)
		username, _ := GetCurrentUsername(c)

		c.JSON(http.StatusOK, gin.H{
			"user_id":    userID,
			"company_id": companyID,
			"role":       role,
			"username":   username,
		})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestLocalAuthMiddleware_OptionalAuth_NoHeader тестирует OptionalAuth без заголовка
func TestLocalAuthMiddleware_OptionalAuth_NoHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	router.GET("/test", middleware.OptionalAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Должен пропустить запрос без авторизации
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestLocalAuthMiddleware_OptionalAuth_ValidToken тестирует OptionalAuth с валидным токеном
func TestLocalAuthMiddleware_OptionalAuth_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	// Создаем тестового пользователя
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	// Генерируем валидный токен
	token, err := jwtService.GenerateAccessToken(user)
	require.NoError(t, err)

	router.GET("/test", middleware.OptionalAuth(), func(c *gin.Context) {
		userID, exists := GetCurrentUserID(c)
		if exists {
			c.JSON(http.StatusOK, gin.H{"user_id": userID, "authenticated": true})
		} else {
			c.JSON(http.StatusOK, gin.H{"authenticated": false})
		}
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestLocalAuthMiddleware_RequireRole_NoRole тестирует RequireRole без роли
func TestLocalAuthMiddleware_RequireRole_NoRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	router.GET("/test", middleware.RequireRole("admin"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestLocalAuthMiddleware_RequireRole_InvalidRole тестирует RequireRole с неверной ролью
func TestLocalAuthMiddleware_RequireRole_InvalidRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	// Создаем тестового пользователя с ролью user
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "user",
	}

	// Генерируем токен
	token, err := jwtService.GenerateAccessToken(user)
	require.NoError(t, err)

	router.GET("/test", middleware.RequireAuth(), middleware.RequireRole("admin"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestLocalAuthMiddleware_RequireRole_ValidRole тестирует RequireRole с валидной ролью
func TestLocalAuthMiddleware_RequireRole_ValidRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	// Создаем тестового пользователя с ролью admin
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	// Генерируем токен
	token, err := jwtService.GenerateAccessToken(user)
	require.NoError(t, err)

	router.GET("/test", middleware.RequireAuth(), middleware.RequireRole("admin"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestLocalAuthMiddleware_RequireCompany_NoCompanyID тестирует RequireCompany без company_id
func TestLocalAuthMiddleware_RequireCompany_NoCompanyID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	router.GET("/test", middleware.RequireCompany("123"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestLocalAuthMiddleware_RequireCompany_InvalidCompanyID тестирует RequireCompany с неверным company_id
func TestLocalAuthMiddleware_RequireCompany_InvalidCompanyID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	// Создаем тестового пользователя с company_id = "123"
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	// Генерируем токен
	token, err := jwtService.GenerateAccessToken(user)
	require.NoError(t, err)

	router.GET("/test", middleware.RequireAuth(), middleware.RequireCompany("456"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestLocalAuthMiddleware_RequireCompany_ValidCompanyID тестирует RequireCompany с валидным company_id
func TestLocalAuthMiddleware_RequireCompany_ValidCompanyID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	// Создаем тестового пользователя с company_id = "123"
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	// Генерируем токен
	token, err := jwtService.GenerateAccessToken(user)
	require.NoError(t, err)

	router.GET("/test", middleware.RequireAuth(), middleware.RequireCompany("123"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetCurrentUserID тестирует GetCurrentUserID
func TestGetCurrentUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		c.Set("user_id", uint(123))
		userID, exists := GetCurrentUserID(c)
		c.JSON(http.StatusOK, gin.H{"user_id": userID, "exists": exists})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetCurrentCompanyID тестирует GetCurrentCompanyID
func TestGetCurrentCompanyID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		c.Set("company_id", "123")
		companyID, exists := GetCurrentCompanyID(c)
		c.JSON(http.StatusOK, gin.H{"company_id": companyID, "exists": exists})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetCurrentUserRole тестирует GetCurrentUserRole
func TestGetCurrentUserRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		c.Set("role", "admin")
		role, exists := GetCurrentUserRole(c)
		c.JSON(http.StatusOK, gin.H{"role": role, "exists": exists})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetCurrentUsername тестирует GetCurrentUsername
func TestGetCurrentUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		c.Set("username", "testuser")
		username, exists := GetCurrentUsername(c)
		c.JSON(http.StatusOK, gin.H{"username": username, "exists": exists})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetJWTClaims тестирует GetJWTClaims
func TestGetJWTClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	jwtService := setupMockJWTService(t)
	middleware := NewLocalAuthMiddleware(jwtService)

	// Создаем тестового пользователя
	user := &models.LocalUser{
		ID:        1,
		Username:  "testuser",
		CompanyID: "123",
		Role:      "admin",
	}

	// Генерируем токен
	token, err := jwtService.GenerateAccessToken(user)
	require.NoError(t, err)

	router.GET("/test", middleware.RequireAuth(), func(c *gin.Context) {
		jwtClaims, exists := GetJWTClaims(c)
		c.JSON(http.StatusOK, gin.H{"claims": jwtClaims, "exists": exists})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
