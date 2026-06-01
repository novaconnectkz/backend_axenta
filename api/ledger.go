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

// contractCurrency — валюта проводки = валюта договора, с fallback на RUB для
// пустого значения (legacy-договоры без явной Currency). Единая точка fallback:
// charge уже пишет валюту договора (см. chargeSubscription), payment/import/hold
// раньше хардкодили "RUB" — валюто-слепой баланс. Подробности — concepts/multicurrency.
func contractCurrency(ccy string) string {
	if ccy == "" {
		return "RUB"
	}
	return ccy
}

// ── Ф2: баланс per-контрагент (единый лицевой счёт) ─────────────────────────────
// Один контрагент = N договоров = ОДИН баланс = SUM(amount) по counterparty_id.
// charge остаётся per-договор (ContractID), но баланс/платёж/лимит — per-контрагент.

// counterpartyBalance — баланс контрагента: SUM(amount) по ВСЕМ его проводкам.
// company_id обязателен: admin_account_id в legacy-sync общий для компаний, а
// counterparty_id уникален лишь в рамках (admin, company).
func counterpartyBalance(counterpartyID, adminAccountID, companyID uint) decimal.Decimal {
	var sum decimal.Decimal
	database.DB.Model(&models.LedgerEntry{}).
		Where("counterparty_id = ? AND admin_account_id = ? AND company_id = ? AND deleted_at IS NULL",
			counterpartyID, adminAccountID, companyID).
		Select("COALESCE(SUM(amount), 0)").Scan(&sum)
	return sum
}

// balanceForContract — баланс лицевого счёта, относящийся к договору:
//   - есть контрагент (counterparty_id<>0) → единый баланс контрагента (все его договоры);
//   - иначе (партнёр/немигрированный) → legacy баланс самого договора (per-договор).
// Так Ф2 forward-correct, но не ломает контракты вне модели контрагентов (cp=0).
func balanceForContract(contractID, counterpartyID, adminAccountID, companyID uint) decimal.Decimal {
	if counterpartyID != 0 {
		return counterpartyBalance(counterpartyID, adminAccountID, companyID)
	}
	var sum decimal.Decimal
	database.DB.Model(&models.LedgerEntry{}).
		Where("contract_id = ? AND admin_account_id = ? AND company_id = ? AND deleted_at IS NULL",
			contractID, adminAccountID, companyID).
		Select("COALESCE(SUM(amount), 0)").Scan(&sum)
	return sum
}

// balanceForContractCcy — суб-баланс лицевого счёта в КОНКРЕТНОЙ валюте.
// Как balanceForContract, но с фильтром currency=ccy: для money-path ответов
// (new_balance/from_balance/to_balance), где смешивать валюты нельзя (валюто-слепой
// SUM схлопнул бы USD-charge и RUB-payment). cp<>0 → по контрагенту, иначе по договору.
func balanceForContractCcy(contractID, counterpartyID, adminAccountID, companyID uint, ccy string) decimal.Decimal {
	ccy = contractCurrency(ccy)
	q := database.DB.Model(&models.LedgerEntry{}).
		Where("admin_account_id = ? AND company_id = ? AND currency = ? AND deleted_at IS NULL",
			adminAccountID, companyID, ccy)
	if counterpartyID != 0 {
		q = q.Where("counterparty_id = ?", counterpartyID)
	} else {
		q = q.Where("contract_id = ?", contractID)
	}
	var sum decimal.Decimal
	q.Select("COALESCE(SUM(amount), 0)").Scan(&sum)
	return sum
}

// CurrencyBreakdown — суб-баланс единицы (контрагент/договор) в одной валюте.
type CurrencyBreakdown struct {
	Currency string          `json:"currency"`
	Charged  decimal.Decimal `json:"charged"`
	Paid     decimal.Decimal `json:"paid"`
	Balance  decimal.Decimal `json:"balance"` // paid - charged
}

// ledgerBreakdownByCurrency — баланс единицы, разложенный по валютам (GROUP BY currency).
// Один договор одновалютен; мультивалюта возникает только у контрагента с договорами
// в разных валютах. Источник для презентации и для валюто-корректного решения о блокировке.
func ledgerBreakdownByCurrency(contractID, counterpartyID, adminAccountID, companyID uint) []CurrencyBreakdown {
	var rows []struct {
		Currency string
		Charged  decimal.Decimal
		Paid     decimal.Decimal
	}
	q := database.DB.Model(&models.LedgerEntry{}).
		Where("admin_account_id = ? AND company_id = ? AND deleted_at IS NULL", adminAccountID, companyID)
	if counterpartyID != 0 {
		q = q.Where("counterparty_id = ?", counterpartyID)
	} else {
		q = q.Where("contract_id = ?", contractID)
	}
	q.Select("currency, COALESCE(SUM(CASE WHEN amount < 0 THEN -amount ELSE 0 END),0) AS charged, " +
		"COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END),0) AS paid").
		Group("currency").Scan(&rows)
	out := make([]CurrencyBreakdown, 0, len(rows))
	for _, r := range rows {
		out = append(out, CurrencyBreakdown{Currency: r.Currency, Charged: r.Charged, Paid: r.Paid, Balance: r.Paid.Sub(r.Charged)})
	}
	return out
}

// contractLedgerKeys загружает (counterparty_id, company_id, currency) договора из
// tenant-схемы — для резолва per-контрагент баланса и валюты презентации в хендлерах,
// у которых на входе contract_id.
func contractLedgerKeys(tenantDB *gorm.DB, contractID uint) (counterpartyID, companyID uint, currency string, ok bool) {
	var ct models.Contract
	if err := tenantDB.Select("id, counterparty_id, company_id, currency").First(&ct, contractID).Error; err != nil {
		return 0, 0, "", false
	}
	return ct.CounterpartyID, ct.CompanyID, ct.Currency, true
}

// presentBalance — презентационная свёртка суб-балансов в одну валюту по курсу на
// СЕГОДНЯ (для UI). Возвращает (0,false), если для какой-то валюты курс отсутствует
// или устарел — тогда вызывающий показывает суб-баланс валюты договора, НЕ обнуляя.
// ВАЖНО: презентация НИКОГДА не участвует в решении о блокировке (там — без конверсии).
func presentBalance(subs []CurrencyBreakdown, targetCcy string, companyID uint) (decimal.Decimal, bool) {
	targetCcy = contractCurrency(targetCcy)
	rateSource := "cbr_rf"
	var bs models.BillingSettings
	if database.DB.Table("public.billing_settings").Where("company_id = ?", companyID).
		Select("rate_source").First(&bs).Error == nil && bs.RateSource != "" {
		rateSource = bs.RateSource
	}
	total := decimal.Zero
	for _, sb := range subs {
		ccy := contractCurrency(sb.Currency)
		if ccy == targetCcy {
			total = total.Add(sb.Balance)
			continue
		}
		r, _, stale, err := currencyRateService().GetConversionRate(time.Now().UTC(), ccy, targetCcy, rateSource)
		if err != nil || stale || r.LessThanOrEqual(decimal.Zero) {
			return decimal.Zero, false
		}
		total = total.Add(sb.Balance.Mul(r))
	}
	return total.Round(2), true
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
	// Ф2: баланс/разбивка per-контрагент. Резолвим counterparty_id+company_id договора;
	// cp<>0 → агрегируем по контрагенту (все его договоры), иначе legacy per-договор.
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.DB
	}
	cpID, companyID, ccy, _ := contractLedgerKeys(tenantDB, uint(contractID))
	charged, paid := ledgerBreakdown(uint(contractID), cpID, adminAccountID, companyID)
	balance := paid.Sub(charged)
	data := gin.H{
		"contract_id":     contractID,
		"counterparty_id": cpID, // 0 = вне модели контрагентов (баланс per-договор)
		"total_charged":   charged.StringFixed(2),
		"total_paid":      paid.StringFixed(2),
	}
	// Мультивалюта: balance — презентационная свёртка по курсу (или суб-баланс валюты
	// договора при stale-курсе). Одновалютный случай (прод сегодня) идёт прежним путём —
	// balance=paid−charged, новые поля не добавляются → FE-контракт не меняется.
	subs := ledgerBreakdownByCurrency(uint(contractID), cpID, adminAccountID, companyID)
	if len(subs) > 1 {
		ccy = contractCurrency(ccy)
		data["multicurrency"] = true
		data["sub_balances"] = subs
		data["presentation_currency"] = ccy
		if pres, ok := presentBalance(subs, ccy, companyID); ok {
			balance = pres
			data["presentation_only"] = true
		} else {
			balance = decimal.Zero
			for _, s := range subs {
				if contractCurrency(s.Currency) == ccy {
					balance = s.Balance
				}
			}
		}
	}
	debtAmount := decimal.Zero
	if balance.IsNegative() {
		debtAmount = balance.Abs()
	}
	data["balance"] = balance.StringFixed(2)       // >0 переплата, <0 долг
	data["is_debt"] = balance.IsNegative()
	data["debt_amount"] = debtAmount.StringFixed(2) // долг только если balance<0, иначе 0
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}

// ledgerBreakdown — (начислено, оплачено) лицевого счёта. cp<>0 → по контрагенту
// (все договоры), иначе по договору. companyID обязателен для скоупа.
func ledgerBreakdown(contractID, counterpartyID, adminAccountID, companyID uint) (charged, paid decimal.Decimal) {
	var agg struct {
		Charged decimal.Decimal
		Paid    decimal.Decimal
	}
	q := database.DB.Model(&models.LedgerEntry{}).
		Where("admin_account_id = ? AND company_id = ? AND deleted_at IS NULL", adminAccountID, companyID)
	if counterpartyID != 0 {
		q = q.Where("counterparty_id = ?", counterpartyID)
	} else {
		q = q.Where("contract_id = ?", contractID)
	}
	q.Select("COALESCE(SUM(CASE WHEN amount < 0 THEN -amount ELSE 0 END),0) AS charged, COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END),0) AS paid").
		Scan(&agg)
	return agg.Charged, agg.Paid
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
	// Ф2: история per-контрагент (все договоры контрагента); cp=0 → legacy per-договор.
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.DB
	}
	cpID, companyID, _, _ := contractLedgerKeys(tenantDB, uint(contractID))
	q := database.DB.Where("admin_account_id = ? AND company_id = ? AND deleted_at IS NULL", adminAccountID, companyID)
	if cpID != 0 {
		q = q.Where("counterparty_id = ?", cpID)
	} else {
		q = q.Where("contract_id = ?", uint(contractID))
	}
	var entries []models.LedgerEntry
	q.Order("entry_date DESC, id DESC").Limit(2000).Find(&entries)
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
	if err := tenantDB.Select("id, company_id, counterparty_id, currency").First(&contract, req.ContractID).Error; err != nil {
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
		CounterpartyID: contract.CounterpartyID, // Ф2: денорм для баланса per-контрагент
		EntryType:      "payment",
		Amount:         decimal.NewFromFloat(req.Amount), // платёж +
		Currency:       contractCurrency(contract.Currency),
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
		"new_balance": balanceForContractCcy(req.ContractID, contract.CounterpartyID, adminAccountID, contract.CompanyID, contract.Currency).StringFixed(2),
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
		if err := tenantDB.Select("id, company_id, counterparty_id, currency").First(&contract, it.ContractID).Error; err != nil {
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
			CounterpartyID: contract.CounterpartyID, // Ф2: денорм
			EntryType:      "payment",
			Amount:         decimal.NewFromFloat(it.Amount),
			Currency:       contractCurrency(contract.Currency),
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
	if err := tenantDB.Select("id, currency, company_id, counterparty_id").First(&from, req.FromContractID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "договор-источник не найден"})
		return
	}
	if err := tenantDB.Select("id, currency, company_id, counterparty_id").First(&to, req.ToContractID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "договор-получатель не найден"})
		return
	}
	// Ф2: единый ЛС — перевод внутри ОДНОГО контрагента бессмыслен (один баланс).
	if from.CounterpartyID != 0 && from.CounterpartyID == to.CounterpartyID {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "договоры принадлежат одному контрагенту — у них общий лицевой счёт"})
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
				"from_balance": balanceForContractCcy(req.FromContractID, from.CounterpartyID, adminAccountID, from.CompanyID, fromCcy).StringFixed(2),
				"to_balance":   balanceForContractCcy(req.ToContractID, to.CounterpartyID, adminAccountID, to.CompanyID, toCcy).StringFixed(2),
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
	// Ф2: lock + проверка средств — на лицевом счёте ИСТОЧНИКА-контрагента (единый баланс).
	// cp=0 → legacy per-договор. Lock на той же сущности, что и баланс (сериализация переводов
	// между договорами одного контрагента).
	srcLockID := from.CounterpartyID
	if srcLockID == 0 {
		srcLockID = req.FromContractID
	}
	lockKey := transferLockKey(adminAccountID, srcLockID)

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		// Анти-double-spend: advisory-lock на источник + пересчёт баланса ВНУТРИ tx.
		if e := tx.Exec("SELECT pg_advisory_xact_lock(?)", lockKey).Error; e != nil {
			return e
		}
		var bal decimal.Decimal
		// Проверка средств — в валюте СПИСАНИЯ (fromCcy): amount тоже в fromCcy.
		// Без фильтра currency суб-баланс другой валюты ложно «покрыл» бы перевод.
		balQ := tx.Model(&models.LedgerEntry{}).
			Where("admin_account_id = ? AND company_id = ? AND currency = ? AND deleted_at IS NULL", adminAccountID, from.CompanyID, fromCcy)
		if from.CounterpartyID != 0 {
			balQ = balQ.Where("counterparty_id = ?", from.CounterpartyID)
		} else {
			balQ = balQ.Where("contract_id = ?", req.FromContractID)
		}
		balQ.Select("COALESCE(SUM(amount),0)").Scan(&bal)
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
		// Списание у источника — в валюте источника (amount). Стампуем counterparty_id (Ф2).
		out := models.LedgerEntry{
			AdminAccountID: adminAccountID, CompanyID: from.CompanyID, ContractID: req.FromContractID,
			CounterpartyID: from.CounterpartyID,
			EntryType:      "transfer_out", Amount: amount.Neg(), Currency: fromCcy, Source: "transfer",
			ExternalID: "transfer:" + transferID + ":out", Description: desc, EntryDate: now,
			CreatedBy: createdBy, Metadata: &metaOut,
		}
		if err := tx.Create(&out).Error; err != nil {
			return err
		}
		// Зачисление получателю — в его валюте (toAmount), инвариант ledger.ccy==contract.ccy.
		in := models.LedgerEntry{
			AdminAccountID: adminAccountID, CompanyID: to.CompanyID, ContractID: req.ToContractID,
			CounterpartyID: to.CounterpartyID,
			EntryType:      "transfer_in", Amount: toAmount, Currency: toCcy, Source: "transfer",
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
		"from_balance": balanceForContractCcy(req.FromContractID, from.CounterpartyID, adminAccountID, from.CompanyID, fromCcy).StringFixed(2),
		"to_balance":   balanceForContractCcy(req.ToContractID, to.CounterpartyID, adminAccountID, to.CompanyID, toCcy).StringFixed(2),
	}})
}

// transferLockKey — стабильный int64 для pg_advisory_xact_lock (admin+contract).
func transferLockKey(adminAccountID, contractID uint) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(fmt.Sprintf("ledger_transfer:%d:%d", adminAccountID, contractID)))
	return int64(h.Sum64() >> 1)
}
