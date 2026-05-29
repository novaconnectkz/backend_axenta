package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ledgerChargeScheduler — глобальный экземпляр для ручного триггера.
var ledgerChargeScheduler *services.LedgerChargeScheduler

// currencyRateService — сервис курсов для кросс-валютного перевода (П5 фаза 3).
// Ленивая инициализация: package-level var выполнился бы ДО подключения database.DB
// в main (NewCurrencyRateService захватывает database.DB) → svc.db=nil → nil deref.
func currencyRateService() *services.CurrencyRateService {
	return services.NewCurrencyRateService()
}

// SetLedgerChargeScheduler регистрирует scheduler (вызывается из main).
func SetLedgerChargeScheduler(s *services.LedgerChargeScheduler) {
	ledgerChargeScheduler = s
}

// PostLedgerChargeRun — POST /api/auth/ledger/charge/run
// Ручной прогон авто-начисления (для теста). Body опц.: {"date":"YYYY-MM-DD"}
// — начислить за все недостающие дни до этой даты включительно (default вчера).
func PostLedgerChargeRun(c *gin.Context) {
	// Глобальный прогон начислений — только admin/superadmin.
	if !requireContractAssignAccess(c) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "доступно только администратору"})
		return
	}
	if ledgerChargeScheduler == nil {
		// Планировщик может быть выключен флагом — поднимаем разовый экземпляр.
		ledgerChargeScheduler = services.NewLedgerChargeScheduler()
	}
	var req struct {
		Date string `json:"date"`
	}
	_ = c.ShouldBindJSON(&req)
	target := time.Now().UTC().AddDate(0, 0, -1)
	if req.Date != "" {
		if t, e := time.Parse("2006-01-02", req.Date); e == nil {
			target = t.UTC()
		}
	}
	ledgerChargeScheduler.RunUpToDate(target)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": ledgerChargeScheduler.GetStatus()})
}

// ============================================================================
// Лицевой счёт (ledger). Баланс договора = SUM(ledger_entries.amount).
// payment(+)/charge(−). balance>0 переплата, balance<0 долг.
// Проводки иммутабельны (правка — reversal-проводкой).
// ============================================================================

// ledgerBalance считает баланс договора как сумму проводок (источник правды).
// admin_account_id обязателен: ledger в public, contract_id из разных tenant
// могут совпадать — без admin-фильтра балансы смешались бы между компаниями.
func ledgerBalance(contractID, adminAccountID uint) decimal.Decimal {
	var sum decimal.Decimal
	database.DB.Model(&models.LedgerEntry{}).
		Where("contract_id = ? AND admin_account_id = ? AND deleted_at IS NULL", contractID, adminAccountID).
		Select("COALESCE(SUM(amount), 0)").Scan(&sum)
	return sum
}

// GetLedgerBalance — GET /api/auth/ledger/balance/:contract_id
// Баланс договора + разбивка (начислено/оплачено).
func GetLedgerBalance(c *gin.Context) {
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
	var agg struct {
		Charged decimal.Decimal
		Paid    decimal.Decimal
	}
	database.DB.Model(&models.LedgerEntry{}).
		Where("contract_id = ? AND admin_account_id = ? AND deleted_at IS NULL", uint(contractID), adminAccountID).
		Select("COALESCE(SUM(CASE WHEN amount < 0 THEN -amount ELSE 0 END),0) AS charged, COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END),0) AS paid").
		Scan(&agg)
	balance := agg.Paid.Sub(agg.Charged)
	debtAmount := decimal.Zero
	if balance.IsNegative() {
		debtAmount = balance.Abs()
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
		"contract_id":   contractID,
		"balance":       balance.StringFixed(2), // >0 переплата, <0 долг
		"total_charged": agg.Charged.StringFixed(2),
		"total_paid":    agg.Paid.StringFixed(2),
		"is_debt":       balance.IsNegative(),
		"debt_amount":   debtAmount.StringFixed(2), // долг только если balance<0, иначе 0
	}})
}

// GetLedgerEntries — GET /api/auth/ledger/entries/:contract_id
// История проводок договора (дата, тип, сумма, за что, источник).
func GetLedgerEntries(c *gin.Context) {
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
	var entries []models.LedgerEntry
	database.DB.Where("contract_id = ? AND admin_account_id = ? AND deleted_at IS NULL", uint(contractID), adminAccountID).
		Order("entry_date DESC, id DESC").Limit(2000).Find(&entries)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": entries})
}

type ledgerPaymentRequest struct {
	ContractID  uint    `json:"contract_id" binding:"required"`
	Amount      float64 `json:"amount" binding:"required"` // положительная сумма платежа
	Source      string  `json:"source"`                    // manual|excel|1c|payment_system|bank
	Comment     string  `json:"comment"`
	PaymentDate string  `json:"payment_date"` // YYYY-MM-DD, опц.
	ExternalID  string  `json:"external_id"`  // для идемпотентности импорта
}

// PostLedgerPayment — POST /api/auth/ledger/payment
// Платёж на лицевой счёт договора БЕЗ выставления счёта.
func PostLedgerPayment(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	var req ledgerPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректные данные платежа"})
		return
	}
	if req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "сумма платежа должна быть > 0"})
		return
	}
	if !managerCanAccessContract(c, req.ContractID) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "договор вне вашего доступа"})
		return
	}
	if req.Source == "" {
		req.Source = "manual"
	}

	// company_id берём из договора (tenant-схема).
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.DB
	}
	var contract models.Contract
	if err := tenantDB.Select("id, company_id").First(&contract, req.ContractID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "договор не найден"})
		return
	}

	entryDate := time.Now().UTC()
	if req.PaymentDate != "" {
		if t, e := time.Parse("2006-01-02", req.PaymentDate); e == nil {
			entryDate = t
		}
	}
	username, _ := c.Get("username")
	createdBy, _ := username.(string)

	entry := models.LedgerEntry{
		AdminAccountID: adminAccountID,
		CompanyID:      contract.CompanyID,
		ContractID:     req.ContractID,
		EntryType:      "payment",
		Amount:         decimal.NewFromFloat(req.Amount), // платёж +
		Currency:       "RUB",
		Source:         req.Source,
		ExternalID:     req.ExternalID,
		Description:    req.Comment,
		EntryDate:      entryDate,
		CreatedBy:      createdBy,
	}

	// Транзакция + идемпотентность по external_id (если задан).
	txErr := database.DB.Transaction(func(tx *gorm.DB) error {
		if req.ExternalID != "" {
			var cnt int64
			tx.Model(&models.LedgerEntry{}).
				Where("admin_account_id = ? AND company_id = ? AND source = ? AND external_id = ? AND deleted_at IS NULL",
					adminAccountID, contract.CompanyID, req.Source, req.ExternalID).Count(&cnt)
			if cnt > 0 {
				return errLedgerDuplicate
			}
		}
		return tx.Create(&entry).Error
	})
	if txErr == errLedgerDuplicate {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "платёж с таким external_id уже внесён"})
		return
	}
	if txErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": txErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
		"entry_id":    entry.ID,
		"new_balance": ledgerBalance(req.ContractID, adminAccountID).StringFixed(2),
	}})
}

type ledgerImportItem struct {
	ContractID  uint    `json:"contract_id"`
	Amount      float64 `json:"amount"`
	PaymentDate string  `json:"payment_date"`
	ExternalID  string  `json:"external_id"`
	Comment     string  `json:"comment"`
}

// PostLedgerImport — POST /api/auth/ledger/import
// Массовый импорт платежей (Excel-реестр → распарсенный массив на FE).
// source проставляется в зависимости от происхождения (excel по умолчанию).
func PostLedgerImport(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	var body struct {
		Source string             `json:"source"`
		Items  []ledgerImportItem `json:"items" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный реестр"})
		return
	}
	if body.Source == "" {
		body.Source = "excel"
	}
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.DB
	}
	username, _ := c.Get("username")
	createdBy, _ := username.(string)

	inserted, skipped, failed := 0, 0, 0
	errorsList := make([]string, 0)
	for _, it := range body.Items {
		if it.Amount <= 0 || it.ContractID == 0 {
			failed++
			continue
		}
		// Scoping менеджера: нельзя импортировать платёж на чужой договор.
		if !managerCanAccessContract(c, it.ContractID) {
			failed++
			errorsList = append(errorsList, fmt.Sprintf("договор %d вне доступа", it.ContractID))
			continue
		}
		var contract models.Contract
		if err := tenantDB.Select("id, company_id").First(&contract, it.ContractID).Error; err != nil {
			failed++
			errorsList = append(errorsList, fmt.Sprintf("договор %d не найден", it.ContractID))
			continue
		}
		entryDate := time.Now().UTC()
		if it.PaymentDate != "" {
			if t, e := time.Parse("2006-01-02", it.PaymentDate); e == nil {
				entryDate = t
			}
		}
		entry := models.LedgerEntry{
			AdminAccountID: adminAccountID,
			CompanyID:      contract.CompanyID,
			ContractID:     it.ContractID,
			EntryType:      "payment",
			Amount:         decimal.NewFromFloat(it.Amount),
			Currency:       "RUB",
			Source:         body.Source,
			ExternalID:     it.ExternalID,
			Description:    it.Comment,
			EntryDate:      entryDate,
			CreatedBy:      createdBy,
		}
		txErr := database.DB.Transaction(func(tx *gorm.DB) error {
			if it.ExternalID != "" {
				var cnt int64
				tx.Model(&models.LedgerEntry{}).
					Where("admin_account_id = ? AND company_id = ? AND source = ? AND external_id = ? AND deleted_at IS NULL",
						adminAccountID, contract.CompanyID, body.Source, it.ExternalID).Count(&cnt)
				if cnt > 0 {
					return errLedgerDuplicate
				}
			}
			return tx.Create(&entry).Error
		})
		switch txErr {
		case nil:
			inserted++
		case errLedgerDuplicate:
			skipped++
		default:
			failed++
			errorsList = append(errorsList, txErr.Error())
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
		"inserted": inserted, "skipped": skipped, "failed": failed, "errors": errorsList,
	}})
}

var errLedgerDuplicate = fmt.Errorf("ledger duplicate external_id")

type ledgerTransferRequest struct {
	FromContractID uint    `json:"from_contract_id" binding:"required"`
	ToContractID   uint    `json:"to_contract_id" binding:"required"`
	Amount         float64 `json:"amount" binding:"required"` // положительная сумма
	Description    string  `json:"description"`
	IdempotencyKey string  `json:"idempotency_key"` // против дублей retry
}

var errTransferInsufficient = fmt.Errorf("insufficient funds")

// PostLedgerTransfer — POST /api/auth/ledger/transfer
// Перевод средств с лицевого счёта одного договора на другой. Атомарно: заголовок
// LedgerTransfer + пара проводок (transfer_out у from, transfer_in у to) в одной tx.
func PostLedgerTransfer(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	var req ledgerTransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректные данные перевода"})
		return
	}
	if req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "сумма перевода должна быть > 0"})
		return
	}
	if req.FromContractID == req.ToContractID {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "договор-источник и получатель совпадают"})
		return
	}
	// Scoping менеджера: доступ к ОБОИМ договорам.
	if !managerCanAccessContract(c, req.FromContractID) || !managerCanAccessContract(c, req.ToContractID) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "договор вне вашего доступа"})
		return
	}

	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.DB
	}
	var from, to models.Contract
	if err := tenantDB.Select("id, currency, company_id").First(&from, req.FromContractID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "договор-источник не найден"})
		return
	}
	if err := tenantDB.Select("id, currency, company_id").First(&to, req.ToContractID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "договор-получатель не найден"})
		return
	}
	fromCcy, toCcy := from.Currency, to.Currency
	if fromCcy == "" {
		fromCcy = "RUB"
	}
	if toCcy == "" {
		toCcy = "RUB"
	}

	// Право на операцию по политике компании (OperationRoleThreshold).
	if !requireBillingOperation(c, adminAccountID, from.CompanyID) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "перевод недоступен для вашей роли (см. политику биллинга)"})
		return
	}

	amount := decimal.NewFromFloat(req.Amount).Round(2)
	if amount.LessThanOrEqual(decimal.Zero) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "сумма перевода должна быть > 0"})
		return
	}

	// Идемпотентность ДО конверсии (Codex #1): повтор уже созданного перевода должен
	// вернуть прежний результат независимо от текущих курсов (иначе retry упадёт, если
	// курс сегодня пропал/устарел). Проверяем сразу после загрузки договоров.
	if req.IdempotencyKey != "" {
		var ex models.LedgerTransfer
		if database.DB.Where("admin_account_id = ? AND idempotency_key = ?", adminAccountID, req.IdempotencyKey).First(&ex).Error == nil {
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
				"transfer_id":  ex.TransferID,
				"from_balance": ledgerBalance(req.FromContractID, adminAccountID).StringFixed(2),
				"to_balance":   ledgerBalance(req.ToContractID, adminAccountID).StringFixed(2),
				"idempotent":   true,
			}})
			return
		}
	}

	// Кросс-валютный перевод (П5 фаза 3): сумма списания у источника (fromCcy) →
	// конверсия по курсу на сегодня → зачисление получателю (toCcy). Курс фиксируем
	// в header перевода. Одна валюта → rate=1, toAmount=amount.
	transferRate := decimal.NewFromInt(1)
	toAmount := amount
	transferRateSource := ""
	if fromCcy != toCcy {
		rateSource := "cbr_rf"
		var bs models.BillingSettings
		if database.DB.Table("public.billing_settings").
			Where("company_id = ?", from.CompanyID).Select("rate_source").First(&bs).Error == nil && bs.RateSource != "" {
			rateSource = bs.RateSource
		}
		r, _, stale, rerr := currencyRateService().GetConversionRate(time.Now().UTC(), fromCcy, toCcy, rateSource)
		if rerr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "нет курса для конверсии " + fromCcy + "→" + toCcy + ": " + rerr.Error()})
			return
		}
		if r.LessThanOrEqual(decimal.Zero) {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный курс конверсии"})
			return
		}
		if stale {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "курс конверсии устарел, перевод отклонён — обновите курсы"})
			return
		}
		transferRate = r
		transferRateSource = rateSource
		toAmount = amount.Mul(r).Round(2)
		if toAmount.LessThanOrEqual(decimal.Zero) {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "сумма после конверсии нулевая"})
			return
		}
	}

	username, _ := c.Get("username")
	createdBy, _ := username.(string)
	transferID := uuid.New().String()
	now := time.Now().UTC()
	// Metadata пары проводок через json.Marshal — валюты/числа не ломают jsonb
	// спецсимволами (Codex #2). Для кросс-валюты фиксируем курс/встречную сумму (аудит).
	metaOutBytes, _ := json.Marshal(map[string]interface{}{
		"transfer_id":              transferID,
		"counterparty_contract_id": req.ToContractID,
		"rate":                     transferRate.String(),
		"to_amount":                toAmount.StringFixed(2),
		"to_ccy":                   toCcy,
	})
	metaInBytes, _ := json.Marshal(map[string]interface{}{
		"transfer_id":              transferID,
		"counterparty_contract_id": req.FromContractID,
		"rate":                     transferRate.String(),
		"from_amount":              amount.StringFixed(2),
		"from_ccy":                 fromCcy,
	})
	metaOut := string(metaOutBytes)
	metaIn := string(metaInBytes)
	desc := req.Description
	if desc == "" {
		desc = "Перевод между лицевыми счетами"
	}
	lockKey := transferLockKey(adminAccountID, req.FromContractID)

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		// Анти-double-spend: advisory-lock на источник + пересчёт баланса ВНУТРИ tx.
		if e := tx.Exec("SELECT pg_advisory_xact_lock(?)", lockKey).Error; e != nil {
			return e
		}
		var bal decimal.Decimal
		tx.Model(&models.LedgerEntry{}).
			Where("contract_id = ? AND admin_account_id = ? AND deleted_at IS NULL", req.FromContractID, adminAccountID).
			Select("COALESCE(SUM(amount),0)").Scan(&bal)
		if bal.LessThan(amount) {
			return errTransferInsufficient
		}
		hdr := models.LedgerTransfer{
			AdminAccountID: adminAccountID, CompanyID: from.CompanyID,
			TransferID: transferID, IdempotencyKey: req.IdempotencyKey, FromContractID: req.FromContractID, ToContractID: req.ToContractID,
			Amount: amount, Currency: fromCcy, ToAmount: toAmount, ToCurrency: toCcy, Rate: transferRate, RateSource: transferRateSource,
			Status: "completed", Description: desc, CreatedBy: createdBy,
		}
		if err := tx.Create(&hdr).Error; err != nil {
			return err
		}
		// Списание у источника — в валюте источника (amount).
		out := models.LedgerEntry{
			AdminAccountID: adminAccountID, CompanyID: from.CompanyID, ContractID: req.FromContractID,
			EntryType: "transfer_out", Amount: amount.Neg(), Currency: fromCcy, Source: "transfer",
			ExternalID: "transfer:" + transferID + ":out", Description: desc, EntryDate: now,
			CreatedBy: createdBy, Metadata: &metaOut,
		}
		if err := tx.Create(&out).Error; err != nil {
			return err
		}
		// Зачисление получателю — в его валюте (toAmount), инвариант ledger.ccy==contract.ccy.
		in := models.LedgerEntry{
			AdminAccountID: adminAccountID, CompanyID: to.CompanyID, ContractID: req.ToContractID,
			EntryType: "transfer_in", Amount: toAmount, Currency: toCcy, Source: "transfer",
			ExternalID: "transfer:" + transferID + ":in", Description: desc, EntryDate: now,
			CreatedBy: createdBy, Metadata: &metaIn,
		}
		return tx.Create(&in).Error
	}); err != nil {
		if err == errTransferInsufficient {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "недостаточно средств на лицевом счёте источника"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "ошибка перевода: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
		"transfer_id":  transferID,
		"from_balance": ledgerBalance(req.FromContractID, adminAccountID).StringFixed(2),
		"to_balance":   ledgerBalance(req.ToContractID, adminAccountID).StringFixed(2),
	}})
}

// transferLockKey — стабильный int64 для pg_advisory_xact_lock (admin+contract).
func transferLockKey(adminAccountID, contractID uint) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(fmt.Sprintf("ledger_transfer:%d:%d", adminAccountID, contractID)))
	return int64(h.Sum64() >> 1)
}
