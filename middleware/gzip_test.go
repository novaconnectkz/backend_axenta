package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestGzipMiddleware_WithGzipSupport тестирует GzipMiddleware когда клиент поддерживает gzip
func TestGzipMiddleware_WithGzipSupport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(GzipMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello, World!")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))

	// Проверяем, что ответ сжат
	reader, err := gzip.NewReader(w.Body)
	assert.NoError(t, err)
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Equal(t, "Hello, World!", string(decompressed))
}

// TestGzipMiddleware_WithoutGzipSupport тестирует GzipMiddleware когда клиент не поддерживает gzip
func TestGzipMiddleware_WithoutGzipSupport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(GzipMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello, World!")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	// Не устанавливаем Accept-Encoding
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, "Hello, World!", w.Body.String())
}

// TestGzipMiddleware_NoContent тестирует GzipMiddleware с StatusNoContent
func TestGzipMiddleware_NoContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(GzipMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	// Для StatusNoContent не должно быть Content-Encoding
	assert.Empty(t, w.Header().Get("Content-Encoding"))
}

// TestGzipMiddleware_JSONResponse тестирует GzipMiddleware с JSON ответом
func TestGzipMiddleware_JSONResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(GzipMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Hello, World!"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))

	// Декомпрессируем ответ
	reader, err := gzip.NewReader(w.Body)
	assert.NoError(t, err)
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Contains(t, string(decompressed), "Hello, World!")
}

// TestGzipMiddleware_LargeResponse тестирует GzipMiddleware с большим ответом
func TestGzipMiddleware_LargeResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(GzipMiddleware())
	router.GET("/test", func(c *gin.Context) {
		// Создаем большой ответ
		largeData := strings.Repeat("A", 10000)
		c.String(http.StatusOK, largeData)
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))

	// Проверяем, что сжатый ответ меньше оригинального
	assert.True(t, w.Body.Len() < 10000, "Сжатый ответ должен быть меньше оригинального")

	// Декомпрессируем и проверяем содержимое
	reader, err := gzip.NewReader(w.Body)
	assert.NoError(t, err)
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Equal(t, 10000, len(decompressed))
}

// TestGzipMiddleware_DifferentEncodings тестирует GzipMiddleware с разными Accept-Encoding
func TestGzipMiddleware_DifferentEncodings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(GzipMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello, World!")
	})

	testCases := []struct {
		name           string
		acceptEncoding string
		shouldCompress bool
	}{
		{"gzip only", "gzip", true},
		{"gzip with deflate", "gzip, deflate", true},
		{"deflate only", "deflate", false},
		{"no encoding", "", false},
		{"identity", "identity", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/test", nil)
			if tc.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tc.acceptEncoding)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			if tc.shouldCompress {
				assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
			} else {
				assert.Empty(t, w.Header().Get("Content-Encoding"))
			}
		})
	}
}
