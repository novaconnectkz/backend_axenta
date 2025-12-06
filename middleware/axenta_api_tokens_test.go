package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestNewAxentaAPITokensMiddleware тестирует создание нового middleware
func TestNewAxentaAPITokensMiddleware(t *testing.T) {
	middleware := NewAxentaAPITokensMiddleware()
	assert.NotNil(t, middleware)
}

// TestAxentaAPITokensMiddleware_RequireValidToken_NoHeader тестирует RequireValidToken без заголовка
func TestAxentaAPITokensMiddleware_RequireValidToken_NoHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	middleware := NewAxentaAPITokensMiddleware()

	router.GET("/test", middleware.RequireValidToken(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestAxentaAPITokensMiddleware_RequireValidToken_InvalidToken тестирует RequireValidToken с неверным токеном
func TestAxentaAPITokensMiddleware_RequireValidToken_InvalidToken(t *testing.T) {
	// Сохраняем оригинальное значение
	originalTokens := os.Getenv("AXENTA_API_TOKENS")
	defer os.Setenv("AXENTA_API_TOKENS", originalTokens)

	// Устанавливаем валидные токены
	os.Setenv("AXENTA_API_TOKENS", "valid-token-1,valid-token-2")

	gin.SetMode(gin.TestMode)
	router := gin.New()

	middleware := NewAxentaAPITokensMiddleware()

	router.GET("/test", middleware.RequireValidToken(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestAxentaAPITokensMiddleware_RequireValidToken_ValidTokenBearer тестирует RequireValidToken с валидным токеном (Bearer)
func TestAxentaAPITokensMiddleware_RequireValidToken_ValidTokenBearer(t *testing.T) {
	// Сохраняем оригинальное значение
	originalTokens := os.Getenv("AXENTA_API_TOKENS")
	defer os.Setenv("AXENTA_API_TOKENS", originalTokens)

	// Устанавливаем валидные токены
	os.Setenv("AXENTA_API_TOKENS", "valid-token-1,valid-token-2")

	gin.SetMode(gin.TestMode)
	router := gin.New()

	middleware := NewAxentaAPITokensMiddleware()

	router.GET("/test", middleware.RequireValidToken(), func(c *gin.Context) {
		token := GetCurrentAPIToken(c)
		c.JSON(http.StatusOK, gin.H{"token": token, "message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestAxentaAPITokensMiddleware_RequireValidToken_ValidTokenTokenPrefix тестирует RequireValidToken с валидным токеном (Token prefix)
func TestAxentaAPITokensMiddleware_RequireValidToken_ValidTokenTokenPrefix(t *testing.T) {
	// Сохраняем оригинальное значение
	originalTokens := os.Getenv("AXENTA_API_TOKENS")
	defer os.Setenv("AXENTA_API_TOKENS", originalTokens)

	// Устанавливаем валидные токены
	os.Setenv("AXENTA_API_TOKENS", "valid-token-1,valid-token-2")

	gin.SetMode(gin.TestMode)
	router := gin.New()

	middleware := NewAxentaAPITokensMiddleware()

	router.GET("/test", middleware.RequireValidToken(), func(c *gin.Context) {
		token := GetCurrentAPIToken(c)
		c.JSON(http.StatusOK, gin.H{"token": token, "message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Token valid-token-2")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestAxentaAPITokensMiddleware_RequireValidToken_ValidTokenDirect тестирует RequireValidToken с валидным токеном (прямой токен)
func TestAxentaAPITokensMiddleware_RequireValidToken_ValidTokenDirect(t *testing.T) {
	// Сохраняем оригинальное значение
	originalTokens := os.Getenv("AXENTA_API_TOKENS")
	defer os.Setenv("AXENTA_API_TOKENS", originalTokens)

	// Устанавливаем валидные токены
	os.Setenv("AXENTA_API_TOKENS", "valid-token-1,valid-token-2")

	gin.SetMode(gin.TestMode)
	router := gin.New()

	middleware := NewAxentaAPITokensMiddleware()

	router.GET("/test", middleware.RequireValidToken(), func(c *gin.Context) {
		token := GetCurrentAPIToken(c)
		c.JSON(http.StatusOK, gin.H{"token": token, "message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "valid-token-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestAxentaAPITokensMiddleware_RequireValidToken_NoEnvVar тестирует RequireValidToken когда переменная окружения не установлена
func TestAxentaAPITokensMiddleware_RequireValidToken_NoEnvVar(t *testing.T) {
	// Сохраняем оригинальное значение
	originalTokens := os.Getenv("AXENTA_API_TOKENS")
	defer os.Setenv("AXENTA_API_TOKENS", originalTokens)

	// Удаляем переменную окружения
	os.Unsetenv("AXENTA_API_TOKENS")

	gin.SetMode(gin.TestMode)
	router := gin.New()

	middleware := NewAxentaAPITokensMiddleware()

	router.GET("/test", middleware.RequireValidToken(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer any-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestAxentaAPITokensMiddleware_RequireValidToken_EmptyToken тестирует RequireValidToken с пустым токеном
func TestAxentaAPITokensMiddleware_RequireValidToken_EmptyToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	middleware := NewAxentaAPITokensMiddleware()

	router.GET("/test", middleware.RequireValidToken(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestGetCurrentAPIToken тестирует GetCurrentAPIToken
func TestGetCurrentAPIToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		c.Set("api_token", "test-token")
		token := GetCurrentAPIToken(c)
		c.JSON(http.StatusOK, gin.H{"token": token})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetCurrentAPIToken_NoToken тестирует GetCurrentAPIToken когда токен отсутствует
func TestGetCurrentAPIToken_NoToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		token := GetCurrentAPIToken(c)
		c.JSON(http.StatusOK, gin.H{"token": token})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
