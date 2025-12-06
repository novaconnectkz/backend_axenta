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

// setupBillingSimpleTestRouter создает тестовый роутер
func setupBillingSimpleTestRouter(_ *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	return router
}

// TestGetBillingPlansSimple_Success тестирует GetBillingPlansSimple
func TestGetBillingPlansSimple_Success(t *testing.T) {
	router := setupBillingSimpleTestRouter(t)

	router.GET("/api/billing/plans/simple", GetBillingPlansSimple)

	req, _ := http.NewRequest("GET", "/api/billing/plans/simple", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	data, ok := response["data"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, 0, len(data)) // Должен вернуть пустой массив
}

// TestGetSubscriptionsSimple_Success тестирует GetSubscriptionsSimple
func TestGetSubscriptionsSimple_Success(t *testing.T) {
	router := setupBillingSimpleTestRouter(t)

	router.GET("/api/billing/subscriptions/simple", GetSubscriptionsSimple)

	req, _ := http.NewRequest("GET", "/api/billing/subscriptions/simple", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	data, ok := response["data"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, 0, len(data)) // Должен вернуть пустой массив
}

// TestGetBillingSettingsSimple_Success тестирует GetBillingSettingsSimple
func TestGetBillingSettingsSimple_Success(t *testing.T) {
	router := setupBillingSimpleTestRouter(t)

	router.GET("/api/billing/settings/simple", GetBillingSettingsSimple)

	req, _ := http.NewRequest("GET", "/api/billing/settings/simple", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	data, ok := response["data"].(map[string]interface{})
	require.True(t, ok)
	assert.NotNil(t, data["id"])
	assert.NotNil(t, data["company_id"])
}
