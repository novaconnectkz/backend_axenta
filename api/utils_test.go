package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestIsDemoMode_True тестирует isDemoMode с demo=1
func TestIsDemoMode_True(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		result := isDemoMode(c)
		c.JSON(http.StatusOK, gin.H{"demo": result})
	})

	req, _ := http.NewRequest("GET", "/test?demo=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	assert.True(t, response["demo"].(bool))
}

// TestIsDemoMode_TrueString тестирует isDemoMode с demo=true
func TestIsDemoMode_TrueString(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		result := isDemoMode(c)
		c.JSON(http.StatusOK, gin.H{"demo": result})
	})

	req, _ := http.NewRequest("GET", "/test?demo=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	assert.True(t, response["demo"].(bool))
}

// TestIsDemoMode_False тестирует isDemoMode без параметра demo
func TestIsDemoMode_False(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		result := isDemoMode(c)
		c.JSON(http.StatusOK, gin.H{"demo": result})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	assert.False(t, response["demo"].(bool))
}

// TestIsDemoMode_FalseOtherValue тестирует isDemoMode с другим значением
func TestIsDemoMode_FalseOtherValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		result := isDemoMode(c)
		c.JSON(http.StatusOK, gin.H{"demo": result})
	})

	req, _ := http.NewRequest("GET", "/test?demo=0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	assert.False(t, response["demo"].(bool))
}

// TestIsDemoMode_FalseFalseString тестирует isDemoMode с demo=false
func TestIsDemoMode_FalseFalseString(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		result := isDemoMode(c)
		c.JSON(http.StatusOK, gin.H{"demo": result})
	})

	req, _ := http.NewRequest("GET", "/test?demo=false", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	assert.False(t, response["demo"].(bool))
}
