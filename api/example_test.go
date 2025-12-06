package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// setupExampleTestRouter создает тестовый роутер для example API
func setupExampleTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/status", GetStatus)
	router.GET("/version", GetVersion)
	router.GET("/health", HealthCheck)

	return router
}

// TestGetStatus тестирует GetStatus
func TestGetStatus(t *testing.T) {
	router := setupExampleTestRouter()

	req, _ := http.NewRequest("GET", "/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	data, ok := response["data"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "Axenta Backend", data["system"])
	assert.Equal(t, "1.0.0", data["version"])
	assert.Equal(t, "ok", data["health"])
}

// TestGetVersion тестирует GetVersion
func TestGetVersion(t *testing.T) {
	router := setupExampleTestRouter()

	req, _ := http.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	data, ok := response["data"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "v1", data["api_version"])
	assert.NotEmpty(t, data["build"])
}

// TestHealthCheck тестирует HealthCheck
func TestHealthCheck(t *testing.T) {
	router := setupExampleTestRouter()

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	data, ok := response["data"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, true, data["alive"])
	assert.NotEmpty(t, data["timestamp"])
}
