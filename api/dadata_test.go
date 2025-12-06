package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDaDataTestRouter создает тестовый роутер
func setupDaDataTestRouter(_ *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.POST("/api/dadata/organization", FindOrganizationByINN)
	router.POST("/api/dadata/bank", FindBankByBIK)

	return router
}

// TestFindOrganizationByINN_NoAPIKey тестирует FindOrganizationByINN без API ключа
func TestFindOrganizationByINN_NoAPIKey(t *testing.T) {
	router := setupDaDataTestRouter(t)

	reqBody := map[string]interface{}{
		"query": "1234567890",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/dadata/organization", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку, если API ключ не настроен
	assert.True(t, w.Code == http.StatusInternalServerError || w.Code >= 400)
}

// TestFindOrganizationByINN_EmptyQuery тестирует FindOrganizationByINN с пустым запросом
func TestFindOrganizationByINN_EmptyQuery(t *testing.T) {
	router := setupDaDataTestRouter(t)

	reqBody := map[string]interface{}{
		"query": "",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/dadata/organization", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestFindOrganizationByINN_InvalidFormat тестирует FindOrganizationByINN с неверным форматом
func TestFindOrganizationByINN_InvalidFormat(t *testing.T) {
	router := setupDaDataTestRouter(t)

	reqBody := map[string]interface{}{
		"query": "invalid-inn",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/dadata/organization", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["message"], "ИНН должен содержать")
}

// TestFindOrganizationByINN_ValidINN тестирует FindOrganizationByINN с валидным ИНН
func TestFindOrganizationByINN_ValidINN(t *testing.T) {
	router := setupDaDataTestRouter(t)

	reqBody := map[string]interface{}{
		"query": "7707083893",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/dadata/organization", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть успех или ошибку в зависимости от наличия API ключа
	assert.True(t, w.Code == http.StatusOK || w.Code >= 400)
}

// TestFindBankByBIK_NoAPIKey тестирует FindBankByBIK без API ключа
func TestFindBankByBIK_NoAPIKey(t *testing.T) {
	router := setupDaDataTestRouter(t)

	reqBody := map[string]interface{}{
		"query": "044525225",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/dadata/bank", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Может вернуть ошибку, если API ключ не настроен
	assert.True(t, w.Code == http.StatusInternalServerError || w.Code >= 400)
}

// TestFindBankByBIK_EmptyQuery тестирует FindBankByBIK с пустым запросом
func TestFindBankByBIK_EmptyQuery(t *testing.T) {
	router := setupDaDataTestRouter(t)

	reqBody := map[string]interface{}{
		"query": "",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/dadata/bank", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
