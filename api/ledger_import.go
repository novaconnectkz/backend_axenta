package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ============================================================================
// Ф5 — Excel-импорт платежей на лицевые счета контрагентов (батч + откат).
// Визард FE: drag→маппинг→/counterparties/match (превью ✅/⚠️/❌)→approve→/ledger/import-batch.
// Откат: /ledger/import-batch/:id/reverse (reversal-проводки, полный/выборочный).
// Платёж уровня контрагента: counterparty_id set, contract_id=0 (единый ЛС).
// ============================================================================

var digitsOnly = func(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// MatchCounterparties — POST /api/auth/counterparties/match
// Превью-матчинг строк Excel на контрагентов: ИНН exact → matched(✅); имя exact → review(⚠️);
// нет → nomatch(❌). Admin-only. Ничего не пишет — только сопоставление.
func MatchCounterparties(c *gin.Context) {
	if !requireContractAssignAccess(c) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "доступно только администратору"})
		return
	}
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "компания не определена в контексте"})
		return
	}
	var req struct {
		Rows []struct {
			RowIndex   int     `json:"row_index"`
			Identifier string  `json:"identifier"` // ИНН или имя
			Name       string  `json:"name"`       // опц. имя (fallback к матчингу по имени)
			Amount     float64 `json:"amount"`
		} `json:"rows" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректные данные"})
		return
	}

	type result struct {
		RowIndex         int     `json:"row_index"`
		Status           string  `json:"status"` // matched | review | nomatch
		CounterpartyID   uint    `json:"counterparty_id"`
		CounterpartyName string  `json:"counterparty_name"`
		Amount           float64 `json:"amount"`
		Note             string  `json:"note,omitempty"`
	}
	out := make([]result, 0, len(req.Rows))
	for _, row := range req.Rows {
		ident := strings.TrimSpace(row.Identifier)
		res := result{RowIndex: row.RowIndex, Status: "nomatch", Amount: row.Amount}

		// 1) Числовой идентификатор → ИНН/БИН exact.
		if digitsOnly(ident) {
			var cp models.Counterparty
			if database.DB.Model(&models.Counterparty{}).
				Where("admin_account_id = ? AND company_id = ? AND tax_id = ? AND deleted_at IS NULL", adminAccountID, companyID, ident).
				First(&cp).Error == nil {
				res.Status = "matched"
				res.CounterpartyID = cp.ID
				res.CounterpartyName = cp.Name
				out = append(out, res)
				continue
			}
		}

		// 2) Матч по имени (exact-normalized) → review (требует подтверждения оператором).
		nameRaw := row.Name
		if nameRaw == "" && !digitsOnly(ident) {
			nameRaw = ident
		}
		name := cpNormalizeName(nameRaw)
		if name != "" {
			var matches []models.Counterparty
			database.DB.Model(&models.Counterparty{}).
				Where("admin_account_id = ? AND company_id = ? AND name = ? AND deleted_at IS NULL", adminAccountID, companyID, name).
				Limit(2).Find(&matches)
			if len(matches) == 1 {
				res.Status = "review"
				res.CounterpartyID = matches[0].ID
				res.CounterpartyName = matches[0].Name
				res.Note = "совпадение по имени — подтвердите"
			} else if len(matches) > 1 {
				res.Status = "review"
				res.Note = "несколько контрагентов с таким именем — выберите вручную"
			}
		}
		out = append(out, res)
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": out})
}

// PostLedgerImportBatch — POST /api/auth/ledger/import-batch
// Импорт платежей пачкой на лицевые счета контрагентов (после превью-матчинга на FE).
// Идемпотентность по external_id (reference из Excel); без reference — батч-уникальный ключ.
func PostLedgerImportBatch(c *gin.Context) {
	if !requireContractAssignAccess(c) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "доступно только администратору"})
		return
	}
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "компания не определена в контексте"})
		return
	}
	var req struct {
		Source   string `json:"source"`
		Currency string `json:"currency"`
		FileName string `json:"file_name"`
		Rows     []struct {
			CounterpartyID uint    `json:"counterparty_id"`
			Amount         float64 `json:"amount"`
			Date           string  `json:"date"`      // YYYY-MM-DD
			Reference      string  `json:"reference"` // external_id (идемпотентность)
			Comment        string  `json:"comment"`
		} `json:"rows" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный реестр"})
		return
	}
	if len(req.Rows) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "пустой реестр"})
		return
	}
	source := req.Source
	if source == "" {
		source = "excel"
	}
	// Whitelist source: иначе дедуп (admin,company,source,external_id) обходится произвольным
	// source → дубль того же reference (Codex Ф5a HIGH-1).
	switch source {
	case "excel", "1c", "bank", "payment_system":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "недопустимый source (excel|1c|bank|payment_system)"})
		return
	}
	currency := req.Currency
	if currency == "" {
		currency = "RUB"
	}
	username, _ := c.Get("username")
	createdBy, _ := username.(string)

	// Валидные контрагенты компании (защита от чужого counterparty_id) — один запрос.
	validCP := map[uint]bool{}
	{
		var ids []uint
		database.DB.Model(&models.Counterparty{}).
			Where("admin_account_id = ? AND company_id = ? AND deleted_at IS NULL", adminAccountID, companyID).
			Pluck("id", &ids)
		for _, id := range ids {
			validCP[id] = true
		}
	}

	batch := models.LedgerImportBatch{
		AdminAccountID: adminAccountID, CompanyID: companyID, Source: source, Status: "imported",
		Currency: currency, FileName: req.FileName, CreatedBy: createdBy, RowsTotal: len(req.Rows),
	}
	imported, skipped := 0, 0
	total := decimal.Zero
	errorsList := make([]string, 0)

	txErr := database.DB.Transaction(func(tx *gorm.DB) error {
		if e := tx.Table("ledger_import_batches").Create(&batch).Error; e != nil {
			return e
		}
		for _, row := range req.Rows {
			// Округляем ДО проверки >0: tiny-positive float (0.004) иначе пройдёт и вставит
			// нулевую проводку (Codex Ф5a MEDIUM-2).
			amount := decimal.NewFromFloat(row.Amount).Round(2)
			if amount.LessThanOrEqual(decimal.Zero) || row.CounterpartyID == 0 {
				skipped++
				errorsList = append(errorsList, fmt.Sprintf("строка cp=%d: некорректная сумма/контрагент", row.CounterpartyID))
				continue
			}
			if !validCP[row.CounterpartyID] {
				skipped++
				errorsList = append(errorsList, fmt.Sprintf("контрагент %d вне компании", row.CounterpartyID))
				continue
			}
			entryDate := time.Now().UTC()
			if row.Date != "" {
				if t, e := time.Parse("2006-01-02", row.Date); e == nil {
					entryDate = t
				}
			}
			extID := strings.TrimSpace(row.Reference)
			if extID == "" {
				// Батч-уникальный ключ (без cross-batch дедупа — оператор не дал референс).
				extID = fmt.Sprintf("impbatch:%d:cp:%d:%s:%s", batch.ID, row.CounterpartyID, amount.StringFixed(2), entryDate.Format("2006-01-02"))
			}
			// Идемпотентность по (admin, company, source, external_id).
			var cnt int64
			if e := tx.Model(&models.LedgerEntry{}).
				Where("admin_account_id = ? AND company_id = ? AND source = ? AND external_id = ? AND deleted_at IS NULL",
					adminAccountID, companyID, source, extID).Count(&cnt).Error; e != nil {
				return e
			}
			if cnt > 0 {
				skipped++
				continue
			}
			entry := models.LedgerEntry{
				AdminAccountID: adminAccountID, CompanyID: companyID,
				ContractID: 0, CounterpartyID: row.CounterpartyID, // платёж уровня контрагента (единый ЛС)
				EntryType: "payment", Amount: amount, Currency: currency,
				Source: source, ExternalID: extID, Description: row.Comment,
				EntryDate: entryDate, CreatedBy: createdBy, ImportBatchID: batch.ID,
			}
			if e := tx.Create(&entry).Error; e != nil {
				return e
			}
			imported++
			total = total.Add(amount)
		}
		// Финализируем счётчики батча.
		return tx.Table("ledger_import_batches").Where("id = ?", batch.ID).
			Updates(map[string]interface{}{"rows_imported": imported, "rows_skipped": skipped, "total_amount": total}).Error
	})
	if txErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": txErr.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
		"batch_id": batch.ID, "rows_total": len(req.Rows), "imported": imported, "skipped": skipped,
		"total_amount": total.StringFixed(2), "errors": errorsList,
	}})
}

// PostLedgerImportBatchReverse — POST /api/auth/ledger/import-batch/:id/reverse
// Откат батча reversal-проводками. Body опц. {"entry_ids":[...]} — выборочный откат;
// пусто → полный (status→reversed). Идемпотентно: уже сторнированные проводки пропускаются.
func PostLedgerImportBatchReverse(c *gin.Context) {
	if !requireContractAssignAccess(c) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "доступно только администратору"})
		return
	}
	batchID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный id батча"})
		return
	}
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "компания не определена в контексте"})
		return
	}
	var req struct {
		EntryIDs []uint `json:"entry_ids"`
	}
	_ = c.ShouldBindJSON(&req)
	selective := map[uint]bool{}
	for _, id := range req.EntryIDs {
		selective[id] = true
	}

	var batch models.LedgerImportBatch
	if database.DB.Table("ledger_import_batches").
		Where("id = ? AND admin_account_id = ? AND company_id = ? AND deleted_at IS NULL", batchID, adminAccountID, companyID).
		First(&batch).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "батч не найден"})
		return
	}

	username, _ := c.Get("username")
	reversedBy, _ := username.(string)
	now := time.Now().UTC()
	reversedCount := 0

	txErr := database.DB.Transaction(func(tx *gorm.DB) error {
		// payment-проводки батча (только в рамках admin+company).
		var entries []models.LedgerEntry
		if e := tx.Where("import_batch_id = ? AND admin_account_id = ? AND company_id = ? AND entry_type = ? AND deleted_at IS NULL",
			batchID, adminAccountID, companyID, "payment").Find(&entries).Error; e != nil {
			return e
		}
		for i := range entries {
			en := &entries[i]
			if len(selective) > 0 && !selective[en.ID] {
				continue
			}
			// Уже сторнирована? (reversal с reversal_of_id = en.ID) → пропустить (идемпотентно).
			var rc int64
			if e := tx.Model(&models.LedgerEntry{}).
				Where("reversal_of_id = ? AND entry_type = ? AND deleted_at IS NULL", en.ID, "reversal").Count(&rc).Error; e != nil {
				return e
			}
			if rc > 0 {
				continue
			}
			rev := models.LedgerEntry{
				AdminAccountID: en.AdminAccountID, CompanyID: en.CompanyID,
				ContractID: en.ContractID, CounterpartyID: en.CounterpartyID,
				EntryType: "reversal", Amount: en.Amount.Neg(), Currency: en.Currency,
				Source: en.Source, ExternalID: fmt.Sprintf("reverse:batch:%d:entry:%d", batchID, en.ID),
				Description: "Откат импорт-батча " + strconv.FormatUint(batchID, 10),
				EntryDate:   now, CreatedBy: reversedBy, ReversalOfID: &en.ID, ImportBatchID: batch.ID,
			}
			if e := tx.Create(&rev).Error; e != nil {
				return e
			}
			reversedCount++
		}
		// Полный откат (не выборочный) → статус батча reversed.
		if len(selective) == 0 {
			if e := tx.Table("ledger_import_batches").Where("id = ?", batchID).
				Updates(map[string]interface{}{"status": "reversed", "reversed_at": now, "reversed_by": reversedBy}).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if txErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": txErr.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
		"batch_id": batchID, "reversed": reversedCount, "selective": len(selective) > 0,
	}})
}

// GetLedgerImportBatches — GET /api/auth/ledger/import-batches
// История батчей импорта компании (для FE: список + кнопка отката).
func GetLedgerImportBatches(c *gin.Context) {
	if !requireContractAssignAccess(c) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "доступно только администратору"})
		return
	}
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "компания не определена в контексте"})
		return
	}
	var batches []models.LedgerImportBatch
	database.DB.Table("ledger_import_batches").
		Where("admin_account_id = ? AND company_id = ? AND deleted_at IS NULL", adminAccountID, companyID).
		Order("created_at DESC").Limit(200).Find(&batches)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": batches})
}
