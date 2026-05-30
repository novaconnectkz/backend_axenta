package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// setupCounterpartyTest поднимает in-memory БД + роутер с admin-контекстом.
func setupCounterpartyTest(t *testing.T) *gin.Engine {
	t.Helper()
	if err := database.SetupTestDatabase(); err != nil {
		t.Fatalf("setup test db: %v", err)
	}
	if err := database.DB.AutoMigrate(&models.Counterparty{}, &models.LedgerEntry{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Контекст admin (superadmin) — проходит requireContractAssignAccess + скоуп admin/company.
	r.Use(func(c *gin.Context) {
		c.Set("admin_account_id", uint(1))
		c.Set("company_id", uint(1))
		c.Set("is_superadmin", true)
		c.Set("username", "tester")
		c.Next()
	})
	r.GET("/counterparties/search", SearchCounterparties)
	r.GET("/counterparties", GetCounterparties)
	r.GET("/counterparties/:id", GetCounterparty)
	r.POST("/counterparties", CreateCounterparty)
	r.PUT("/counterparties/:id", UpdateCounterparty)
	r.DELETE("/counterparties/:id", DeleteCounterparty)
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, url string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateCounterparty(t *testing.T) {
	r := setupCounterpartyTest(t)
	defer database.CleanupTestDatabase()

	w := doJSON(t, r, "POST", "/counterparties", models.Counterparty{
		Name:        "ООО Ромашка",
		IDType:      "inn",
		TaxID:       "7701234567",
		CreditLimit: decimal.NewFromInt(5000),
	})
	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "ООО Ромашка", data["name"])
	assert.Equal(t, false, data["manual_review"])
	assert.Greater(t, data["id"].(float64), float64(0))
	// admin/company проставлены сервером, не из тела.
	assert.Equal(t, float64(1), data["admin_account_id"])
	assert.Equal(t, float64(1), data["company_id"])
}

func TestCreateCounterpartyDuplicateTaxID(t *testing.T) {
	r := setupCounterpartyTest(t)
	defer database.CleanupTestDatabase()

	cp := models.Counterparty{Name: "ООО Дубль", IDType: "inn", TaxID: "5009999999"}
	w1 := doJSON(t, r, "POST", "/counterparties", cp)
	assert.Equal(t, http.StatusCreated, w1.Code)

	w2 := doJSON(t, r, "POST", "/counterparties", cp)
	assert.Equal(t, http.StatusConflict, w2.Code)
}

func TestCreateCounterpartyNoTaxIDIsManualReview(t *testing.T) {
	r := setupCounterpartyTest(t)
	defer database.CleanupTestDatabase()

	w := doJSON(t, r, "POST", "/counterparties", models.Counterparty{Name: "ИП Без ИНН"})
	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, true, data["manual_review"])
	// Без tax_id допускаются несколько (партиальный uniq не покрывает) — второй тоже 201.
	w2 := doJSON(t, r, "POST", "/counterparties", models.Counterparty{Name: "ИП Без ИНН 2"})
	assert.Equal(t, http.StatusCreated, w2.Code)
}

func TestCreateCounterpartyValidations(t *testing.T) {
	r := setupCounterpartyTest(t)
	defer database.CleanupTestDatabase()

	// Пустое имя → 400.
	w := doJSON(t, r, "POST", "/counterparties", models.Counterparty{TaxID: "111"})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Недопустимый id_type → 400.
	w = doJSON(t, r, "POST", "/counterparties", models.Counterparty{Name: "X", IDType: "garbage"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearchCounterparties(t *testing.T) {
	r := setupCounterpartyTest(t)
	defer database.CleanupTestDatabase()

	// ASCII-имена: sqlite LOWER() не понижает кириллицу (только ASCII), а на prod PG
	// кейсфолд кириллицы делает COLLATE "und-x-icu" (см. ciLikeExpr / dashboard_search).
	// Здесь тестируем плюмбинг LIKE+scope, не кириллический кейсфолд.
	for _, n := range []string{"Alpha Ltd", "Beta Ltd", "Gamma Ltd"} {
		doJSON(t, r, "POST", "/counterparties", models.Counterparty{Name: n, IDType: "inn", TaxID: fmt.Sprintf("inn-%s", n)})
	}
	w := doJSON(t, r, "GET", "/counterparties/search?q=beta", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	rows := resp["data"].([]interface{})
	assert.Equal(t, 1, len(rows))
	assert.Equal(t, "Beta Ltd", rows[0].(map[string]interface{})["name"])
}

func TestUpdateCounterparty(t *testing.T) {
	r := setupCounterpartyTest(t)
	defer database.CleanupTestDatabase()

	w := doJSON(t, r, "POST", "/counterparties", models.Counterparty{Name: "ООО Старое", IDType: "inn", TaxID: "7700000001"})
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	id := uint(resp["data"].(map[string]interface{})["id"].(float64))

	w2 := doJSON(t, r, "PUT", fmt.Sprintf("/counterparties/%d", id), models.Counterparty{
		Name:        "ООО Новое",
		IDType:      "inn",
		TaxID:       "7700000001",
		CreditLimit: decimal.NewFromInt(9999),
	})
	assert.Equal(t, http.StatusOK, w2.Code)
	var resp2 map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	data := resp2["data"].(map[string]interface{})
	assert.Equal(t, "ООО Новое", data["name"])
	assert.Equal(t, "9999", data["credit_limit"])
	// Скоуп-ключи неизменны.
	assert.Equal(t, float64(1), data["admin_account_id"])
}

func TestDeleteCounterparty(t *testing.T) {
	r := setupCounterpartyTest(t)
	defer database.CleanupTestDatabase()

	// Без ссылок — удаляется.
	w := doJSON(t, r, "POST", "/counterparties", models.Counterparty{Name: "ООО Удаляемое", IDType: "inn", TaxID: "7700000009"})
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	id := uint(resp["data"].(map[string]interface{})["id"].(float64))
	wDel := doJSON(t, r, "DELETE", fmt.Sprintf("/counterparties/%d", id), nil)
	assert.Equal(t, http.StatusOK, wDel.Code)

	// С проводкой — удаление запрещено (409).
	w2 := doJSON(t, r, "POST", "/counterparties", models.Counterparty{Name: "ООО С проводкой", IDType: "inn", TaxID: "7700000010"})
	var resp2 map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	id2 := uint(resp2["data"].(map[string]interface{})["id"].(float64))
	database.DB.Create(&models.LedgerEntry{
		AdminAccountID: 1, CompanyID: 1, ContractID: 100, CounterpartyID: id2,
		EntryType: "payment", Amount: decimal.NewFromInt(100), Currency: "RUB",
		Source: "manual", EntryDate: time.Now().UTC(),
	})
	wDel2 := doJSON(t, r, "DELETE", fmt.Sprintf("/counterparties/%d", id2), nil)
	assert.Equal(t, http.StatusConflict, wDel2.Code)
}

func TestCounterpartyScopeIsolation(t *testing.T) {
	r := setupCounterpartyTest(t)
	defer database.CleanupTestDatabase()

	// Контрагент другого admin не должен быть виден/удаляем.
	other := models.Counterparty{AdminAccountID: 999, CompanyID: 1, Name: "Чужой", IDType: "inn", TaxID: "8800000001"}
	database.DB.Create(&other)

	w := doJSON(t, r, "GET", fmt.Sprintf("/counterparties/%d", other.ID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Поиск с q-фильтром НЕ должен протекать на чужого admin (OR обёрнут в скобки).
	database.DB.Create(&models.Counterparty{AdminAccountID: 999, CompanyID: 1, Name: "Searchme Ltd", IDType: "inn", TaxID: "8800000002"})
	ws := doJSON(t, r, "GET", "/counterparties/search?q=searchme", nil)
	assert.Equal(t, http.StatusOK, ws.Code)
	var resp map[string]interface{}
	_ = json.Unmarshal(ws.Body.Bytes(), &resp)
	assert.Nil(t, resp["data"], "чужой admin не должен находиться через q-поиск")
}
