package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// LedgerChargeScheduler — ежедневное авто-начисление (charge) по клиентским
// договорам в лицевой счёт (ledger). Этап B1.
//
// Модель: ledger = accrual-баланс. Каждый день начисляется charge за состав
// объектов ИМЕННО ТОГО дня (исторически), а не «как сейчас» — это убирает
// перекос при изменении объектов задним числом и делает повторный прогон
// идемпотентным (тот же день → тот же состав → тот же external_id → скип).
//
// Дневная сумма подписки = price × кол-во активных объектов / дней_в_периоде.
// monthly → дней в календарном месяце дня; yearly → дней в году.
//
// Известные ограничения v1 (см. wiki/concepts/billing-ledger.md):
//   - скидки (Contract.discount_*) и НДС НЕ применяются — charge = price×objects;
//   - цена плана берётся текущая (нет pricing history) — безопасно, т.к. cutoff
//     делает окно backfill коротким;
//   - ContractObject.status историчен только по created_at/deleted_at + привязке
//     start_date/end_date (сам столбец status не версионируется).
type LedgerChargeScheduler struct {
	cron        *cron.Cron
	db          *gorm.DB
	rateSvc     *CurrencyRateService // конверсия валют при начислении (П5 фаза 2)
	enforcement *EnforcementService  // B1: физическая блокировка учётки за неоплату (nil → no-op)
	isRunning   bool
	lastRun     time.Time
}

// SetEnforcement подключает B1-enforcement (вызывается из main.go одним общим
// AxentaServerToken). nil/не вызван → хуки enforcement — no-op.
func (s *LedgerChargeScheduler) SetEnforcement(e *EnforcementService) { s.enforcement = e }

// advisory-lock ключ, чтобы два прогона (cron + ручной триггер) не пересеклись.
const ledgerChargeLockKey int64 = 778811

func NewLedgerChargeScheduler() *LedgerChargeScheduler {
	c := cron.New(
		cron.WithLocation(time.UTC),
		cron.WithChain(cron.Recover(cron.DefaultLogger)),
	)
	return &LedgerChargeScheduler{cron: c, db: database.DB, rateSvc: NewCurrencyRateService()}
}

// Start запускает ежедневный прогон в 02:00 UTC (после snapshot-планировщиков).
func (s *LedgerChargeScheduler) Start() error {
	_, err := s.cron.AddFunc("0 2 * * *", func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в LedgerChargeScheduler: %v", r)
			}
		}()
		// Цель — вчерашний день (полный завершённый день в UTC).
		yesterday := time.Now().UTC().AddDate(0, 0, -1)
		s.RunUpToDate(dayFloor(yesterday))
	})
	if err != nil {
		return err
	}
	s.cron.Start()
	s.isRunning = true
	log.Println("✅ LedgerChargeScheduler запущен (ежедневно 02:00 UTC)")
	return nil
}

func (s *LedgerChargeScheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
		s.isRunning = false
		log.Println("🛑 LedgerChargeScheduler остановлен")
	}
}

// cutoffDate — самая ранняя дата, за которую вообще разрешено начислять.
// Защита от backfill за годы при первом запуске на проде.
// env LEDGER_AUTO_CHARGE_START=YYYY-MM-DD; если не задан → вчера (без истории).
func (s *LedgerChargeScheduler) cutoffDate() time.Time {
	if v := os.Getenv("LEDGER_AUTO_CHARGE_START"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			return dayFloor(t.UTC())
		}
		log.Printf("⚠️ LEDGER_AUTO_CHARGE_START=%q не распарсился (нужен YYYY-MM-DD), игнорирую", v)
	}
	return dayFloor(time.Now().UTC().AddDate(0, 0, -1))
}

// RunUpToDate — начислить за все недостающие дни до targetDate включительно.
// Catch-up идёт per-subscription: от max(cutoff, sub.start, lastCharge+1) до targetDate.
func (s *LedgerChargeScheduler) RunUpToDate(targetDate time.Time) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ ПАНИКА в RunUpToDate: %v", r)
		}
	}()
	targetDate = dayFloor(targetDate)
	// Защита от future-backfill: не начисляем за сегодня и будущее (день ещё не закрыт).
	yesterday := dayFloor(time.Now().UTC().AddDate(0, 0, -1))
	if targetDate.After(yesterday) {
		targetDate = yesterday
	}
	cutoff := s.cutoffDate()

	ctx := context.Background()
	sqlDB, err := s.db.DB()
	if err != nil {
		log.Printf("❌ LedgerCharge: не удалось получить *sql.DB: %v", err)
		return
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		log.Printf("❌ LedgerCharge: не удалось взять соединение: %v", err)
		return
	}
	defer conn.Close()

	var locked bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", ledgerChargeLockKey).Scan(&locked); err != nil {
		log.Printf("❌ LedgerCharge: ошибка advisory-lock: %v", err)
		return
	}
	if !locked {
		log.Println("⏭️ LedgerCharge: другой прогон уже идёт (advisory-lock занят), пропускаю")
		return
	}
	defer conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", ledgerChargeLockKey)

	start := time.Now()
	log.Printf("💸 LedgerCharge: старт начисления до %s (cutoff %s)", targetDate.Format("2006-01-02"), cutoff.Format("2006-01-02"))

	var companies []models.Company
	if err := s.db.Session(&gorm.Session{}).Table("public.companies").
		Where("is_active = ?", true).Find(&companies).Error; err != nil {
		log.Printf("❌ LedgerCharge: ошибка получения компаний: %v", err)
		return
	}

	totalEntries, totalSkipped := 0, 0
	for _, company := range companies {
		e, sk := s.chargeCompany(company, cutoff, targetDate)
		totalEntries += e
		totalSkipped += sk
	}

	s.lastRun = time.Now()
	log.Printf("✅ LedgerCharge: готово за %v — компаний=%d, проводок=%d, пропущено(дубли)=%d",
		time.Since(start), len(companies), totalEntries, totalSkipped)

	// После начисления — sweep приостановок за долг (П2, фаза A: только CRM-статус).
	s.RunSuspensionSweep(companies)
}

// RunSuspensionSweep — по всем активным компаниям проверяет клиентские договоры:
// balance < −credit_limit → приостановить (billing_suspension reason=billing_debt +
// Contract.Status='suspended'); долг погашен → снять приостановку и вернуть прежний статус.
// Фаза A: меняем только CRM-статус (без блокировки у провайдера).
func (s *LedgerChargeScheduler) RunSuspensionSweep(companies []models.Company) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ ПАНИКА в RunSuspensionSweep: %v", r)
		}
	}()
	if len(companies) == 0 {
		_ = s.db.Session(&gorm.Session{}).Table("public.companies").
			Where("is_active = ?", true).Find(&companies).Error
	}
	suspended, resolved := 0, 0
	for _, company := range companies {
		su, re := s.sweepCompanySuspensions(company)
		suspended += su
		resolved += re
	}
	if suspended > 0 || resolved > 0 {
		log.Printf("🔒 SuspensionSweep: приостановлено=%d, снято=%d", suspended, resolved)
	}
	// B1: дотянуть недо-выполненные физблоки (physical_ok=false) вне транзита active→suspended.
	if s.enforcement != nil {
		s.enforcement.ReassertPending()
	}
}

// joinUintCSV / parseUintCSV — сериализация списка id договоров для BillingSuspension.AffectedContractIDs.
func joinUintCSV(ids []uint) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	return strings.Join(parts, ",")
}

func parseUintCSV(s string) []uint {
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

func (s *LedgerChargeScheduler) sweepCompanySuspensions(company models.Company) (suspended, resolved int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ ПАНИКА в sweepCompanySuspensions(%d): %v", company.ID, r)
		}
	}()
	schema := company.DatabaseSchema
	if schema == "" {
		schema = fmt.Sprintf("tenant_%d", company.ID)
	}
	tenantDB, err := database.ConnectToTenant(schema)
	if err != nil {
		return
	}
	// Схема-квалифицированное имя contracts: статусы договоров обновляем ЧЕРЕЗ tx (public-
	// соединение), а не tenantDB — иначе UPDATE в другой транзакции и атомарность с suspension
	// теряется (tenantDB = отдельное соединение ConnectToTenant). Codex HIGH-1.
	contractsTable := fmt.Sprintf(`"%s".contracts`, schema)

	// Клиентские договоры в активном/приостановленном статусе.
	var contracts []models.Contract
	// Order("id") — детерминизм: в чистом Б2 sweep группирует per-cp и берёт лимит ПЕРВОГО
	// договора группы; без явного порядка Find отдаёт строки произвольно → threshold скакал бы
	// между договорами с разными лимитами (Codex HIGH). Per-договор порог — Б4.
	if err := tenantDB.Where("contract_type = ? AND status IN ? AND deleted_at IS NULL",
		"client", []string{"active", "suspended"}).Order("id").Find(&contracts).Error; err != nil {
		return
	}
	pub := s.db.Session(&gorm.Session{})
	now := time.Now().UTC()

	// Активные holds компании (Ф3: per-контрагент или legacy per-договор). Индексируем
	// двояко: cp<>0 → по (admin, counterparty_id); cp=0 → по (admin, contract_id).
	// admin в ключе обязателен: id может совпадать у разных админов одной company (Codex #3).
	type unitKey struct {
		adminID uint
		id      uint // counterparty_id (cp-unit) или contract_id (legacy)
	}
	holdByCP := make(map[unitKey]models.BillingHold)
	holdByContract := make(map[unitKey]models.BillingHold)
	{
		var holds []models.BillingHold
		pub.Table("public.billing_holds").
			Where("company_id = ? AND active = ? AND deleted_at IS NULL", company.ID, true).
			Find(&holds)
		for _, h := range holds {
			if h.CounterpartyID != 0 {
				holdByCP[unitKey{h.AdminAccountID, h.CounterpartyID}] = h
			} else {
				holdByContract[unitKey{h.AdminAccountID, h.ContractID}] = h
			}
		}
	}

	// Ф3: единица приостановки — КОНТРАГЕНТ (cp<>0, гасит ВСЕ его client-договоры одним
	// решением) либо отдельный договор (cp=0, legacy). Группируем client-договоры в единицы.
	type billingUnit struct {
		adminID      uint
		cpID         uint            // 0 → legacy per-договор
		contractID   uint            // legacy contract_id (cp=0); 0 для контрагента
		creditLimit  decimal.Decimal // Б2: лимит с договора (cp-группа — лимит первого по id)
		activeIDs    []uint          // client-договоры в 'active' (кандидаты на suspend)
		suspendedIDs []uint          // client-договоры в 'suspended' (кандидаты на restore)
	}
	unitByCP := make(map[unitKey]*billingUnit)
	units := make([]*billingUnit, 0, len(contracts))
	for i := range contracts {
		ct := &contracts[i]
		if ct.CounterpartyID != 0 {
			k := unitKey{ct.AdminAccountID, ct.CounterpartyID}
			u := unitByCP[k]
			if u == nil {
				// Б2: лимит группы = лимит ПЕРВОГО договора по id (Order("id") выше даёт детерминизм).
				// Per-договор порог появится в Б4; до него вся cp-группа гасится одним threshold.
				u = &billingUnit{adminID: ct.AdminAccountID, cpID: ct.CounterpartyID, creditLimit: ct.CreditLimit}
				unitByCP[k] = u
				units = append(units, u)
			}
			if ct.Status == "active" {
				u.activeIDs = append(u.activeIDs, ct.ID)
			} else {
				u.suspendedIDs = append(u.suspendedIDs, ct.ID)
			}
		} else {
			u := &billingUnit{adminID: ct.AdminAccountID, contractID: ct.ID, creditLimit: ct.CreditLimit}
			if ct.Status == "active" {
				u.activeIDs = []uint{ct.ID}
			} else {
				u.suspendedIDs = []uint{ct.ID}
			}
			units = append(units, u)
		}
	}

	for _, u := range units {
		// Б2: лимит авторитетен на Contract (зеркало на контрагента снято). В чистом Б2 sweep
		// ещё группирует per-cp → единый лимит группы = лимит договора-юнита (per-договор порог — Б4).
		creditLimit := u.creditLimit
		threshold := creditLimit.Neg() // допустимый минус: balance < −creditLimit → блок (в КАЖДОЙ валюте)

		// Per-currency суб-балансы единицы (СТРОГО в рамках company+admin, Codex critical).
		// Валюто-слепой SUM схлопывал бы USD-charge и RUB-payment — блокировка считается
		// по каждой валюте отдельно, БЕЗ конверсии (курс не должен (раз)блокировать).
		var subRows []struct {
			Currency string
			Bal      decimal.Decimal
		}
		balQ := pub.Table("public.ledger_entries").
			Where("admin_account_id = ? AND company_id = ? AND deleted_at IS NULL", u.adminID, company.ID)
		if u.cpID != 0 {
			balQ = balQ.Where("counterparty_id = ?", u.cpID)
		} else {
			balQ = balQ.Where("contract_id = ?", u.contractID)
		}
		balQ.Select("currency, COALESCE(SUM(amount),0) AS bal").Group("currency").Scan(&subRows)

		// Единица в долге ⇔ хотя бы один суб-баланс < −creditLimit. worstBal — самый
		// глубокий минус (для отчётного DebtAmount и lifecycle зонта). inDebt считается
		// ОДИН раз — иначе зонт и suspend разъедутся (один на bal, другой на per-currency).
		inDebt := false
		worstBal := decimal.Zero
		for _, sr := range subRows {
			if sr.Bal.LessThan(threshold) {
				inDebt = true
				if sr.Bal.LessThan(worstBal) {
					worstBal = sr.Bal
				}
			}
		}

		// Lifecycle зонта (П3/П4) ДО решения о приостановке. Решение — pure holdLifecycleAction.
		var hold models.BillingHold
		var hasHold bool
		if u.cpID != 0 {
			hold, hasHold = holdByCP[unitKey{u.adminID, u.cpID}]
		} else {
			hold, hasHold = holdByContract[unitKey{u.adminID, u.contractID}]
		}
		holdActive := false
		if hasHold {
			switch holdLifecycleActionMulti(inDebt, worstBal, threshold, hold.HoldUntil, now) {
			case holdFulfill:
				// Atomic WHERE active=true; ошибку логируем (не глотаем — Codex #7).
				if e := pub.Table("public.billing_holds").Where("id = ? AND active = ?", hold.ID, true).
					Updates(map[string]interface{}{"status": "fulfilled", "active": false, "resolved_at": now}).Error; e != nil {
					log.Printf("⚠️ SuspensionSweep: не удалось закрыть hold %d (fulfilled): %v", hold.ID, e)
				}
			case holdExpire:
				// Блокируем дальше ТОЛЬКО если expire реально снял active (RowsAffected>0, Codex #7).
				res := pub.Table("public.billing_holds").Where("id = ? AND active = ?", hold.ID, true).
					Updates(map[string]interface{}{"status": "expired", "active": false, "resolved_at": now})
				if res.Error != nil {
					log.Printf("⚠️ SuspensionSweep: не удалось закрыть hold %d (expired): %v", hold.ID, res.Error)
					holdActive = true
				} else if res.RowsAffected == 0 {
					holdActive = true
				}
			default:
				holdActive = true // зонт ещё держит — приостановку не делаем
			}
		}

		// Активная debt-приостановка единицы (cp-уровень или legacy per-договор).
		suspQ := pub.Table("public.billing_suspensions").
			Where("admin_account_id = ? AND company_id = ? AND reason = ? AND active = ? AND deleted_at IS NULL",
				u.adminID, company.ID, "billing_debt", true)
		if u.cpID != 0 {
			suspQ = suspQ.Where("counterparty_id = ?", u.cpID)
		} else {
			suspQ = suspQ.Where("counterparty_id = 0 AND contract_id = ?", u.contractID)
		}
		var active models.BillingSuspension
		hasActive := suspQ.First(&active).Error == nil

		switch {
		case inDebt && !hasActive && !holdActive && len(u.activeIDs) > 0:
			// Долг → приостановить ВСЕ активные договоры единицы (единый ЛС). Одна suspension-строка
			// (per-контрагент или legacy) + статусы договоров — в одной tx (атомарность, Codex H1).
			susp := models.BillingSuspension{
				AdminAccountID: u.adminID, CompanyID: company.ID, ContractID: u.contractID, CounterpartyID: u.cpID,
				Reason: "billing_debt", PreviousStatus: "active", DebtAmount: worstBal.Abs(),
				Active: true, SuspendedBy: "scheduler",
				AffectedContractIDs: joinUintCSV(u.activeIDs), // точный restore при resolve
			}
			err := pub.Transaction(func(tx *gorm.DB) error {
				if e := tx.Table("public.billing_suspensions").Create(&susp).Error; e != nil {
					return e // partial-unique поймает гонку → дубль не создастся
				}
				// Статусы — через tx (schema-qualified), в ТОЙ ЖЕ транзакции (атомарность, Codex HIGH-1).
				return tx.Table(contractsTable).
					Where("id IN ? AND status = ?", u.activeIDs, "active").Update("status", "suspended").Error
			})
			if err == nil {
				suspended += len(u.activeIDs)
				if s.enforcement != nil {
					s.enforcement.Enforce(susp, tenantDB) // B1: физблок (shadow/live); не возвращает error — физика не откатывает CRM-статус
				}
			}
		case !inDebt && hasActive:
			// Долг погашен → снять приостановку + вернуть в 'active' ТОЛЬКО договоры, которые
			// ИМЕННО ЭТА строка приостановила (AffectedContractIDs) — не трогаем приостановленные
			// по др. причине (нет подписок/ручной bulk). Codex HIGH-2.
			restoreIDs := parseUintCSV(active.AffectedContractIDs)
			if len(restoreIDs) == 0 {
				restoreIDs = u.suspendedIDs // fallback для старых строк без affected-списка
			}
			err := pub.Transaction(func(tx *gorm.DB) error {
				if e := tx.Table("public.billing_suspensions").Where("id = ?", active.ID).
					Updates(map[string]interface{}{"active": false, "resolved_at": now}).Error; e != nil {
					return e
				}
				if len(restoreIDs) == 0 {
					return nil
				}
				return tx.Table(contractsTable).
					Where("id IN ? AND status = ?", restoreIDs, "suspended").Update("status", "active").Error
			})
			if err == nil {
				resolved += len(restoreIDs)
				if s.enforcement != nil {
					s.enforcement.Reactivate(active) // B1: физразблок наших block-строк
				}
			}
		}
	}
	return
}

// routeCadence — П6: эффективная каденция подписки и выбор charge-пути.
// Override каденции тарифа (planCadence) перебивает глобальную каденцию компании
// (companyCadence) — это Ф4. Возвращает "lump" | "monthly" | "daily".
func routeCadence(companyCadence, planCadence, planPeriod, longSubCharge string) string {
	cadence := companyCadence
	if pc := strings.TrimSpace(planCadence); pc != "" {
		cadence = pc // Ф4: override тарифа
	}
	switch {
	case cadence == "period" && planPeriod == "yearly" && longSubCharge == "lump_sum":
		return "lump"
	case cadence == "monthly" || cadence == "period":
		return "monthly"
	default:
		return "daily"
	}
}

// chargeCompany начисляет по всем клиентским подпискам одной компании.
func (s *LedgerChargeScheduler) chargeCompany(company models.Company, cutoff, targetDate time.Time) (entries, skipped int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ ПАНИКА в chargeCompany(%d): %v", company.ID, r)
		}
	}()

	// Подписки лежат в public. Берём активные с привязкой к договору.
	// Known limitation v1: отменённую/истёкшую за прошедшие сутки подписку
	// (status уже не 'active') не доберём за последний незакрытый день —
	// потеря ≤1 дня, для accrual несущественно. Отвязка объекта (status→inactive,
	// subscription_id→NULL) тоже искажает backfill прошлых дней; cutoff держит
	// окно backfill коротким, поэтому риск мал.
	var subs []models.Subscription
	if err := s.db.Session(&gorm.Session{}).Table("public.subscriptions").
		Preload("BillingPlan").
		Where("company_id = ? AND status = ? AND contract_id IS NOT NULL", company.ID, "active").
		Find(&subs).Error; err != nil {
		log.Printf("⚠️ LedgerCharge: компания %d — ошибка загрузки подписок: %v", company.ID, err)
		return
	}
	if len(subs) == 0 {
		return
	}

	// Отдельный per-tenant пул (search_path в DSN) — не мутируем общий пул в фоне.
	schema := company.DatabaseSchema
	if schema == "" {
		schema = fmt.Sprintf("tenant_%d", company.ID)
	}
	tenantDB, err := database.ConnectToTenant(schema)
	if err != nil {
		log.Printf("⚠️ LedgerCharge: компания %d — нет tenant-подключения: %v", company.ID, err)
		return
	}

	// Политика биллинга компании (П5 источник курса + П6 каденция/мин.дни/длинные подписки).
	rateSource := "cbr_rf"
	companyCadence := "daily"  // П6: daily | monthly | period
	minDaysFull := 5           // мин. дней присутствия объекта в месяце для полного месяца
	longSubCharge := "monthly" // П6: monthly | lump_sum — как списывать подписки >1 мес
	{
		var bs models.BillingSettings
		if s.db.Session(&gorm.Session{}).Table("public.billing_settings").
			Where("company_id = ?", company.ID).
			Select("rate_source, charge_cadence, min_days_for_full_month, long_subscription_charge").First(&bs).Error == nil {
			if bs.RateSource != "" {
				rateSource = bs.RateSource
			}
			if bs.ChargeCadence != "" {
				companyCadence = bs.ChargeCadence
			}
			if bs.MinDaysForFullMonth > 0 {
				minDaysFull = bs.MinDaysForFullMonth
			}
			if bs.LongSubscriptionCharge != "" {
				longSubCharge = bs.LongSubscriptionCharge
			}
		}
	}

	// Кэш проверенных договоров: только client + active билятся.
	// currency держим для конверсии валют (П5); billingMode — для тайминга месячной
	// каденции (П6: prepaid → 1-е тек. месяца, postpaid → 1-е след.).
	type ctInfo struct {
		ok             bool
		currency       string
		billingMode    string
		counterpartyID uint // Ф2: денорм для стамповки проводок (баланс per-контрагент)
	}
	contractInfo := make(map[uint]ctInfo)
	// Кэш курсов на прогон компании: (day|base|quote|source) → результат GetRate.
	// Backfill бьёт одни и те же пары/даты по многим подпискам (Codex #6).
	rateCache := make(map[string]cachedRate)
	for _, sub := range subs {
		if sub.ContractID == nil {
			continue
		}
		cid := *sub.ContractID
		info, checked := contractInfo[cid]
		if !checked {
			var ct models.Contract
			if e := tenantDB.Select("id, contract_type, status, currency, billing_mode, counterparty_id, admin_account_id").First(&ct, cid).Error; e != nil {
				info = ctInfo{ok: false}
			} else {
				ccy := ct.Currency
				if ccy == "" {
					ccy = "RUB"
				}
				// Б2: billing_mode авторитетен на Contract (зеркало на контрагента снято).
				billingMode := ct.BillingMode
				info = ctInfo{ok: ct.ContractType == "client" && ct.Status == "active", currency: ccy, billingMode: billingMode, counterpartyID: ct.CounterpartyID}
			}
			contractInfo[cid] = info
		}
		if !info.ok {
			continue
		}

		// П6 Ф4: эффективная каденция + выбор charge-пути (override тарифа > глобал).
		var e, sk int
		switch routeCadence(companyCadence, sub.BillingPlan.ChargeCadence, sub.BillingPlan.BillingPeriod, longSubCharge) {
		case "lump":
			// Длинная подписка разово: вся сумма периода 1 проводкой (П6 Ф3).
			e, sk = s.chargeSubscriptionPeriodLump(tenantDB, company.ID, &sub, cid, info.counterpartyID, info.currency, info.billingMode, rateSource, minDaysFull, rateCache, cutoff, targetDate)
		case "monthly":
			// monthly, либо period+помесячно (в т.ч. yearly→price/12), либо period на
			// месячном тарифе — единое месячное начисление.
			e, sk = s.chargeSubscriptionMonthly(tenantDB, company.ID, &sub, cid, info.counterpartyID, info.currency, info.billingMode, rateSource, minDaysFull, rateCache, cutoff, targetDate)
		default:
			// daily — посуточное начисление как раньше.
			e, sk = s.chargeSubscription(tenantDB, company.ID, &sub, cid, info.counterpartyID, info.currency, rateSource, rateCache, cutoff, targetDate)
		}
		entries += e
		skipped += sk
	}
	return
}

// chargeSubscription начисляет дневные charge'и по одной подписке за период.
// contractCcy — валюта договора (ledger пишется в ней, инвариант Codex [H]).
// rateSource — источник курса для конверсии plan.Currency → contractCcy (П5).
func (s *LedgerChargeScheduler) chargeSubscription(tenantDB *gorm.DB, companyID uint, sub *models.Subscription, contractID, counterpartyID uint, contractCcy, rateSource string, rateCache map[string]cachedRate, cutoff, targetDate time.Time) (entries, skipped int) {
	price := sub.BillingPlan.Price
	if price.IsZero() {
		return
	}
	// Валюта плана (в чём задана цена). Пустая → RUB.
	planCcy := sub.BillingPlan.Currency
	if planCcy == "" {
		planCcy = "RUB"
	}
	if contractCcy == "" {
		contractCcy = "RUB"
	}

	// Нижняя граница периода: max(cutoff, sub.start, lastCharge+1).
	startDay := cutoff
	if subStart := dayFloor(sub.StartDate.UTC()); subStart.After(startDay) {
		startDay = subStart
	}
	// Checkpoint daily-пути: MAX(entry_date) ТОЛЬКО по дневным проводкам. Месячные/годовые
	// проводки игнорируем — их entry_date «убегает» вперёд (postpaid monthly за июнь имеет
	// дату 1 июля), и общий MAX перепрыгнул бы непокрытый дневной день (Codex Ф5 #1). При
	// свежей смене monthly/lump→daily дневных нет → checkpoint nil → старт от cutoff, а
	// покрытые месяцы дёшево скипаются coveredMonths-гардом ниже.
	if last := s.lastDailyChargeDate(sub.ID); last != nil {
		next := last.AddDate(0, 0, 1)
		if next.After(startDay) {
			startDay = next
		}
	}

	yearly := sub.BillingPlan.BillingPeriod == "yearly"

	// П6 Ф5 (forward-only): месяцы, уже покрытые месячной/годовой проводкой прежней
	// каденции. При смене monthly/lump→daily пропускаем их дни — иначе месяц, оплаченный
	// одной проводкой, списался бы повторно посуточно (латентный двойной счёт).
	coveredMonths, cErr := s.monthsCoveredByLongerCadence(sub.AdminAccountID, companyID, sub.ID)
	if cErr != nil {
		log.Printf("⚠️ LedgerCharge: sub %d — ошибка проверки покрытия каденции, стоп подписки: %v", sub.ID, cErr)
		return
	}

	for day := startDay; !day.After(targetDate); day = day.AddDate(0, 0, 1) {
		// Подписка должна быть активна в этот день.
		if dayFloor(sub.StartDate.UTC()).After(day) {
			continue
		}
		if sub.EndDate != nil && dayFloor(sub.EndDate.UTC()).Before(day) {
			continue
		}
		// Месяц уже покрыт более длинной каденцией (Ф5) → не дублируем посуточно.
		if coveredMonths[day.Format("2006-01")] {
			continue
		}

		count, err := s.objectCountOnDay(tenantDB, sub.ID, day)
		if err != nil {
			// Ошибка БД — НЕ трактуем как 0 объектов. Останавливаем подписку,
			// чтобы checkpoint не уехал за непросчитанный день (ретрай в след. прогон).
			log.Printf("⚠️ LedgerCharge: sub %d день %s — ошибка подсчёта объектов, стоп подписки: %v",
				sub.ID, day.Format("2006-01-02"), err)
			return
		}
		if count == 0 {
			continue
		}

		// Конверсия валюты (П5): daily в валюте плана → валюту договора по курсу на
		// ДЕНЬ начисления (historical, как состав объектов). ledger всегда в валюте
		// договора (инвариант Codex [H]). Снимок курса в Metadata — для аудита/rebill.
		var chargeAmount decimal.Decimal
		var metaPtr *string
		if planCcy == contractCcy {
			// Same-currency: округляем дневную сумму до 2 знаков (как раньше).
			chargeAmount = dailyChargeAmount(price, count, day, yearly)
		} else {
			rate, rateDate, stale, rerr := s.getRateCached(rateCache, day, planCcy, contractCcy, rateSource)
			if rerr != nil {
				// Курса на день нет → НЕ начисляем по rate=1. Стоп подписки: checkpoint
				// не уедет за непросчитанный день, доберём когда курс появится (Codex [M]).
				log.Printf("⚠️ LedgerCharge: sub %d день %s — нет курса %s→%s (%s), стоп подписки: %v",
					sub.ID, day.Format("2006-01-02"), planCcy, contractCcy, rateSource, rerr)
				return
			}
			// rate ≤ 0 — невозможный курс: rate=0 потерял бы день, rate<0 дал бы кредит
			// вместо charge. Стоп подписки до исправления курса (Codex #1 critical).
			if rate.LessThanOrEqual(decimal.Zero) {
				log.Printf("❌ LedgerCharge: sub %d день %s — некорректный курс %s→%s = %s, стоп подписки",
					sub.ID, day.Format("2006-01-02"), planCcy, contractCcy, rate.String())
				return
			}
			// Stale-курс (fallback старше порога) НЕ постим: ждём свежий курс, чтобы не
			// фиксировать неточную сумму в иммутабельной проводке (Codex #3). Доберём позже.
			if stale {
				log.Printf("⚠️ LedgerCharge: sub %d день %s — курс %s→%s устарел (rate_date=%s), стоп подписки до свежего курса",
					sub.ID, day.Format("2006-01-02"), planCcy, contractCcy, rateDate.Format("2006-01-02"))
				return
			}
			// RAW дневная сумма (без предв. округления) → конверсия → единственный round (Codex #2).
			rawDaily := dailyChargeAmountRaw(price, count, day, yearly)
			chargeAmount = convertAmount(rawDaily, rate)
			// Metadata через json.Marshal — без ручной сборки строки (Codex #4: source/ccy
			// могли бы сломать jsonb спецсимволами).
			mb, mErr := json.Marshal(map[string]string{
				"rate":            rate.String(),
				"rate_date":       rateDate.Format("2006-01-02"),
				"source":          rateSource,
				"original_amount": rawDaily.Round(2).String(),
				"original_ccy":    planCcy,
			})
			if mErr != nil {
				log.Printf("⚠️ LedgerCharge: sub %d день %s — ошибка сборки metadata, стоп подписки: %v",
					sub.ID, day.Format("2006-01-02"), mErr)
				return
			}
			ms := string(mb)
			metaPtr = &ms
		}
		if chargeAmount.IsZero() {
			continue
		}

		extID := fmt.Sprintf("autocharge:sub:%d:%s", sub.ID, day.Format("2006-01-02"))
		subID := sub.ID
		entry := models.LedgerEntry{
			AdminAccountID: sub.AdminAccountID,
			CompanyID:      companyID,
			ContractID:     contractID,
			CounterpartyID: counterpartyID, // Ф2: денорм для баланса per-контрагент
			SubscriptionID: &subID,
			EntryType:      "charge",
			Amount:         chargeAmount.Neg(), // charge < 0, в валюте договора
			Currency:       contractCcy,
			Source:         "auto_charge",
			ExternalID:     extID,
			Description:    fmt.Sprintf("Начисление за %s (%d объектов)", day.Format("2006-01-02"), count),
			EntryDate:      day,
			CreatedBy:      "scheduler",
			Metadata:       metaPtr,
		}

		// Идемпотентность: уникальный (admin, company, source, external_id).
		txErr := s.db.Transaction(func(tx *gorm.DB) error {
			var cnt int64
			if e := tx.Model(&models.LedgerEntry{}).
				Where("admin_account_id = ? AND company_id = ? AND source = ? AND external_id = ? AND deleted_at IS NULL",
					sub.AdminAccountID, companyID, "auto_charge", extID).Count(&cnt).Error; e != nil {
				return e
			}
			if cnt > 0 {
				return errChargeExists
			}
			return tx.Create(&entry).Error
		})
		switch txErr {
		case nil:
			entries++
		case errChargeExists:
			skipped++
		default:
			// Сбой записи дня — стоп подписки, checkpoint не уезжает за непроведённый
			// день (ретрай в след. прогон). Иначе MAX(entry_date) перепрыгнет дыру.
			log.Printf("⚠️ LedgerCharge: sub %d день %s — ошибка записи, стоп подписки: %v",
				sub.ID, day.Format("2006-01-02"), txErr)
			return
		}
	}
	return
}

// monthFloor — первое число месяца даты (00:00 UTC).
func monthFloor(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// monthlyPlanAmount — месячная сумма плана за 1 объект: yearly → price/12, иначе price.
func monthlyPlanAmount(price decimal.Decimal, billingPeriod string) decimal.Decimal {
	if billingPeriod == "yearly" {
		return price.Div(decimal.NewFromInt(12))
	}
	return price
}

// objectsPresentInMonthGE — кол-во объектов подписки с присутствием ≥ minDays дней в
// месяце [monthStart, monthStart+1мес). Присутствие = пересечение интервала привязки
// объекта [start_date, end_date] с месяцем. Бинарно: ≥minDays → полный месяц (П6).
// окне [windowStart, windowEnd). Присутствие = пересечение интервала привязки объекта
// [start_date, end_date] с окном. Бинарно: ≥minDays → засчитываем (П6).
// windowEnd УЖЕ ограничен днём наблюдения вызывающим (today+1) — для текущего (ещё не
// завершённого) периода считаем только прошедшие дни (иначе prepaid на 1-е считал бы
// будущее присутствие и переначислял бы за объект, ушедший раньше minDays — Codex c).
// Окно может быть месяцем (monthly) или годом (yearly lump). Window-based вместо
// календарного месяца чинит late-start: подписка с 29-го числа набирает minDays в окне
// периода, а не упирается в 3 дня анкор-месяца (Codex H4).
// Возвращает error — вызывающий обязан трактовать как «не знаем» и остановить подписку.
func (s *LedgerChargeScheduler) objectsPresentInWindowGE(tenantDB *gorm.DB, subID uint, windowStart, windowEnd time.Time, minDays int) (int, error) {
	var objs []models.ContractObject
	// Unscoped: историчность держим сами (deleted_at) — объект, удалённый позже окна,
	// считается за дни ДО удаления.
	err := tenantDB.Unscoped().
		Where("subscription_id = ? AND status = ?", subID, "active").
		Where("start_date < ?", windowEnd).
		Where("(end_date IS NULL OR end_date >= ?)", windowStart).
		Where("(deleted_at IS NULL OR deleted_at >= ?)", windowStart).
		Find(&objs).Error
	if err != nil {
		return 0, err
	}
	cnt := 0
	for _, o := range objs {
		// Окно присутствия объекта, ограниченное windowStart/windowEnd.
		ps := windowStart
		if st := dayFloor(o.StartDate.UTC()); st.After(ps) {
			ps = st
		}
		pe := windowEnd
		if o.EndDate != nil {
			// end_date включительно → +1 день.
			if e := dayFloor(o.EndDate.UTC()).AddDate(0, 0, 1); e.Before(pe) {
				pe = e
			}
		}
		if o.DeletedAt.Valid {
			if d := dayFloor(o.DeletedAt.Time.UTC()); d.Before(pe) {
				pe = d
			}
		}
		if pe.After(ps) {
			days := int(pe.Sub(ps).Hours() / 24)
			if days >= minDays {
				cnt++
			}
		}
	}
	return cnt, nil
}

// capWindowEnd — верхняя граница окна = min(естественный конец периода, день наблюдения).
func capWindowEnd(periodEnd, observedEnd time.Time) time.Time {
	if observedEnd.Before(periodEnd) {
		return observedEnd
	}
	return periodEnd
}

// periodCoveredByShorterCadence — есть ли за период [periodStart, periodEnd) хоть одна
// ДНЕВНАЯ или МЕСЯЧНАЯ auto_charge проводка (от прежней каденции). Матч по external_id:
// monthly `:YYYY-MM` и daily `:YYYY-MM-DD` оба начинаются с `:YYYY-MM`, годовой `:y:` не
// матчится. Проверяем по external_id (логическое покрытие), а НЕ по entry_date — postpaid
// проводка за декабрь имеет entry_date=1 янв и выпала бы из окна периода (Codex C1).
func (s *LedgerChargeScheduler) periodCoveredByShorterCadence(adminAccountID, companyID, subID uint, periodStart, periodEnd time.Time) (bool, error) {
	conds := make([]string, 0, 12)
	args := make([]interface{}, 0, 12)
	for mm := periodStart; mm.Before(periodEnd); mm = mm.AddDate(0, 1, 0) {
		conds = append(conds, "external_id LIKE ?")
		args = append(args, fmt.Sprintf("autocharge:sub:%d:%s%%", subID, mm.Format("2006-01")))
	}
	if len(conds) == 0 {
		return false, nil
	}
	var cnt int64
	if err := s.db.Model(&models.LedgerEntry{}).
		Where("admin_account_id = ? AND company_id = ? AND source = ? AND deleted_at IS NULL",
			adminAccountID, companyID, "auto_charge").
		Where("("+strings.Join(conds, " OR ")+")", args...).
		Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// monthsCoveredByLongerCadence — множество месяцев (ключ "2006-01") этой подписки, уже
// покрытых БОЛЕЕ длинной каденцией: месячной проводкой (:YYYY-MM) или годовым lump
// (:y:YYYY-MM, покрывает 12 месяцев периода). П6 Ф5 (forward-only): нужно daily-пути,
// чтобы при смене каденции monthly/lump→daily НЕ списать повторно дни месяца, уже
// оплаченного одной проводкой (зеркало гардов a/C1/C2, которых на daily-пути не было →
// латентный двойной счёт). SQL портативен PG+SQLite: `NOT LIKE '..____-__-%'` отсекает
// дневные external_id (YYYY-MM-DD), оставляя monthly (YYYY-MM) и lump (y:YYYY-MM).
func (s *LedgerChargeScheduler) monthsCoveredByLongerCadence(adminAccountID, companyID, subID uint) (map[string]bool, error) {
	prefix := fmt.Sprintf("autocharge:sub:%d:", subID)
	var ids []string
	if err := s.db.Model(&models.LedgerEntry{}).
		Where("admin_account_id = ? AND company_id = ? AND source = ? AND subscription_id = ? AND deleted_at IS NULL",
			adminAccountID, companyID, "auto_charge", subID).
		Where("external_id LIKE ?", prefix+"%").
		Where("external_id NOT LIKE ?", prefix+"____-__-%").
		Pluck("external_id", &ids).Error; err != nil {
		return nil, err
	}
	covered := make(map[string]bool, len(ids))
	for _, id := range ids {
		suf := strings.TrimPrefix(id, prefix)
		if strings.HasPrefix(suf, "y:") {
			// Годовой lump: 12 месяцев от начала периода.
			if t, e := time.Parse("2006-01", strings.TrimPrefix(suf, "y:")); e == nil {
				for k := 0; k < 12; k++ {
					covered[t.AddDate(0, k, 0).Format("2006-01")] = true
				}
			}
			continue
		}
		// Месячная :YYYY-MM (ровно 7 символов, парсится как 2006-01).
		if _, e := time.Parse("2006-01", suf); e == nil {
			covered[suf] = true
		}
	}
	return covered, nil
}

// chargeSubscriptionMonthly — П6 monthly-каденция: ОДНО начисление за месяц на день
// биллинга (prepaid → 1-е тек. месяца, postpaid → 1-е след.). Сумма = полная месячная
// цена × (объекты с присутствием ≥ minDays). Идемпотентность per-month (external_id
// :YYYY-MM). cutoff ограничивает backfill снизу.
func (s *LedgerChargeScheduler) chargeSubscriptionMonthly(tenantDB *gorm.DB, companyID uint, sub *models.Subscription, contractID, counterpartyID uint, contractCcy, billingMode, rateSource string, minDays int, rateCache map[string]cachedRate, cutoff, targetDate time.Time) (entries, skipped int) {
	price := sub.BillingPlan.Price
	if price.IsZero() {
		return
	}
	planCcy := sub.BillingPlan.Currency
	if planCcy == "" {
		planCcy = "RUB"
	}
	if contractCcy == "" {
		contractCcy = "RUB"
	}
	monthly := monthlyPlanAmount(price, sub.BillingPlan.BillingPeriod)
	prepaid := billingMode != "postpaid"
	// День наблюдения: для текущего месяца считаем присутствие только до сегодня+1 (Codex c).
	observedEnd := dayFloor(targetDate).AddDate(0, 0, 1)

	// Стартовый месяц: max(месяц старта подписки, месяц cutoff). Для postpaid берём на
	// месяц раньше — billingDay завершённого месяца = 1-е следующего, иначе только что
	// закрытый месяц не попал бы в скан (Codex e2).
	cutoffMonth := monthFloor(cutoff)
	if !prepaid {
		cutoffMonth = cutoffMonth.AddDate(0, -1, 0)
	}
	startMonth := monthFloor(sub.StartDate.UTC())
	if cutoffMonth.After(startMonth) {
		startMonth = cutoffMonth
	}

	for m := startMonth; ; m = m.AddDate(0, 1, 0) {
		// День биллинга месяца m.
		billingDay := m // prepaid → 1-е тек. месяца
		if !prepaid {
			billingDay = m.AddDate(0, 1, 0) // postpaid → 1-е след. месяца
		}
		if billingDay.After(targetDate) {
			break // день биллинга ещё не наступил
		}
		// Не биллим раньше cutoff (его day-precision; monthFloor её терял — Codex e1).
		if billingDay.Before(dayFloor(cutoff)) {
			continue
		}
		// Подписка закончилась раньше месяца m → дальше нечего начислять.
		if sub.EndDate != nil && monthFloor(sub.EndDate.UTC()).Before(m) {
			break
		}

		extID := fmt.Sprintf("autocharge:sub:%d:%s", sub.ID, m.Format("2006-01"))

		// Идемпотентность: месяц уже начислен monthly-проводкой?
		var exist int64
		if e := s.db.Model(&models.LedgerEntry{}).
			Where("admin_account_id = ? AND company_id = ? AND source = ? AND external_id = ? AND deleted_at IS NULL",
				sub.AdminAccountID, companyID, "auto_charge", extID).Count(&exist).Error; e != nil {
			log.Printf("⚠️ LedgerCharge[m]: sub %d %s — ошибка идемпотентности, стоп: %v", sub.ID, m.Format("2006-01"), e)
			return
		}
		if exist > 0 {
			skipped++
			continue
		}

		// Анти-двойное-списание при смене каденции (Codex a): если за этот месяц уже есть
		// ЕЖЕДНЕВНЫЕ проводки (external_id :YYYY-MM-DD), месяц уже покрыт — не дублируем.
		// ⚠️ Known limitation (Ф5 #2, под-счёт): при daily→monthly В СЕРЕДИНЕ месяца дни до
		// смены уже списаны посуточно, а полный monthly за месяц пропускается → остаток
		// месяца (дни после последней дневной проводки) не добирается. Сознательно: forward-
		// only без частичного monthly-пересчёта; под-счёт ≪ двойного счёта по тяжести. Чистое
		// поведение — менять каденцию на границе месяца.
		var dailyExist int64
		if e := s.db.Model(&models.LedgerEntry{}).
			Where("admin_account_id = ? AND company_id = ? AND source = ? AND external_id LIKE ? AND deleted_at IS NULL",
				sub.AdminAccountID, companyID, "auto_charge", fmt.Sprintf("autocharge:sub:%d:%s-%%", sub.ID, m.Format("2006-01"))).
			Count(&dailyExist).Error; e != nil {
			log.Printf("⚠️ LedgerCharge[m]: sub %d %s — ошибка проверки дневных, стоп: %v", sub.ID, m.Format("2006-01"), e)
			return
		}
		if dailyExist > 0 {
			skipped++
			continue
		}

		// Анти-двойное при обратной смене (lump→monthly): если ГОД, содержащий m, уже
		// списан годовой lump-проводкой (:y:<начало периода>) — месяц покрыт (Codex C2).
		lumpPeriod := monthFloor(sub.StartDate.UTC())
		for !m.Before(lumpPeriod.AddDate(1, 0, 0)) {
			lumpPeriod = lumpPeriod.AddDate(1, 0, 0)
		}
		var lumpExist int64
		if e := s.db.Model(&models.LedgerEntry{}).
			Where("admin_account_id = ? AND company_id = ? AND source = ? AND external_id = ? AND deleted_at IS NULL",
				sub.AdminAccountID, companyID, "auto_charge", fmt.Sprintf("autocharge:sub:%d:y:%s", sub.ID, lumpPeriod.Format("2006-01"))).
			Count(&lumpExist).Error; e != nil {
			log.Printf("⚠️ LedgerCharge[m]: sub %d %s — ошибка проверки lump, стоп: %v", sub.ID, m.Format("2006-01"), e)
			return
		}
		if lumpExist > 0 {
			skipped++
			continue
		}

		count, err := s.objectsPresentInWindowGE(tenantDB, sub.ID, m, capWindowEnd(m.AddDate(0, 1, 0), observedEnd), minDays)
		if err != nil {
			log.Printf("⚠️ LedgerCharge[m]: sub %d %s — ошибка подсчёта объектов, стоп: %v", sub.ID, m.Format("2006-01"), err)
			return
		}
		if count == 0 {
			continue
		}
		rawAmount := monthly.Mul(decimal.NewFromInt(int64(count)))

		// Конверсия валюты (П5) на день биллинга. ledger в валюте договора (инвариант [H]).
		var chargeAmount decimal.Decimal
		var metaPtr *string
		if planCcy == contractCcy {
			chargeAmount = rawAmount.Round(2)
		} else {
			rate, rateDate, stale, rerr := s.getRateCached(rateCache, billingDay, planCcy, contractCcy, rateSource)
			if rerr != nil {
				log.Printf("⚠️ LedgerCharge[m]: sub %d %s — нет курса %s→%s, стоп: %v", sub.ID, m.Format("2006-01"), planCcy, contractCcy, rerr)
				return
			}
			if rate.LessThanOrEqual(decimal.Zero) {
				log.Printf("❌ LedgerCharge[m]: sub %d %s — некорректный курс %s, стоп", sub.ID, m.Format("2006-01"), rate.String())
				return
			}
			if stale {
				log.Printf("⚠️ LedgerCharge[m]: sub %d %s — курс устарел (%s), стоп", sub.ID, m.Format("2006-01"), rateDate.Format("2006-01-02"))
				return
			}
			chargeAmount = convertAmount(rawAmount, rate)
			mb, mErr := json.Marshal(map[string]string{
				"rate": rate.String(), "rate_date": rateDate.Format("2006-01-02"),
				"source": rateSource, "original_amount": rawAmount.Round(2).String(), "original_ccy": planCcy,
			})
			if mErr != nil {
				log.Printf("⚠️ LedgerCharge[m]: sub %d %s — metadata, стоп: %v", sub.ID, m.Format("2006-01"), mErr)
				return
			}
			ms := string(mb)
			metaPtr = &ms
		}
		if chargeAmount.IsZero() {
			continue
		}

		subID := sub.ID
		entry := models.LedgerEntry{
			AdminAccountID: sub.AdminAccountID,
			CompanyID:      companyID,
			ContractID:     contractID,
			CounterpartyID: counterpartyID, // Ф2: денорм для баланса per-контрагент
			SubscriptionID: &subID,
			EntryType:      "charge",
			Amount:         chargeAmount.Neg(),
			Currency:       contractCcy,
			Source:         "auto_charge",
			ExternalID:     extID,
			Description:    fmt.Sprintf("Начисление за %s (%d объектов, месяц)", m.Format("2006-01"), count),
			EntryDate:      billingDay,
			CreatedBy:      "scheduler",
			Metadata:       metaPtr,
		}
		txErr := s.db.Transaction(func(tx *gorm.DB) error {
			var cnt int64
			if e := tx.Model(&models.LedgerEntry{}).
				Where("admin_account_id = ? AND company_id = ? AND source = ? AND external_id = ? AND deleted_at IS NULL",
					sub.AdminAccountID, companyID, "auto_charge", extID).Count(&cnt).Error; e != nil {
				return e
			}
			if cnt > 0 {
				return errChargeExists
			}
			return tx.Create(&entry).Error
		})
		switch txErr {
		case nil:
			entries++
		case errChargeExists:
			skipped++
		default:
			log.Printf("⚠️ LedgerCharge[m]: sub %d %s — ошибка записи, стоп: %v", sub.ID, m.Format("2006-01"), txErr)
			return
		}
	}
	return
}

// chargeSubscriptionPeriodLump — П6 Ф3 period-каденция, длинные (yearly) подписки РАЗОВО:
// вся сумма года 1 проводкой на день биллинга начала периода (prepaid → 1-е месяца старта
// периода, postpaid → 1-е следующего). Периоды (годы) катятся от месяца старта подписки.
// Сумма = полная годовая цена × объекты с присутствием ≥minDays в месяце старта периода.
func (s *LedgerChargeScheduler) chargeSubscriptionPeriodLump(tenantDB *gorm.DB, companyID uint, sub *models.Subscription, contractID, counterpartyID uint, contractCcy, billingMode, rateSource string, minDays int, rateCache map[string]cachedRate, cutoff, targetDate time.Time) (entries, skipped int) {
	price := sub.BillingPlan.Price // полная годовая цена
	if price.IsZero() {
		return
	}
	planCcy := sub.BillingPlan.Currency
	if planCcy == "" {
		planCcy = "RUB"
	}
	if contractCcy == "" {
		contractCcy = "RUB"
	}
	prepaid := billingMode != "postpaid"
	observedEnd := dayFloor(targetDate).AddDate(0, 0, 1)
	subID := sub.ID

	for p := monthFloor(sub.StartDate.UTC()); ; p = p.AddDate(1, 0, 0) {
		periodEnd := p.AddDate(1, 0, 0)
		billingDay := p // prepaid → 1-е месяца старта периода
		if !prepaid {
			billingDay = p.AddDate(0, 1, 0) // postpaid → 1-е след. месяца
		}
		if billingDay.After(targetDate) {
			break
		}
		if billingDay.Before(dayFloor(cutoff)) {
			continue
		}
		// Подписка закончилась раньше начала периода → стоп.
		if sub.EndDate != nil && dayFloor(sub.EndDate.UTC()).Before(p) {
			break
		}

		extID := fmt.Sprintf("autocharge:sub:%d:y:%s", subID, p.Format("2006-01"))

		// Идемпотентность периода.
		var exist int64
		if e := s.db.Model(&models.LedgerEntry{}).
			Where("admin_account_id = ? AND company_id = ? AND source = ? AND external_id = ? AND deleted_at IS NULL",
				sub.AdminAccountID, companyID, "auto_charge", extID).Count(&exist).Error; e != nil {
			log.Printf("⚠️ LedgerCharge[y]: sub %d %s — ошибка идемпотентности, стоп: %v", subID, p.Format("2006"), e)
			return
		}
		if exist > 0 {
			skipped++
			continue
		}

		// Анти-двойное-списание (Codex C1): если за ЛЮБОЙ из 12 месяцев периода уже есть
		// дневная/месячная проводка — год покрыт прежней каденцией, не дублируем годовой.
		// Проверяем по external_id (monthly :YYYY-MM и daily :YYYY-MM-DD оба начинаются с
		// :YYYY-MM), а НЕ по entry_date — иначе postpaid-декабрь (дата=1 янв) выпал бы из
		// окна периода и не детектился.
		covered, err := s.periodCoveredByShorterCadence(sub.AdminAccountID, companyID, subID, p, periodEnd)
		if err != nil {
			log.Printf("⚠️ LedgerCharge[y]: sub %d %s — ошибка проверки покрытия, стоп: %v", subID, p.Format("2006"), err)
			return
		}
		if covered {
			skipped++
			continue
		}

		// Окно присутствия = период [p, periodEnd), ограниченный днём наблюдения.
		count, err := s.objectsPresentInWindowGE(tenantDB, subID, p, capWindowEnd(periodEnd, observedEnd), minDays)
		if err != nil {
			log.Printf("⚠️ LedgerCharge[y]: sub %d %s — ошибка подсчёта объектов, стоп: %v", subID, p.Format("2006"), err)
			return
		}
		if count == 0 {
			continue
		}
		rawAmount := price.Mul(decimal.NewFromInt(int64(count)))

		var chargeAmount decimal.Decimal
		var metaPtr *string
		if planCcy == contractCcy {
			chargeAmount = rawAmount.Round(2)
		} else {
			rate, rateDate, stale, rerr := s.getRateCached(rateCache, billingDay, planCcy, contractCcy, rateSource)
			if rerr != nil {
				log.Printf("⚠️ LedgerCharge[y]: sub %d %s — нет курса %s→%s, стоп: %v", subID, p.Format("2006"), planCcy, contractCcy, rerr)
				return
			}
			if rate.LessThanOrEqual(decimal.Zero) {
				log.Printf("❌ LedgerCharge[y]: sub %d %s — некорректный курс %s, стоп", subID, p.Format("2006"), rate.String())
				return
			}
			if stale {
				log.Printf("⚠️ LedgerCharge[y]: sub %d %s — курс устарел (%s), стоп", subID, p.Format("2006"), rateDate.Format("2006-01-02"))
				return
			}
			chargeAmount = convertAmount(rawAmount, rate)
			mb, mErr := json.Marshal(map[string]string{
				"rate": rate.String(), "rate_date": rateDate.Format("2006-01-02"),
				"source": rateSource, "original_amount": rawAmount.Round(2).String(), "original_ccy": planCcy,
			})
			if mErr != nil {
				log.Printf("⚠️ LedgerCharge[y]: sub %d %s — metadata, стоп: %v", subID, p.Format("2006"), mErr)
				return
			}
			ms := string(mb)
			metaPtr = &ms
		}
		if chargeAmount.IsZero() {
			continue
		}

		sid := subID
		entry := models.LedgerEntry{
			AdminAccountID: sub.AdminAccountID,
			CompanyID:      companyID,
			ContractID:     contractID,
			CounterpartyID: counterpartyID, // Ф2: денорм для баланса per-контрагент
			SubscriptionID: &sid,
			EntryType:      "charge",
			Amount:         chargeAmount.Neg(),
			Currency:       contractCcy,
			Source:         "auto_charge",
			ExternalID:     extID,
			Description:    fmt.Sprintf("Начисление за период %s–%s (%d объектов, год)", p.Format("2006-01"), periodEnd.AddDate(0, 0, -1).Format("2006-01"), count),
			EntryDate:      billingDay,
			CreatedBy:      "scheduler",
			Metadata:       metaPtr,
		}
		txErr := s.db.Transaction(func(tx *gorm.DB) error {
			var cnt int64
			if e := tx.Model(&models.LedgerEntry{}).
				Where("admin_account_id = ? AND company_id = ? AND source = ? AND external_id = ? AND deleted_at IS NULL",
					sub.AdminAccountID, companyID, "auto_charge", extID).Count(&cnt).Error; e != nil {
				return e
			}
			if cnt > 0 {
				return errChargeExists
			}
			return tx.Create(&entry).Error
		})
		switch txErr {
		case nil:
			entries++
		case errChargeExists:
			skipped++
		default:
			log.Printf("⚠️ LedgerCharge[y]: sub %d %s — ошибка записи, стоп: %v", subID, p.Format("2006"), txErr)
			return
		}
	}
	return
}

// lastDailyChargeDate — последняя дата ДНЕВНОЙ auto_charge проводки подписки (external_id
// формата `autocharge:sub:N:YYYY-MM-DD`). Daily-checkpoint обязан игнорировать месячные/
// годовые проводки: их entry_date уезжает вперёд и сместил бы старт daily за непокрытый
// день при смене каденции (Ф5 #1). Паттерн `____-__-__` (ровно YYYY-MM-DD) портативен PG+SQLite.
func (s *LedgerChargeScheduler) lastDailyChargeDate(subID uint) *time.Time {
	var t *time.Time
	s.db.Model(&models.LedgerEntry{}).
		Where("subscription_id = ? AND source = ? AND deleted_at IS NULL", subID, "auto_charge").
		Where("external_id LIKE ?", fmt.Sprintf("autocharge:sub:%d:____-__-__", subID)).
		Select("MAX(entry_date)").Scan(&t)
	if t != nil {
		d := dayFloor(t.UTC())
		return &d
	}
	return nil
}

// objectCountOnDay — кол-во объектов подписки, биллящихся в указанный день.
// Историческое состояние: запись существовала (created_at/deleted_at) И
// привязка активна (start_date/end_date). status='active' — best-effort.
// Возвращает error: при сбое tenant-БД нельзя трактовать 0 как «нет объектов»
// (иначе день тихо пропускается, а checkpoint MAX(entry_date) уезжает вперёд —
// потерянное начисление навсегда). Вызывающий обязан остановить подписку.
func (s *LedgerChargeScheduler) objectCountOnDay(tenantDB *gorm.DB, subID uint, day time.Time) (int, error) {
	dayStart := dayFloor(day)
	dayEnd := dayStart.AddDate(0, 0, 1)
	var cnt int64
	// Unscoped(): GORM иначе авто-фильтрует deleted_at IS NULL и объект,
	// удалённый позже, не считался бы даже за дни ДО удаления. Историчность
	// держим сами через created_at/deleted_at.
	err := tenantDB.Unscoped().Model(&models.ContractObject{}).
		Where("subscription_id = ? AND status = ?", subID, "active").
		Where("created_at < ?", dayEnd).
		Where("deleted_at IS NULL OR deleted_at >= ?", dayStart).
		Where("start_date < ?", dayEnd).
		Where("end_date IS NULL OR end_date >= ?", dayStart).
		Count(&cnt).Error
	return int(cnt), err
}

func (s *LedgerChargeScheduler) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"is_running": s.isRunning,
		"last_run":   s.lastRun,
		"cutoff":     s.cutoffDate().Format("2006-01-02"),
	}
}

var errChargeExists = fmt.Errorf("ledger auto_charge already exists")

// cachedRate — закэшированный результат конверсии (П5 #6).
type cachedRate struct {
	rate     decimal.Decimal
	rateDate time.Time
	stale    bool
	err      error
}

// getRateCached — курс конверсии base→quote с кэшем на прогон компании. Использует
// GetConversionRate (pivot источника): прямой / inverse / cross. Для cbr_rf (pivot=RUB)
// договор в RUB = прямой курс план→RUB, договор в иной валюте = inverse/cross через RUB
// (П5 backlog: inverse/cross в charge). rate всегда >0, иначе err.
func (s *LedgerChargeScheduler) getRateCached(cache map[string]cachedRate, day time.Time, base, quote, source string) (decimal.Decimal, time.Time, bool, error) {
	key := fmt.Sprintf("%s|%s|%s|%s", day.Format("2006-01-02"), base, quote, source)
	if c, ok := cache[key]; ok {
		return c.rate, c.rateDate, c.stale, c.err
	}
	rate, rateDate, stale, err := s.rateSvc.GetConversionRate(day, base, quote, source)
	cache[key] = cachedRate{rate: rate, rateDate: rateDate, stale: stale, err: err}
	return rate, rateDate, stale, err
}

// holdAction — решение sweep'а по зонту (П3/П4).
type holdAction int

const (
	holdKeep    holdAction = iota // зонт держит, авто-приостановку пропускаем
	holdFulfill                   // долг погашен → закрыть зонт как fulfilled
	holdExpire                    // срок истёк, долг остался → expired, дальше обычный suspend
)

// holdLifecycleAction — pure-решение жизненного цикла зонта на момент now.
// balance >= threshold (threshold = −creditLimit) → долг в пределах → fulfill.
// Иначе в долгу: срок истёк (now >= holdUntil) → expire; иначе keep (держим).
// Порядок важен: fulfill приоритетнее expire (вышел из долга в день истечения — закрыть как исполненный).
func holdLifecycleAction(balance, threshold decimal.Decimal, holdUntil, now time.Time) holdAction {
	if !balance.LessThan(threshold) {
		return holdFulfill
	}
	if !holdUntil.After(now) {
		return holdExpire
	}
	return holdKeep
}

// holdLifecycleActionMulti — per-currency обёртка над holdLifecycleAction (сигнатуру
// pure-функции НЕ меняем, её table-тест остаётся валиден). inDebt уже посчитан по всем
// суб-балансам единицы (хотя бы один < threshold). Не в долгу → fulfill (передаём
// threshold==threshold → !LessThan → fulfill); в долгу → решает срок зонта по worstBal.
func holdLifecycleActionMulti(inDebt bool, worstBal, threshold decimal.Decimal, holdUntil, now time.Time) holdAction {
	if !inDebt {
		return holdLifecycleAction(threshold, threshold, holdUntil, now)
	}
	return holdLifecycleAction(worstBal, threshold, holdUntil, now)
}

// --- helpers ---

// dailyChargeAmountRaw — дневная сумма БЕЗ округления (high-precision): price × count
// / дней_в_периоде. Используется для конверсии валют, чтобы не терять копейки на
// двойном округлении (round в валюте плана → ×rate → round в валюте договора — Codex #2).
func dailyChargeAmountRaw(price decimal.Decimal, count int, day time.Time, yearly bool) decimal.Decimal {
	if count <= 0 || price.IsZero() {
		return decimal.Zero
	}
	periodDays := daysInMonth(day)
	if yearly {
		periodDays = daysInYear(day)
	}
	monthly := price.Mul(decimal.NewFromInt(int64(count)))
	return monthly.Div(decimal.NewFromInt(int64(periodDays)))
}

// dailyChargeAmount — дневная сумма начисления (>0), округлённая до 2 знаков
// (same-currency путь: ledger в валюте плана = валюте договора).
func dailyChargeAmount(price decimal.Decimal, count int, day time.Time, yearly bool) decimal.Decimal {
	return dailyChargeAmountRaw(price, count, day, yearly).Round(2)
}

// convertAmount — сумма amount (в валюте плана) → валюту договора по курсу rate
// (quote за 1 base), округление до 2 знаков на границе posting (Codex [M] decimal scale).
// amount передаётся RAW (без предв. округления) — единственное округление здесь.
func convertAmount(amount, rate decimal.Decimal) decimal.Decimal {
	return amount.Mul(rate).Round(2)
}

func dayFloor(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func daysInMonth(t time.Time) int {
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return first.AddDate(0, 1, 0).Add(-time.Hour).Day()
}

func daysInYear(t time.Time) int {
	y := t.Year()
	if (y%4 == 0 && y%100 != 0) || y%400 == 0 {
		return 366
	}
	return 365
}
