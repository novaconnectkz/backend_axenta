package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
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
	cron      *cron.Cron
	db        *gorm.DB
	rateSvc   *CurrencyRateService // конверсия валют при начислении (П5 фаза 2)
	isRunning bool
	lastRun   time.Time
}

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

	// Клиентские договоры в активном/приостановленном статусе.
	var contracts []models.Contract
	if err := tenantDB.Where("contract_type = ? AND status IN ? AND deleted_at IS NULL",
		"client", []string{"active", "suspended"}).Find(&contracts).Error; err != nil {
		return
	}
	pub := s.db.Session(&gorm.Session{})
	now := time.Now().UTC()

	// Активные holds компании (П3/П4) — один запрос, map по (contract_id, admin_account_id).
	// admin в ключе обязателен: contract_id может совпадать у разных админов одной company
	// → без admin чужой зонт заблокировал бы приостановку (зеркало scoping баланса, Codex #3).
	// Зонт блокирует авто-приостановку до hold_until; просроченный гасим тут же.
	type holdKey struct {
		contractID uint
		adminID    uint
	}
	holdByContract := make(map[holdKey]models.BillingHold)
	{
		var holds []models.BillingHold
		pub.Table("public.billing_holds").
			Where("company_id = ? AND active = ? AND deleted_at IS NULL", company.ID, true).
			Find(&holds)
		for _, h := range holds {
			holdByContract[holdKey{h.ContractID, h.AdminAccountID}] = h
		}
	}

	for i := range contracts {
		ct := &contracts[i]
		// Баланс СТРОГО в рамках компании+админа (contract_id может совпадать в разных
		// tenant-схемах одного admin → без company_id балансы смешались бы — Codex critical).
		var bal decimal.Decimal
		pub.Table("public.ledger_entries").
			Where("contract_id = ? AND admin_account_id = ? AND company_id = ? AND deleted_at IS NULL", ct.ID, ct.AdminAccountID, company.ID).
			Select("COALESCE(SUM(amount),0)").Scan(&bal)
		threshold := ct.CreditLimit.Neg() // допустимый минус: balance < −creditLimit → блок

		// Lifecycle зонта (П3/П4) ДО решения о приостановке. Решение — pure-функция
		// holdLifecycleAction (тестируется без БД); здесь только применяем эффект.
		hold, hasHold := holdByContract[holdKey{ct.ID, ct.AdminAccountID}]
		holdActive := false
		if hasHold {
			switch holdLifecycleAction(bal, threshold, hold.HoldUntil, now) {
			case holdFulfill:
				// Долг погашен (в т.ч. promise исполнен) → закрываем зонт как fulfilled.
				// Atomic WHERE active=true; ошибку логируем (не глотаем — Codex #7).
				if e := pub.Table("public.billing_holds").Where("id = ? AND active = ?", hold.ID, true).
					Updates(map[string]interface{}{"status": "fulfilled", "active": false, "resolved_at": now}).Error; e != nil {
					log.Printf("⚠️ SuspensionSweep: не удалось закрыть hold %d (fulfilled): %v", hold.ID, e)
				}
			case holdExpire:
				// Срок зонта истёк, долг остался → expired. Блокируем дальше ТОЛЬКО если
				// expire реально снял active (RowsAffected>0) — иначе оставляем holdActive,
				// чтобы не приостановить договор при всё ещё активном зонте в БД (Codex #7).
				res := pub.Table("public.billing_holds").Where("id = ? AND active = ?", hold.ID, true).
					Updates(map[string]interface{}{"status": "expired", "active": false, "resolved_at": now})
				if res.Error != nil {
					log.Printf("⚠️ SuspensionSweep: не удалось закрыть hold %d (expired): %v", hold.ID, res.Error)
					holdActive = true // не смогли снять зонт → не блокируем в этот прогон
				} else if res.RowsAffected == 0 {
					holdActive = true // кто-то уже снял/изменил зонт параллельно → не блокируем
				}
			default:
				holdActive = true // зонт ещё держит — приостановку не делаем
			}
		}

		var active models.BillingSuspension
		hasActive := pub.Table("public.billing_suspensions").
			Where("contract_id = ? AND admin_account_id = ? AND company_id = ? AND reason = ? AND active = ? AND deleted_at IS NULL",
				ct.ID, ct.AdminAccountID, company.ID, "billing_debt", true).First(&active).Error == nil

		switch {
		case bal.LessThan(threshold) && !hasActive && !holdActive && ct.Status == "active":
			// Приостанавливаем ТОЛЬКО из status='active' (не трогаем ручной suspend/draft/...).
			// suspension create + status update — в одной tx (атомарность, Codex H1).
			susp := models.BillingSuspension{
				AdminAccountID: ct.AdminAccountID, CompanyID: company.ID, ContractID: ct.ID,
				Reason: "billing_debt", PreviousStatus: "active", DebtAmount: bal.Abs(),
				Active: true, SuspendedBy: "scheduler",
			}
			err := pub.Transaction(func(tx *gorm.DB) error {
				if e := tx.Table("public.billing_suspensions").Create(&susp).Error; e != nil {
					return e // partial-unique поймает гонку → дубль не создастся
				}
				return tenantDB.Model(&models.Contract{}).Where("id = ? AND status = ?", ct.ID, "active").Update("status", "suspended").Error
			})
			if err == nil {
				suspended++
			}
		case !bal.LessThan(threshold) && hasActive:
			// Долг погашен/в пределах → снять debt-приостановку. Возвращаем статус в 'active'
			// (приостанавливали только из active). Ручной suspend сюда не попадёт (нет active billing_debt).
			err := pub.Transaction(func(tx *gorm.DB) error {
				if e := tx.Table("public.billing_suspensions").Where("id = ?", active.ID).
					Updates(map[string]interface{}{"active": false, "resolved_at": now}).Error; e != nil {
					return e
				}
				return tenantDB.Model(&models.Contract{}).Where("id = ? AND status = ?", ct.ID, "suspended").Update("status", "active").Error
			})
			if err == nil {
				resolved++
			}
		}
	}
	return
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

	// Источник курса компании (П5): из политики биллинга, default cbr_rf.
	rateSource := "cbr_rf"
	{
		var bs models.BillingSettings
		if s.db.Session(&gorm.Session{}).Table("public.billing_settings").
			Where("company_id = ?", company.ID).Select("rate_source").First(&bs).Error == nil && bs.RateSource != "" {
			rateSource = bs.RateSource
		}
	}

	// Кэш проверенных договоров: только client + active билятся.
	// currency держим для конверсии валют (П5): ledger пишется в валюте договора.
	type ctInfo struct {
		ok       bool
		currency string
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
			if e := tenantDB.Select("id, contract_type, status, currency").First(&ct, cid).Error; e != nil {
				info = ctInfo{ok: false}
			} else {
				ccy := ct.Currency
				if ccy == "" {
					ccy = "RUB"
				}
				info = ctInfo{ok: ct.ContractType == "client" && ct.Status == "active", currency: ccy}
			}
			contractInfo[cid] = info
		}
		if !info.ok {
			continue
		}

		e, sk := s.chargeSubscription(tenantDB, company.ID, &sub, cid, info.currency, rateSource, rateCache, cutoff, targetDate)
		entries += e
		skipped += sk
	}
	return
}

// chargeSubscription начисляет дневные charge'и по одной подписке за период.
// contractCcy — валюта договора (ledger пишется в ней, инвариант Codex [H]).
// rateSource — источник курса для конверсии plan.Currency → contractCcy (П5).
func (s *LedgerChargeScheduler) chargeSubscription(tenantDB *gorm.DB, companyID uint, sub *models.Subscription, contractID uint, contractCcy, rateSource string, rateCache map[string]cachedRate, cutoff, targetDate time.Time) (entries, skipped int) {
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
	if last := s.lastChargeDate(sub.ID); last != nil {
		next := last.AddDate(0, 0, 1)
		if next.After(startDay) {
			startDay = next
		}
	}

	yearly := sub.BillingPlan.BillingPeriod == "yearly"

	for day := startDay; !day.After(targetDate); day = day.AddDate(0, 0, 1) {
		// Подписка должна быть активна в этот день.
		if dayFloor(sub.StartDate.UTC()).After(day) {
			continue
		}
		if sub.EndDate != nil && dayFloor(sub.EndDate.UTC()).Before(day) {
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

// lastChargeDate — дата последней auto_charge проводки подписки (per-sub checkpoint).
func (s *LedgerChargeScheduler) lastChargeDate(subID uint) *time.Time {
	var t *time.Time
	s.db.Model(&models.LedgerEntry{}).
		Where("subscription_id = ? AND source = ? AND deleted_at IS NULL", subID, "auto_charge").
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

// cachedRate — закэшированный результат GetRate (П5 #6).
type cachedRate struct {
	rate     decimal.Decimal
	rateDate time.Time
	stale    bool
	err      error
}

// getRateCached — GetRate с кэшем на прогон компании + ограничение источника (Codex #5).
// cbr_rf котирует только в RUB → если валюта договора не RUB, прямой пары нет и
// inverse/cross пока не поддержан: возвращаем ошибку (подписка стопнётся с понятным логом),
// а не молча начисляем неверно.
func (s *LedgerChargeScheduler) getRateCached(cache map[string]cachedRate, day time.Time, base, quote, source string) (decimal.Decimal, time.Time, bool, error) {
	if source == "cbr_rf" && quote != "RUB" {
		return decimal.Zero, time.Time{}, false,
			fmt.Errorf("источник cbr_rf поддерживает только котировку в RUB (договор в %s — нужен другой источник/inverse, не реализовано)", quote)
	}
	key := fmt.Sprintf("%s|%s|%s|%s", day.Format("2006-01-02"), base, quote, source)
	if c, ok := cache[key]; ok {
		return c.rate, c.rateDate, c.stale, c.err
	}
	rate, rateDate, stale, err := s.rateSvc.GetRate(day, base, quote, source)
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
