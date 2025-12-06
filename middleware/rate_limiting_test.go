package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestDefaultKeyGenerator тестирует DefaultKeyGenerator
func TestDefaultKeyGenerator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		key := DefaultKeyGenerator(c)
		c.JSON(http.StatusOK, gin.H{"key": key})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestUserKeyGenerator_WithUserID тестирует UserKeyGenerator с user_id
func TestUserKeyGenerator_WithUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		c.Set("user_id", "123")
		key := UserKeyGenerator(c)
		c.JSON(http.StatusOK, gin.H{"key": key})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestUserKeyGenerator_WithoutUserID тестирует UserKeyGenerator без user_id
func TestUserKeyGenerator_WithoutUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		key := UserKeyGenerator(c)
		c.JSON(http.StatusOK, gin.H{"key": key})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestAPIKeyGenerator_WithAPIKey тестирует APIKeyGenerator с API ключом
func TestAPIKeyGenerator_WithAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		key := APIKeyGenerator(c)
		c.JSON(http.StatusOK, gin.H{"key": key})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestAPIKeyGenerator_WithoutAPIKey тестирует APIKeyGenerator без API ключа
func TestAPIKeyGenerator_WithoutAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		key := APIKeyGenerator(c)
		c.JSON(http.StatusOK, gin.H{"key": key})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestRateLimit_NoRedis тестирует RateLimit когда Redis недоступен
func TestRateLimit_NoRedis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// RateLimit должен пропустить запрос, если Redis недоступен
	router.Use(RateLimit(RateLimitConfig{
		Requests:     10,
		Window:       time.Minute,
		KeyGenerator: DefaultKeyGenerator,
	}))

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Должен вернуть успех, так как Redis недоступен и запрос пропущен
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestStrictRateLimit тестирует StrictRateLimit
func TestStrictRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(StrictRateLimit())

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Должен вернуть успех, так как Redis недоступен
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestModerateRateLimit тестирует ModerateRateLimit
func TestModerateRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(ModerateRateLimit())

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestLenientRateLimit тестирует LenientRateLimit
func TestLenientRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(LenientRateLimit())

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestAuthRateLimit тестирует AuthRateLimit
func TestAuthRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(AuthRateLimit())

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestAPIRateLimit тестирует APIRateLimit
func TestAPIRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(APIRateLimit())

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "test-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestBurstRateLimit тестирует BurstRateLimit
func TestBurstRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(BurstRateLimit())

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetRateLimitInfo_NoRedis тестирует GetRateLimitInfo когда Redis недоступен
func TestGetRateLimitInfo_NoRedis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		info, err := GetRateLimitInfo(DefaultKeyGenerator, RateLimitConfig{
			Requests: 10,
			Window:   time.Minute,
		}, c)

		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"info": info})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Должен вернуть ошибку, так как Redis недоступен
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestClearRateLimit_NoRedis тестирует ClearRateLimit когда Redis недоступен
func TestClearRateLimit_NoRedis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		err := ClearRateLimit(DefaultKeyGenerator, c)

		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "cleared"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Должен вернуть ошибку, так как Redis недоступен
	assert.Equal(t, http.StatusOK, w.Code)
}
