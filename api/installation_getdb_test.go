package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newCtxWith возвращает gin.Context для unit-тестов helper'ов,
// принимающих *gin.Context. Без HTTP routing.
func newCtxWith(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c
}

func openMemDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	return db
}

// =====================================================================
// InstallationAPI.getDB — приоритет tenant_db из контекста
// =====================================================================

func TestInstallationAPI_GetDB_PrefersTenantFromContext(t *testing.T) {
	fallback := openMemDB(t)
	tenant := openMemDB(t)
	api := NewInstallationAPI(fallback, nil)

	c := newCtxWith(t)
	c.Set("tenant_db", tenant)

	got := api.getDB(c)
	assert.Same(t, tenant, got, "должна вернуться tenant_db из контекста, не fallback")
	assert.NotSame(t, fallback, got)
}

func TestInstallationAPI_GetDB_FallsBackWhenContextEmpty(t *testing.T) {
	fallback := openMemDB(t)
	api := NewInstallationAPI(fallback, nil)

	c := newCtxWith(t)
	// tenant_db не положен — middleware не отработал

	got := api.getDB(c)
	assert.Same(t, fallback, got, "без tenant_db должна вернуться api.DB")
}

func TestInstallationAPI_GetDB_FallsBackOnWrongType(t *testing.T) {
	fallback := openMemDB(t)
	api := NewInstallationAPI(fallback, nil)

	c := newCtxWith(t)
	c.Set("tenant_db", "not a gorm.DB") // защита от повреждённого контекста

	got := api.getDB(c)
	assert.Same(t, fallback, got, "при неверном типе tenant_db — fallback на api.DB")
}

func TestInstallationAPI_GetDB_FallsBackOnNilTenantDB(t *testing.T) {
	fallback := openMemDB(t)
	api := NewInstallationAPI(fallback, nil)

	c := newCtxWith(t)
	var nilDB *gorm.DB
	c.Set("tenant_db", nilDB)

	got := api.getDB(c)
	// (*gorm.DB)(nil) — валидный type assertion, поэтому возвращается nil tenant.
	// Это документирует текущее поведение: middleware обязан класть НЕ-nil DB.
	assert.Nil(t, got, "при nil-указателе в tenant_db helper возвращает его как есть — middleware ответственно за non-nil")
}

func TestInstallationAPI_GetDB_BothNilReturnsNil(t *testing.T) {
	api := NewInstallationAPI(nil, nil)
	c := newCtxWith(t)

	got := api.getDB(c)
	assert.Nil(t, got, "ни tenant_db, ни api.DB — должен вернуться nil без паники")
}

// =====================================================================
// installationCompanyID — соседний helper, проверим заодно
// =====================================================================

func TestInstallationCompanyID_FromContext(t *testing.T) {
	c := newCtxWith(t)
	c.Set("company_id", uint(42))
	assert.Equal(t, uint(42), installationCompanyID(c))
}

func TestInstallationCompanyID_AbsentReturnsZero(t *testing.T) {
	c := newCtxWith(t)
	assert.Equal(t, uint(0), installationCompanyID(c))
}

func TestInstallationCompanyID_WrongTypeReturnsZero(t *testing.T) {
	c := newCtxWith(t)
	c.Set("company_id", "not-uint")
	assert.Equal(t, uint(0), installationCompanyID(c))
}
