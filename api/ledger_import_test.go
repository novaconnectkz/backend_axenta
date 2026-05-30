package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func parseJSON(t *testing.T, w *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
		t.Fatalf("parse json: %v (body=%s)", err, w.Body.String())
	}
}

func uidStr(v uint) string { return strconv.FormatUint(uint64(v), 10) }

func setupImportTest(t *testing.T) (*gin.Engine, uint, uint) {
	t.Helper()
	if err := database.SetupTestDatabase(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := database.DB.AutoMigrate(&models.Counterparty{}, &models.LedgerEntry{}, &models.LedgerImportBatch{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Два контрагента компании 1.
	cp1 := models.Counterparty{AdminAccountID: 1, CompanyID: 1, IDType: "inn", TaxID: "7701234567", Name: "ООО Альфа"}
	cp2 := models.Counterparty{AdminAccountID: 1, CompanyID: 1, IDType: "other", TaxID: "", Name: "ИП Бета", ManualReview: true}
	database.DB.Create(&cp1)
	database.DB.Create(&cp2)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("admin_account_id", uint(1))
		c.Set("company_id", uint(1))
		c.Set("is_superadmin", true)
		c.Set("username", "tester")
		c.Next()
	})
	r.POST("/counterparties/match", MatchCounterparties)
	r.POST("/ledger/import-batch", PostLedgerImportBatch)
	r.POST("/ledger/import-batch/:id/reverse", PostLedgerImportBatchReverse)
	return r, cp1.ID, cp2.ID
}

func TestMatchCounterparties(t *testing.T) {
	r, cp1, _ := setupImportTest(t)
	defer database.CleanupTestDatabase()

	w := doJSON(t, r, "POST", "/counterparties/match", map[string]any{
		"rows": []map[string]any{
			{"row_index": 0, "identifier": "7701234567", "amount": 100}, // ИНН cp1 → matched
			{"row_index": 1, "identifier": "ИП Бета", "amount": 200},      // имя cp2 → review
			{"row_index": 2, "identifier": "9999999999", "amount": 300},   // нет → nomatch
		},
	})
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	parseJSON(t, w, &resp)
	rows := resp["data"].([]any)
	assert.Equal(t, 3, len(rows))
	r0 := rows[0].(map[string]any)
	assert.Equal(t, "matched", r0["status"])
	assert.Equal(t, float64(cp1), r0["counterparty_id"])
	assert.Equal(t, "review", rows[1].(map[string]any)["status"])
	assert.Equal(t, "nomatch", rows[2].(map[string]any)["status"])
}

func TestImportBatchAndReverse(t *testing.T) {
	r, cp1, cp2 := setupImportTest(t)
	defer database.CleanupTestDatabase()

	// Импорт 2 платежей.
	w := doJSON(t, r, "POST", "/ledger/import-batch", map[string]any{
		"source": "excel",
		"rows": []map[string]any{
			{"counterparty_id": cp1, "amount": 1000, "date": "2026-05-01", "reference": "PAY-1"},
			{"counterparty_id": cp2, "amount": 500, "date": "2026-05-01", "reference": "PAY-2"},
		},
	})
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	parseJSON(t, w, &resp)
	data := resp["data"].(map[string]any)
	assert.Equal(t, float64(2), data["imported"])
	assert.Equal(t, float64(0), data["skipped"])
	batchID := uint(data["batch_id"].(float64))

	// Балансы контрагентов = платежи (платёж уровня контрагента: contract_id=0).
	assert.Equal(t, "1000.00", counterpartyBalance(cp1, 1, 1).StringFixed(2))
	assert.Equal(t, "500.00", counterpartyBalance(cp2, 1, 1).StringFixed(2))

	// Идемпотентность: повтор того же reference → всё skipped.
	w2 := doJSON(t, r, "POST", "/ledger/import-batch", map[string]any{
		"source": "excel",
		"rows": []map[string]any{
			{"counterparty_id": cp1, "amount": 1000, "date": "2026-05-01", "reference": "PAY-1"},
		},
	})
	var resp2 map[string]any
	parseJSON(t, w2, &resp2)
	assert.Equal(t, float64(1), resp2["data"].(map[string]any)["skipped"])
	assert.Equal(t, "1000.00", counterpartyBalance(cp1, 1, 1).StringFixed(2)) // не задвоился

	// Откат всего батча → reversal-проводки, балансы обнуляются.
	wr := doJSON(t, r, "POST", "/ledger/import-batch/"+uidStr(batchID)+"/reverse", map[string]any{})
	assert.Equal(t, http.StatusOK, wr.Code)
	var respR map[string]any
	parseJSON(t, wr, &respR)
	assert.Equal(t, float64(2), respR["data"].(map[string]any)["reversed"])
	assert.Equal(t, "0.00", counterpartyBalance(cp1, 1, 1).StringFixed(2))
	assert.Equal(t, "0.00", counterpartyBalance(cp2, 1, 1).StringFixed(2))

	// Повторный откат идемпотентен (уже сторнировано → 0 новых).
	wr2 := doJSON(t, r, "POST", "/ledger/import-batch/"+uidStr(batchID)+"/reverse", map[string]any{})
	var respR2 map[string]any
	parseJSON(t, wr2, &respR2)
	assert.Equal(t, float64(0), respR2["data"].(map[string]any)["reversed"])

	// Батч помечен reversed.
	var batch models.LedgerImportBatch
	database.DB.First(&batch, batchID)
	assert.Equal(t, "reversed", batch.Status)
}
