package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ============================================================================
// Billing holds (П3 отсрочка + П4 обещанный платёж).
// «Зонт» над договором: временно блокирует авто-приостановку за долг, НЕ трогая
// баланс ledger (источник правды). Lifecycle гасит sweep в LedgerChargeScheduler.
// ============================================================================

var errHoldExists = fmt.Errorf("active hold already exists")

type ledgerHoldRequest struct {
	ContractID uint    `json:"contract_id" binding:"required"`
	HoldType   string  `json:"hold_type" binding:"required"`  // deferral | promise
	HoldUntil  string  `json:"hold_until" binding:"required"` // YYYY-MM-DD — до какой даты держим
	Amount     float64 `json:"amount"`                        // обязательна для promise
	Reason     string  `json:"reason"`
}

// PostLedgerHold — POST /api/auth/ledger/hold
// Создаёт зонт отсрочки/обещанного платежа над договором. Гейты: доступ к договору
// + право операции по политике компании. Политика: deferral → hold_until ≤ now+MaxDeferralDays;
// promise → AllowPromisedPayments=true. Создание зонта снимает активную debt-приостановку.
func PostLedgerHold(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	var req ledgerHoldRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректные данные зонта"})
		return
	}
	if req.HoldType != "deferral" && req.HoldType != "promise" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "hold_type должен быть deferral или promise"})
		return
	}
	holdDay, e := time.Parse("2006-01-02", req.HoldUntil)
	if e != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "hold_until должен быть в формате YYYY-MM-DD"})
		return
	}
	// Семантика «держим включительно весь выбранный день»: храним hold_until как
	// EXCLUSIVE-границу = начало следующего дня (00:00 UTC). Sweep держит зонт пока
	// now < hold_until, поэтому выбранный день покрыт целиком (и локальное утро
	// следующего дня для TZ +N — KZ/RU полностью внутри). FE при показе вычитает 1 день.
	holdUntil := holdUntilExclusive(holdDay)
	now := time.Now().UTC()
	if !holdUntil.After(now) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "дата зонта должна быть в будущем"})
		return
	}
	if !managerCanAccessContract(c, req.ContractID) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "договор вне вашего доступа"})
		return
	}

	// company_id берём из договора (tenant-схема).
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.DB
	}
	var contract models.Contract
	if err := tenantDB.Select("id, company_id, credit_limit, counterparty_id, currency").First(&contract, req.ContractID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "договор не найден"})
		return
	}

	// Право на операцию по политике компании (OperationRoleThreshold).
	if !requireBillingOperation(c, adminAccountID, contract.CompanyID) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "операция недоступна для вашей роли (см. политику биллинга)"})
		return
	}

	// Политика компании: разрешённость типа зонта + лимит отсрочки.
	var bs models.BillingSettings
	if err := database.DB.Table("public.billing_settings").
		Where("company_id = ? AND admin_account_id = ?", contract.CompanyID, adminAccountID).First(&bs).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "политика биллинга не настроена"})
		return
	}
	amount := decimal.Zero
	switch req.HoldType {
	case "deferral":
		if bs.MaxDeferralDays <= 0 {
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "отсрочка не разрешена политикой компании"})
			return
		}
		if !deferralWithinPolicy(holdUntil, now, bs.MaxDeferralDays) {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": fmt.Sprintf("отсрочка превышает лимит политики (%d дней)", bs.MaxDeferralDays)})
			return
		}
	case "promise":
		if !bs.AllowPromisedPayments {
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "обещанные платежи не разрешены политикой компании"})
			return
		}
		if req.Amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "для обещанного платежа нужна положительная сумма"})
			return
		}
		amount = decimal.NewFromFloat(req.Amount).Round(2)
	}

	// Долг на момент создания (для отчёта): balance < 0 → долг.
	// Scope строго company+admin+contract (contract_id может совпадать в разных
	// tenant-схемах одного admin → без company_id балансы смешались бы — Codex).
	// Ф2: долг считаем по балансу КОНТРАГЕНТА (единый ЛС); cp=0 → legacy per-договор.
	debtAtCreate := decimal.Zero
	var bal decimal.Decimal
	// Долг для отчёта — в валюте договора (hold per-договорной семантики); фильтр currency
	// чтобы валюто-слепой SUM не схлопнул разные валюты контрагента.
	balQ := database.DB.Model(&models.LedgerEntry{}).
		Where("admin_account_id = ? AND company_id = ? AND currency = ? AND deleted_at IS NULL",
			adminAccountID, contract.CompanyID, contractCurrency(contract.Currency))
	if contract.CounterpartyID != 0 {
		balQ = balQ.Where("counterparty_id = ?", contract.CounterpartyID)
	} else {
		balQ = balQ.Where("contract_id = ?", req.ContractID)
	}
	balQ.Select("COALESCE(SUM(amount),0)").Scan(&bal)
	if bal.IsNegative() {
		debtAtCreate = bal.Abs()
	}

	username, _ := c.Get("username")
	createdBy, _ := username.(string)

	// Ф3: зонт уровня КОНТРАГЕНТА (cp<>0 → блокирует приостановку всех его договоров,
	// ContractID=0) либо legacy per-договор (cp=0 → ContractID=req.ContractID).
	cpID := contract.CounterpartyID
	holdContractID := req.ContractID
	if cpID != 0 {
		holdContractID = 0
	}
	hold := models.BillingHold{
		AdminAccountID: adminAccountID,
		CompanyID:      contract.CompanyID,
		ContractID:     holdContractID,
		CounterpartyID: cpID,
		HoldType:       req.HoldType,
		Amount:         amount,
		Currency:       contractCurrency(contract.Currency),
		HoldUntil:      holdUntil,
		Status:         "active",
		Active:         true,
		DebtAtCreate:   debtAtCreate,
		Reason:         req.Reason,
		CreatedBy:      createdBy,
	}

	// Схема компании для tenant-таблицы contracts (одна физ. БД, разные схемы) —
	// чтобы UPDATE статуса шёл в ТОЙ ЖЕ public-транзакции (атомарность, Codex #1).
	tenantSchema := fmt.Sprintf("tenant_%d", contract.CompanyID)
	{
		var comp models.Company
		if database.DB.Table("public.companies").Select("id, database_schema").First(&comp, contract.CompanyID).Error == nil && comp.DatabaseSchema != "" {
			tenantSchema = comp.DatabaseSchema
		}
	}
	contractsTable := fmt.Sprintf(`"%s".contracts`, tenantSchema)

	// Транзакция (вся в одной физ. БД): создать зонт (partial-unique ловит дубль) +
	// снять активную debt-приостановку этого договора, вернув ПРЕЖНИЙ статус
	// (UX: оператор дал отсрочку → сервис разблокируется сразу). Ручной suspend не трогаем.
	var snappedSusps []models.BillingSuspension // снятые в tx — для физразблока после коммита (§3.7b)
	txErr := database.DB.Transaction(func(tx *gorm.DB) error {
		// Анти-дубль: один активный зонт на ЕДИНИЦУ (контрагент cp<>0 / договор cp=0).
		// Гонку ловит partial-unique (idx_hold_cp/idx_hold_legacy) → ниже 23505→409 (Codex #5).
		dupQ := tx.Table("public.billing_holds").
			Where("admin_account_id = ? AND company_id = ? AND active = ? AND deleted_at IS NULL",
				adminAccountID, contract.CompanyID, true)
		if cpID != 0 {
			dupQ = dupQ.Where("counterparty_id = ?", cpID)
		} else {
			dupQ = dupQ.Where("counterparty_id = 0 AND contract_id = ?", req.ContractID)
		}
		var cnt int64
		if e := dupQ.Count(&cnt).Error; e != nil {
			return e
		}
		if cnt > 0 {
			return errHoldExists
		}
		if e := tx.Table("public.billing_holds").Create(&hold).Error; e != nil {
			return e
		}
		// Снять активную debt-приостановку ЕДИНИЦЫ + вернуть её suspended-договоры в 'active'
		// (UX: оператор дал отсрочку → сервис разблокируется сразу). Ручной suspend не трогаем.
		suspQ := tx.Table("public.billing_suspensions").
			Where("admin_account_id = ? AND company_id = ? AND reason = ? AND active = ? AND deleted_at IS NULL",
				adminAccountID, contract.CompanyID, "billing_debt", true)
		if cpID != 0 {
			suspQ = suspQ.Where("counterparty_id = ?", cpID)
		} else {
			suspQ = suspQ.Where("counterparty_id = 0 AND contract_id = ?", req.ContractID)
		}
		// Б3.5 §3.7a: зонт per-контрагент снимает ВСЕ его активные debt-приостановки. После Б3
		// их может быть N (по строке на договор) — раньше First() снимал ровно ОДНУ из N.
		var susps []models.BillingSuspension
		if e := suspQ.Find(&susps).Error; e != nil {
			return e
		}
		restoreSet := map[uint]bool{}
		for i := range susps {
			s := &susps[i]
			if e := tx.Table("public.billing_suspensions").Where("id = ?", s.ID).
				Updates(map[string]interface{}{"active": false, "resolved_at": now}).Error; e != nil {
				return e
			}
			// Вернуть в 'active' ТОЛЬКО договоры, которые ЭТИ debt-строки приостановили
			// (AffectedContractIDs) — не топчем приостановленные по др. причине (Codex HIGH-3).
			// Fallback per-suspension (Codex HIGH): строка без affected-списка → её собственный
			// contract_id (cp-строка с ContractID=0 → req.ContractID). Иначе из N снятых suspension
			// восстановился бы только req.ContractID, остальные остались бы suspended в CRM.
			affected := parseAffectedContractIDs(s.AffectedContractIDs)
			if len(affected) == 0 {
				if s.ContractID != 0 {
					affected = []uint{s.ContractID}
				} else {
					affected = []uint{req.ContractID}
				}
			}
			for _, id := range affected {
				restoreSet[id] = true
			}
		}
		if len(restoreSet) > 0 {
			restoreIDs := make([]uint, 0, len(restoreSet))
			for id := range restoreSet {
				restoreIDs = append(restoreIDs, id)
			}
			// UPDATE условный по 'suspended' — не трогаем ручной cancel/expired/draft.
			if e := tx.Table(contractsTable).Where("id IN ? AND status = ?", restoreIDs, "suspended").
				Update("status", "active").Error; e != nil {
				return e
			}
		}
		snappedSusps = susps
		return nil
	})
	if txErr == errHoldExists {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "по этому договору уже есть активный зонт"})
		return
	}
	// Гонка: partial-unique idx_hold_active → unique_violation (23505) → 409, не 500 (Codex #5).
	var pgErr *pq.Error
	if errors.As(txErr, &pgErr) && pgErr.Code == "23505" {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "по этому договору уже есть активный зонт"})
		return
	}
	if txErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": txErr.Error()})
		return
	}

	// Б3.5 §3.7b: зонт = отсрочка → снять и ФИЗИЧЕСКУЮ блокировку учёток (раньше PostLedgerHold
	// снимал только CRM-статус, физблок в Axenta оставался → зонт был фикцией на физ-уровне).
	// После коммита, как RESTORE-путь sweep. heldByOther внутри Reactivate защищает учётку,
	// которую держит ДРУГАЯ активная suspension (общий Axenta-аккаунт у нескольких неоплат).
	if enforcementSvc != nil {
		for i := range snappedSusps {
			enforcementSvc.Reactivate(snappedSusps[i])
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": hold})
}

// GetLedgerHolds — GET /api/auth/ledger/holds/:contract_id
// Активные зонты + история (последние 200) по договору.
func GetLedgerHolds(c *gin.Context) {
	contractID, err := strconv.ParseUint(c.Param("contract_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный contract_id"})
		return
	}
	if !managerCanAccessContract(c, uint(contractID)) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "договор вне вашего доступа"})
		return
	}
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	// Ф3: зонты ЕДИНИЦЫ — контрагента договора (cp<>0) либо самого договора (cp=0, legacy).
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.DB
	}
	cpID, companyID, _, _ := contractLedgerKeys(tenantDB, uint(contractID))
	q := database.DB.Where("admin_account_id = ? AND deleted_at IS NULL", adminAccountID)
	if companyID != 0 {
		q = q.Where("company_id = ?", companyID) // Codex HIGH-5: не течь между компаниями admin
	}
	if cpID != 0 {
		q = q.Where("counterparty_id = ?", cpID)
	} else {
		q = q.Where("counterparty_id = 0 AND contract_id = ?", uint(contractID))
	}
	var holds []models.BillingHold
	q.Order("active DESC, created_at DESC").Limit(200).Find(&holds)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": holds})
}

// parseAffectedContractIDs — разбор CSV BillingSuspension.AffectedContractIDs (Ф3).
func parseAffectedContractIDs(s string) []uint {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []uint
	for _, p := range strings.Split(s, ",") {
		if v, e := strconv.ParseUint(strings.TrimSpace(p), 10, 64); e == nil && v > 0 {
			out = append(out, uint(v))
		}
	}
	return out
}

// PostLedgerHoldCancel — POST /api/auth/ledger/hold/:id/cancel
// Отмена активного зонта оператором (status=cancelled). Договор НЕ блокируем тут же —
// следующий sweep решит по балансу (если долг остался — приостановит).
func PostLedgerHoldCancel(c *gin.Context) {
	holdID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный id"})
		return
	}
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	var hold models.BillingHold
	if err := database.DB.Where("id = ? AND admin_account_id = ? AND deleted_at IS NULL", uint(holdID), adminAccountID).
		First(&hold).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "зонт не найден"})
		return
	}
	if !managerCanAccessHold(c, hold) { // cp-зонт (ContractID=0) → доступ по контрагенту (live-flip gate)
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "зонт вне вашего доступа"})
		return
	}
	if !requireBillingOperation(c, adminAccountID, hold.CompanyID) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "операция недоступна для вашей роли (см. политику биллинга)"})
		return
	}
	if !hold.Active {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "зонт уже неактивен"})
		return
	}
	now := time.Now().UTC()
	if err := database.DB.Table("public.billing_holds").Where("id = ? AND active = ?", hold.ID, true).
		Updates(map[string]interface{}{"status": "cancelled", "active": false, "resolved_at": now}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"id": hold.ID, "status": "cancelled"}})
}

// --- pure helpers (тестируемы без БД) ---

// holdUntilExclusive — из выбранного дня (YYYY-MM-DD) делает EXCLUSIVE-границу зонта:
// начало СЛЕДУЮЩЕГО дня в UTC. Sweep держит зонт пока now < hold_until, поэтому
// выбранный день покрыт целиком (включительно), а UTC+N (KZ/RU) — внутри окна.
func holdUntilExclusive(holdDay time.Time) time.Time {
	return time.Date(holdDay.Year(), holdDay.Month(), holdDay.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
}

// deferralWithinPolicy — укладывается ли exclusive-граница holdUntil в лимит
// MaxDeferralDays от now (включительно по дню). maxDays<=0 трактуется как «нельзя»
// вызывающим до этой проверки; здесь сравниваем границы.
func deferralWithinPolicy(holdUntil, now time.Time, maxDays int) bool {
	nowDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	maxUntil := nowDay.AddDate(0, 0, maxDays+1) // +1: exclusive-граница последнего разрешённого дня
	return !holdUntil.After(maxUntil)
}
