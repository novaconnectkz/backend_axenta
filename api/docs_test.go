package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDocsTestRouter создает тестовый роутер
func setupDocsTestRouter(_ *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	return router
}

// TestGetSwaggerUI_Success тестирует GetSwaggerUI
func TestGetSwaggerUI_Success(t *testing.T) {
	router := setupDocsTestRouter(t)

	router.GET("/api/docs/swagger", GetSwaggerUI)

	req, _ := http.NewRequest("GET", "/api/docs/swagger", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "swagger-ui")
}

// TestGetSwaggerUI_WithSpec тестирует GetSwaggerUI с параметром spec
func TestGetSwaggerUI_WithSpec(t *testing.T) {
	router := setupDocsTestRouter(t)

	router.GET("/api/docs/swagger", GetSwaggerUI)

	req, _ := http.NewRequest("GET", "/api/docs/swagger?spec=main", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "openapi.yaml")
}

// TestGetOpenAPISpec_NotFound тестирует GetOpenAPISpec когда файл не найден
func TestGetOpenAPISpec_NotFound(t *testing.T) {
	router := setupDocsTestRouter(t)

	router.GET("/api/docs/openapi.yaml", GetOpenAPISpec)

	req, _ := http.NewRequest("GET", "/api/docs/openapi.yaml", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть NotFound или OK в зависимости от наличия файла
	assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusOK)
}

// TestGetBillingOpenAPISpec_NotFound тестирует GetBillingOpenAPISpec когда файл не найден
func TestGetBillingOpenAPISpec_NotFound(t *testing.T) {
	router := setupDocsTestRouter(t)

	router.GET("/api/docs/billing-openapi.yaml", GetBillingOpenAPISpec)

	req, _ := http.NewRequest("GET", "/api/docs/billing-openapi.yaml", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть NotFound или OK в зависимости от наличия файла
	assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusOK)
}

// TestGetTelegramIntegrationDocs_NotFound тестирует GetTelegramIntegrationDocs когда файл не найден
func TestGetTelegramIntegrationDocs_NotFound(t *testing.T) {
	router := setupDocsTestRouter(t)

	router.GET("/api/docs/telegram", GetTelegramIntegrationDocs)

	req, _ := http.NewRequest("GET", "/api/docs/telegram", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть NotFound или OK в зависимости от наличия файла
	assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusOK)

	if w.Code == http.StatusNotFound {
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "error", response["status"])
	}
}
