package api

import (
	"backend_axenta/database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAxentaSyncTriggerTestDB создает тестовую базу данных
func setupAxentaSyncTriggerTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	database.DB = db
	return db
}

// setupAxentaSyncTriggerTestRouter создает тестовый роутер
func setupAxentaSyncTriggerTestRouter(_ *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	return router
}

// TestTriggerAxentaSync_Success тестирует успешный запуск синхронизации
func TestTriggerAxentaSync_Success(t *testing.T) {
	setupAxentaSyncTriggerTestDB(t)
	router := setupAxentaSyncTriggerTestRouter(t)

	router.POST("/api/axenta-sync/trigger", TriggerAxentaSync)

	req, _ := http.NewRequest("POST", "/api/axenta-sync/trigger", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["status"])
	assert.Contains(t, response["message"], "принят")
}
