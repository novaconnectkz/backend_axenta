package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// setupCORSTestRouter создает тестовый роутер с CORS middleware
func setupCORSTestRouter(config CustomCORSConfig) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CustomCORS(config))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})
	return router
}

// TestCustomCORS_AllowedOrigin тестирует CORS с разрешенным origin
func TestCustomCORS_AllowedOrigin(t *testing.T) {
	config := CustomCORSConfig{
		AllowOrigins:     []string{"http://localhost:3000", "https://example.com"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		ExposeHeaders:    []string{"X-Total-Count"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	router := setupCORSTestRouter(config)

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

// TestCustomCORS_NotAllowedOrigin тестирует CORS с неразрешенным origin
func TestCustomCORS_NotAllowedOrigin(t *testing.T) {
	config := CustomCORSConfig{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	router := setupCORSTestRouter(config)

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://malicious.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Origin не должен быть установлен для неразрешенного origin
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

// TestCustomCORS_NoOrigin тестирует CORS без Origin заголовка
func TestCustomCORS_NoOrigin(t *testing.T) {
	config := CustomCORSConfig{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	router := setupCORSTestRouter(config)

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

// TestCustomCORS_OPTIONSRequest тестирует CORS для OPTIONS запроса (preflight)
func TestCustomCORS_OPTIONSRequest(t *testing.T) {
	config := CustomCORSConfig{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	router := setupCORSTestRouter(config)

	req, _ := http.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type,Authorization")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "GET,POST,PUT,DELETE", w.Header().Get("Access-Control-Allow-Methods"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	assert.Equal(t, "43200", w.Header().Get("Access-Control-Max-Age"))
}

// TestCustomCORS_XTenantIDHeader тестирует CORS с x-tenant-id заголовком
func TestCustomCORS_XTenantIDHeader(t *testing.T) {
	config := CustomCORSConfig{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	router := setupCORSTestRouter(config)

	req, _ := http.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type,x-tenant-id")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	// x-tenant-id должен быть добавлен к разрешенным заголовкам
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "x-tenant-id")
}

// TestCustomCORS_ExposeHeaders тестирует Expose-Headers
func TestCustomCORS_ExposeHeaders(t *testing.T) {
	config := CustomCORSConfig{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Content-Type"},
		ExposeHeaders:    []string{"X-Total-Count", "X-Page-Number"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	router := setupCORSTestRouter(config)

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Access-Control-Expose-Headers"), "X-Total-Count")
	assert.Contains(t, w.Header().Get("Access-Control-Expose-Headers"), "X-Page-Number")
}

// TestCustomCORS_MultipleOrigins тестирует CORS с несколькими разрешенными origin
func TestCustomCORS_MultipleOrigins(t *testing.T) {
	config := CustomCORSConfig{
		AllowOrigins:     []string{"http://localhost:3000", "https://example.com", "https://app.example.com"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	router := setupCORSTestRouter(config)

	// Тест с первым origin
	req1, _ := http.NewRequest("GET", "/test", nil)
	req1.Header.Set("Origin", "http://localhost:3000")
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, "http://localhost:3000", w1.Header().Get("Access-Control-Allow-Origin"))

	// Тест со вторым origin
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.Header.Set("Origin", "https://example.com")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, "https://example.com", w2.Header().Get("Access-Control-Allow-Origin"))
}

// TestCustomCORS_VaryHeader тестирует Vary заголовок
func TestCustomCORS_VaryHeader(t *testing.T) {
	config := CustomCORSConfig{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	router := setupCORSTestRouter(config)

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))
}
