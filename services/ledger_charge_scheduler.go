package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"context"
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
	return &LedgerChargeScheduler{cron: c, db: database.DB}
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

	// Кэш проверенных договоров: только client + active билятся.
	contractOK := make(map[uint]bool)
	for _, sub := range subs {
		if sub.ContractID == nil {
			continue
		}
		cid := *sub.ContractID
		ok, checked := contractOK[cid]
		if !checked {
			var ct models.Contract
			if e := tenantDB.Select("id, contract_type, status").First(&ct, cid).Error; e != nil {
				ok = false
			} else {
				ok = ct.ContractType == "client" && ct.Status == "active"
			}
			contractOK[cid] = ok
		}
		if !ok {
			continue
		}

		e, sk := s.chargeSubscription(tenantDB, company.ID, &sub, cid, cutoff, targetDate)
		entries += e
		skipped += sk
	}
	return
}

// chargeSubscription начисляет дневные charge'и по одной подписке за период.
func (s *LedgerChargeScheduler) chargeSubscription(tenantDB *gorm.DB, companyID uint, sub *models.Subscription, contractID uint, cutoff, targetDate time.Time) (entries, skipped int) {
	price := sub.BillingPlan.Price
	if price.IsZero() {
		return
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

		count := s.objectCountOnDay(tenantDB, sub.ID, day)
		if count == 0 {
			continue
		}

		daily := dailyChargeAmount(price, count, day, yearly)
		if daily.IsZero() {
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
			Amount:         daily.Neg(), // charge < 0
			Currency:       "RUB",
			Source:         "auto_charge",
			ExternalID:     extID,
			Description:    fmt.Sprintf("Начисление за %s (%d объектов)", day.Format("2006-01-02"), count),
			EntryDate:      day,
			CreatedBy:      "scheduler",
		}

		// Идемпотентность: уникальный (admin, company, source, external_id).
		txErr := s.db.Transaction(func(tx *gorm.DB) error {
			var cnt int64
			tx.Model(&models.LedgerEntry{}).
				Where("admin_account_id = ? AND company_id = ? AND source = ? AND external_id = ? AND deleted_at IS NULL",
					sub.AdminAccountID, companyID, "auto_charge", extID).Count(&cnt)
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
			log.Printf("⚠️ LedgerCharge: sub %d день %s — ошибка: %v", sub.ID, day.Format("2006-01-02"), txErr)
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
func (s *LedgerChargeScheduler) objectCountOnDay(tenantDB *gorm.DB, subID uint, day time.Time) int {
	dayStart := dayFloor(day)
	dayEnd := dayStart.AddDate(0, 0, 1)
	var cnt int64
	// Unscoped(): GORM иначе авто-фильтрует deleted_at IS NULL и объект,
	// удалённый позже, не считался бы даже за дни ДО удаления. Историчность
	// держим сами через created_at/deleted_at.
	tenantDB.Unscoped().Model(&models.ContractObject{}).
		Where("subscription_id = ? AND status = ?", subID, "active").
		Where("created_at < ?", dayEnd).
		Where("deleted_at IS NULL OR deleted_at >= ?", dayStart).
		Where("start_date < ?", dayEnd).
		Where("end_date IS NULL OR end_date >= ?", dayStart).
		Count(&cnt)
	return int(cnt)
}

func (s *LedgerChargeScheduler) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"is_running": s.isRunning,
		"last_run":   s.lastRun,
		"cutoff":     s.cutoffDate().Format("2006-01-02"),
	}
}

var errChargeExists = fmt.Errorf("ledger auto_charge already exists")

// --- helpers ---

// dailyChargeAmount — дневная сумма начисления (>0): price × count / дней_в_периоде,
// округлённая до 2 знаков. monthly → дней в календарном месяце дня; yearly → дней в году.
func dailyChargeAmount(price decimal.Decimal, count int, day time.Time, yearly bool) decimal.Decimal {
	if count <= 0 || price.IsZero() {
		return decimal.Zero
	}
	periodDays := daysInMonth(day)
	if yearly {
		periodDays = daysInYear(day)
	}
	monthly := price.Mul(decimal.NewFromInt(int64(count)))
	return monthly.Div(decimal.NewFromInt(int64(periodDays))).Round(2)
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
