package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// wialonAutoVerifyMaxDaily — Вариант 4: порог авто-verify бэкфилл-снимка (₽/день, до скидки).
// Структурно точный день (seasonal=0) дороже порога остаётся estimated → проходит через
// оператора. env WIALON_AUTOVERIFY_MAX_DAILY_RUB; пусто/0/невалид = без порога (полный авто).
func wialonAutoVerifyMaxDaily() decimal.Decimal {
	s := strings.TrimSpace(os.Getenv("WIALON_AUTOVERIFY_MAX_DAILY_RUB"))
	if s == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(s)
	if err != nil || d.IsNegative() {
		return decimal.Zero
	}
	return d
}

// WialonPartnerSnapshotService — Ф2 partner billing для Wialon-дилеров.
//
// Модель «поддерево» (2026-06-06): Wialon-партнёр = ПРЯМОЙ дилер (is_direct_dealer)
// под интеграционной у/з, биллится за ВСЁ поддерево под собой — как Wialon CMS и
// bill_wialon. База — account/get_account_data (поддеревная usage), не прямой
// units_count: тот недосчитывал дилера с субаккаунтами (Шевердяев: прямые 31, дерево 110).
//
// total = ObjectsTotal (activated+seasonal), active = ObjectsActive (activated_units.usage,
// оплачиваемые; деактивированные/seasonal НЕ биллятся → active<total возможно).
// Источник — wialon_account_statuses.objects_total/objects_active (наполняется
// collectAccountsForConnection: DISTINCT ON свежайшая строка per user из object_stats).
// Двойного счёта нет — биллятся только прямые дилеры под интеграцией (их дети — под ними).
//
// Снимок пишется в общую partner_daily_snapshots с partner_source='wialon'
// (схема Ф0). partner_external_id = wialon_user_id (строка).
type WialonPartnerSnapshotService struct {
	db   *gorm.DB // public/main: wialon_account_statuses, billing_plans, companies
	cron *cron.Cron
}

func NewWialonPartnerSnapshotService(db *gorm.DB) *WialonPartnerSnapshotService {
	return &WialonPartnerSnapshotService{db: db}
}

// wialonStatsStaleAfter — порог свежести wialon_account_statuses. Сборщик тикает
// каждые 15 мин; 24ч даёт большой запас, но ловит затяжной сбой sync конкретного
// коннекта (LastCollectedAt застывает) → снимок в needs_review, не авто-биллим старое.
const wialonStatsStaleAfter = 24 * time.Hour

// Start запускает daily-cron (00:50 UTC — после SKIF 00:45, до billing 01:00).
// Пишет снимок за ВЧЕРА (день N-1, только что завершившийся) — сегодняшний день ещё идёт,
// его биллить нельзя. На N=08.06 00:50 → снимок 07.06. Плюс fillForwardGaps добирает старые дыры.
func (s *WialonPartnerSnapshotService) Start() error {
	// SkipIfStillRunning (Codex): не запускать новый прогон, пока идёт предыдущий (gap-fill
	// может затянуться) — иначе перекрытие → last-writer-wins на upsert.
	c := cron.New(cron.WithLocation(time.UTC), cron.WithChain(
		cron.SkipIfStillRunning(cron.DefaultLogger), cron.Recover(cron.DefaultLogger)))
	_, err := c.AddFunc("50 0 * * *", func() {
		yesterday := time.Now().UTC().AddDate(0, 0, -1)
		if _, e := s.GenerateForAllTenants(yesterday); e != nil {
			log.Printf("⚠️ Wialon partner snapshot cron: %v", e)
		}
	})
	if err != nil {
		return err
	}
	c.Start()
	s.cron = c
	return nil
}

// aggregateAccount возвращает (total, active, ok) объектов Wialon-партнёра по модели
// «поддерево» (account_data): total = objects_total (activated+seasonal),
// active = objects_active (оплачиваемые = activated_units.usage). ok=false если
// аккаунт не найден/невалиден — caller ПРОПУСКАЕТ снимок (не пишет zero-cost, Codex C2).
//
// active < total возможно: деактивированные (seasonal) юниты НЕ биллятся (= Wialon CMS,
// = bill_wialon). НЕ units_count: для дилера он недосчитывает поддерево (Шевердяев 31 vs 110).
//
// fresh=false если снимок аккаунта протух (sync не обновлял > wialonStatsStaleAfter):
// caller ставит SourceWarn → снимок уходит в needs_review (не биллим устаревшие данные,
// Codex: transient sync-fail не должен молча забиллить старое/нулевое значение).
//
// public.wialon_account_statuses явно: s.db — pooled conn, search_path мог быть
// переключён на tenant-схему GetTenantDBByID (Codex H2).
func (s *WialonPartnerSnapshotService) aggregateAccount(connID uint, userIDStr string) (total, active int, fresh, ok bool) {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return 0, 0, false, false
	}
	var acc models.WialonAccountStatus
	if err := s.db.Table("public.wialon_account_statuses").
		Where("connection_id = ? AND wialon_user_id = ?", connID, userID).
		First(&acc).Error; err != nil {
		return 0, 0, false, false
	}
	fresh = time.Since(acc.LastCollectedAt) <= wialonStatsStaleAfter
	return acc.ObjectsTotal, acc.ObjectsActive, fresh, true
}

// GenerateForAllTenants — daily проход по всем активным тенантам.
func (s *WialonPartnerSnapshotService) GenerateForAllTenants(date time.Time) (int, error) {
	if err := s.db.Exec("SET search_path TO public").Error; err != nil {
		return 0, err
	}
	var companies []models.Company
	if err := s.db.Table("public.companies").Where("is_active = ?", true).Find(&companies).Error; err != nil {
		return 0, err
	}
	total := 0
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}
		n, err := s.GenerateForTenant(tenantDB, company.ID, date)
		if err != nil {
			log.Printf("⚠️ Wialon partner snapshots tenant %d: %v", company.ID, err)
			continue
		}
		total += n
	}
	s.db.Exec("SET search_path TO public")
	log.Printf("✅ Wialon partner snapshots: создано/обновлено %d за %s", total, date.Format("2006-01-02"))
	return total, nil
}

// GenerateForTenant — снимки за date для active Wialon-партнёрских договоров тенанта.
// companyID — id тенанта (public.companies): нужен для прохода по дилерам БЕЗ договора
// (connections + дефолт-план), который зеркалит Axenta (см. generateNoContractDealers).
func (s *WialonPartnerSnapshotService) GenerateForTenant(tenantDB *gorm.DB, companyID uint, date time.Time) (int, error) {
	var contracts []models.Contract
	if err := tenantDB.
		Where("contract_type = ? AND status = ? AND partner_source = ?", "partner", "active", "wialon").
		Where("partner_external_id <> ''").
		Find(&contracts).Error; err != nil {
		return 0, err
	}

	day := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	created := 0
	for i := range contracts {
		c := &contracts[i]
		// Gap-aware: добрать пропущенные прошлые дни [last_snapshot+1 .. вчера] через
		// get_statistics (smart-gate) ДО записи сегодня. Пропуски возникают, если день не записан
		// кроном (сервер лежал в 00:50 / договор создан в середине дня после крона) — без добора
		// день остаётся вечной дырой. GenerateForContractPeriod сам уважает срок и smart-gate.
		s.fillForwardGaps(tenantDB, c, day)

		// Энфорс срока договора: не пишем снимок вне [start_date, end_date] (end_date NULL = open).
		if !c.IsActiveOn(day) {
			continue
		}
		if c.TariffPlanID == nil {
			log.Printf("⚠️ Wialon договор %d без тарифа, пропуск", c.ID)
			continue
		}
		var plan models.BillingPlan
		// Явно public.billing_plans: s.db — pooled connection, чей search_path мог
		// быть переключён GetTenantDBByID на tenant-схему (где billing_plans пуст).
		if err := s.db.Table("public.billing_plans").Where("id = ?", *c.TariffPlanID).First(&plan).Error; err != nil {
			log.Printf("⚠️ Wialon договор %d: тариф %d не найден: %v", c.ID, *c.TariffPlanID, err)
			continue
		}

		// Подтверждённый вручную снимок заморожен — cron его не перезатирает.
		if partnerSnapshotIsApproved(tenantDB, "wialon", c.PartnerConnectionID, c.PartnerExternalID, c.ID, day) {
			log.Printf("🔒 Wialon снимок договора %d на %s подтверждён вручную — пропуск", c.ID, day.Format("2006-01-02"))
			continue
		}

		total, active, fresh, ok := s.aggregateAccount(c.PartnerConnectionID, c.PartnerExternalID)
		if !ok {
			log.Printf("⚠️ Wialon договор %d: аккаунт %s не найден в wialon_account_statuses, пропуск (без zero-снимка)", c.ID, c.PartnerExternalID)
			continue
		}

		// Устаревшие stats (sync застрял) → не биллим молча, помечаем needs_review.
		sourceWarn := ""
		if !fresh {
			sourceWarn = "wialon stats устарели (sync не обновлял аккаунт > 24ч)"
			log.Printf("⚠️ Wialon договор %d: stats аккаунта %s устарели → needs_review", c.ID, c.PartnerExternalID)
		}

		snap := models.PartnerDailySnapshot{
			AdminAccountID:     c.AdminAccountID,
			CompanyID:          c.CompanyID,
			ContractID:         c.ID,
			SnapshotDate:       day,
			PartnerCompanyID:   0, // у Wialon нет Axenta company id
			PartnerSource:      "wialon",
			ConnectionID:       c.PartnerConnectionID,
			PartnerExternalID:  c.PartnerExternalID,
			TariffPlanID:       plan.ID,
			MonthlyPrice:       plan.Price,
			TotalObjectsCount:  total,
			ActiveObjectsCount: active,
			DiscountType:       c.DiscountType,
			DiscountPercent:    c.GetDiscountPercent(active),
			DiscountFixed:      c.GetDiscountFixed(),
			Status:             "completed",
			// Ф1: cross-source -1 (objects_active выведен из того же синка account_data —
			// независимого второго контура нет; сверка через continuity-guard + freshness). Ф2: Z-отчёт.
			VerifySecondaryCount: -1,
			SourceWarn:           sourceWarn,
			Notes:                "Wialon partner snapshot (Ф2): поддерево дилера (account_data: objects_active=оплачиваемые)",
		}

		// BeforeCreate досчитает daily_price/cost_before_discount/discount_amount/daily_cost.
		if err := tenantDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "partner_source"}, {Name: "connection_id"}, {Name: "partner_external_id"},
				{Name: "contract_id"}, {Name: "snapshot_date"},
			},
			UpdateAll: true,
		}).Create(&snap).Error; err != nil {
			log.Printf("⚠️ Wialon снимок договора %d: %v", c.ID, err)
			continue
		}
		created++
		log.Printf("✅ Wialon снимок: договор %d аккаунт %s — %d/%d объектов (active/total)", c.ID, c.PartnerExternalID, active, total)
	}

	// Дилеры БЕЗ договора — ориентир по дефолт-тарифу (паритет с Axenta). Не биллится.
	created += s.generateNoContractDealers(tenantDB, companyID, contracts, day)
	return created, nil
}

// generateNoContractDealers пишет снимки-«ориентиры» для прямых Wialon-дилеров,
// у которых НЕТ активного партнёрского договора на день — чтобы они были видны в
// справочнике снимков (оранжевое «без договора», подсветка) так же, как Axenta-партнёры
// без договора. Стоимость считается по ДЕФОЛТНОМУ тарифу тенанта (как ориентир) с
// contract_id=0 — в биллинг такой снимок НЕ идёт (billing всегда фильтрует contract.ID>0).
//
// Скоуп: только is_direct_dealer (прямые под интеграционной у/з) — дилеры-дилеров не
// плодим (паритет с дропдауном договоров и applyDirectPartnerFilter в списке).
func (s *WialonPartnerSnapshotService) generateNoContractDealers(
	tenantDB *gorm.DB, companyID uint, contracts []models.Contract, day time.Time,
) int {
	// 1) Дефолт-план тенанта (зеркало Axenta: последний active). Fallback 70₽ ТОЛЬКО
	//    когда плана нет (ErrRecordNotFound); реальная ошибка БД (timeout/SQL) → НЕ пишем
	//    ориентиры с выдуманной ценой, выходим (Codex #3: не маскировать сбой как 70₽).
	var defaultPlan models.BillingPlan
	if err := s.db.Table("public.billing_plans").
		Where("is_active = ? AND admin_account_id = ?", true, companyID).
		Order("created_at DESC").First(&defaultPlan).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("⚠️ Wialon ориентиры: ошибка чтения billing_plans (company %d): %v — пропуск", companyID, err)
			return 0
		}
		defaultPlan = models.BillingPlan{Name: "Базовый партнёрский план", Price: decimal.NewFromFloat(70), BillingPeriod: "monthly", AdminAccountID: companyID}
	}

	// 2) Множество (connID|extID) дилеров, у которых УЖЕ есть договор, активный на день —
	//    их пропускаем (биллящаяся строка договора имеет приоритет, без дублей в справочнике).
	contracted := make(map[string]bool, len(contracts))
	for i := range contracts {
		c := &contracts[i]
		if c.IsActiveOn(day) {
			contracted[fmt.Sprintf("%d|%s", c.PartnerConnectionID, c.PartnerExternalID)] = true
		}
	}

	// 3) Connections тенанта (public, keyed company_id).
	var connIDs []uint
	if err := s.db.Table("public.wialon_connections").
		Where("company_id = ? AND deleted_at IS NULL", companyID).
		Pluck("id", &connIDs).Error; err != nil {
		log.Printf("⚠️ Wialon ориентиры: ошибка чтения connections (company %d): %v — пропуск", companyID, err)
		return 0
	}
	if len(connIDs) == 0 {
		return 0
	}

	// 4) Прямые дилеры по этим connections.
	var dealers []models.WialonAccountStatus
	if err := s.db.Table("public.wialon_account_statuses").
		Where("connection_id IN ? AND is_direct_dealer = ? AND is_active = ?", connIDs, true, true).
		Find(&dealers).Error; err != nil {
		log.Printf("⚠️ Wialon ориентиры: ошибка чтения дилеров (company %d): %v — пропуск", companyID, err)
		return 0
	}

	created := 0
	for i := range dealers {
		d := &dealers[i]
		extID := fmt.Sprintf("%d", d.WialonUserID)
		if contracted[fmt.Sprintf("%d|%s", d.ConnectionID, extID)] {
			continue // есть договор → не дублируем «ориентиром»
		}
		// Подтверждённый вручную «без договора» снимок заморожен — не перезатираем.
		if partnerSnapshotIsApproved(tenantDB, "wialon", d.ConnectionID, extID, 0, day) {
			continue
		}

		// Устаревшие stats (sync застрял) → needs_review (не показываем молча старое как точное).
		sourceWarn := ""
		if time.Since(d.LastCollectedAt) > wialonStatsStaleAfter {
			sourceWarn = "wialon stats устарели (sync не обновлял аккаунт > 24ч)"
		}

		snap := models.PartnerDailySnapshot{
			AdminAccountID:     companyID,
			CompanyID:          companyID,
			ContractID:         0, // Нет договора — ориентир, в биллинг не идёт
			SnapshotDate:       day,
			PartnerCompanyID:   0,
			PartnerSource:      "wialon",
			ConnectionID:       d.ConnectionID,
			PartnerExternalID:  extID,
			TariffPlanID:       defaultPlan.ID,
			MonthlyPrice:       defaultPlan.Price,
			TotalObjectsCount:  d.ObjectsTotal,
			ActiveObjectsCount: d.ObjectsActive,
			DiscountType:       "none", // без договора — без скидок
			DiscountPercent:    decimal.Zero,
			DiscountFixed:      decimal.Zero,
			Status:             "completed",
			VerifySecondaryCount: -1,
			SourceWarn:           sourceWarn,
			Notes:                "Wialon дилер БЕЗ договора: ориентир по дефолт-тарифу (в биллинг не идёт)",
		}

		// BeforeCreate досчитает daily_cost + verify_status. OnConflict по тому же
		// составному ключу (contract_id=0 не пересекается с договорными строками).
		if err := tenantDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "partner_source"}, {Name: "connection_id"}, {Name: "partner_external_id"},
				{Name: "contract_id"}, {Name: "snapshot_date"},
			},
			UpdateAll: true,
		}).Create(&snap).Error; err != nil {
			log.Printf("⚠️ Wialon снимок-ориентир дилера %s (conn %d): %v", extID, d.ConnectionID, err)
			continue
		}
		created++
	}
	if created > 0 {
		log.Printf("✅ Wialon снимки-ориентиры (без договора): создано/обновлено %d за %s", created, day.Format("2006-01-02"))
	}
	return created
}

// fillForwardGaps — gap-aware добор: восстанавливает ЛЮБЫЕ пропущенные дни договора в окне
// [start_date .. min(вчера, end_date)] через get_statistics (smart-gate). Триггерится из daily-
// крона ПЕРЕД записью сегодня. Сравнивает фактическое число снимков с ожидаемым; полно → no-op
// (get_statistics не дёргается). Есть дыра (хвост ИЛИ интерьер — лежал крон / договор создан
// после 00:50 / пропущенный день) → перезаливает весь период (идемпотентно). Самозалечивание.
func (s *WialonPartnerSnapshotService) fillForwardGaps(tenantDB *gorm.DB, c *models.Contract, snapDay time.Time) {
	if c.StartDate == nil || c.TariffPlanID == nil || c.PartnerExternalID == "" {
		return
	}
	startDay := time.Date(c.StartDate.Year(), c.StartDate.Month(), c.StartDate.Day(), 0, 0, 0, 0, time.UTC)
	// snapDay = день, который пишет forward-путь (вчера). Дыры добираем СТРОГО до него, чтобы не
	// пересекаться с forward-записью snapDay (его account_data пишет цикл отдельно).
	upper := snapDay.AddDate(0, 0, -1)
	if c.EndDate != nil {
		ed := time.Date(c.EndDate.Year(), c.EndDate.Month(), c.EndDate.Day(), 0, 0, 0, 0, time.UTC)
		if ed.Before(upper) {
			upper = ed // договор закончился раньше — ожидаем дни только до end_date
		}
	}
	if startDay.After(upper) {
		return // прошлого in-period окна нет
	}

	expected := int64(upper.Sub(startDay).Hours()/24) + 1
	var actual int64
	tenantDB.Table("partner_daily_snapshots").
		Where("partner_source = ? AND connection_id = ? AND partner_external_id = ? AND contract_id = ? AND deleted_at IS NULL",
			"wialon", c.PartnerConnectionID, c.PartnerExternalID, c.ID).
		Where("snapshot_date >= ? AND snapshot_date <= ?", startDay, upper).
		Count(&actual)
	if actual >= expected {
		return // дыр нет — не дёргаем get_statistics (норма каждого дня)
	}

	n, failed, err := s.GenerateForContractPeriod(tenantDB, c.ID, startDay, upper, true) // gapOnly
	if err != nil {
		log.Printf("⚠️ Wialon gap-fill договор %d [%s..%s]: %v",
			c.ID, startDay.Format("2006-01-02"), upper.Format("2006-01-02"), err)
		return
	}
	log.Printf("🩹 Wialon gap-fill договор %d: было %d/%d, добор %d (провалов %d) [%s..%s]",
		c.ID, actual, expected, n, failed, startDay.Format("2006-01-02"), upper.Format("2006-01-02"))
}

// GenerateForContractPeriod — backfill снимков Wialon-партнёрского договора за [from..to].
//
// История total per-date берётся из get_statistics recursive=0 на bact-ресурсе ПРЯМОГО дилера.
// M1-замер (conn 7/8/9, 2026-06-06): recursive=0 на bact дилера = поддеревный avl_unit_total,
// сходится с forward account_data (gate2 чисто); recursive=1 над-считает x2 (bact уже агрегирует
// поддерево). active(date) = total(date) − seasonal(now): связь activated == avl_unit − seasonal
// подтверждена на 12 субъектах с деактивированными (gate1).
//
// seasonal заморожен на текущем значении (get_statistics не отдаёт per-date activated/seasonal,
// только avl_unit_total). → снимок IsEstimated при seasonal>0 (active может дрейфовать на длинных
// окнах). При seasonal==0 active=total=реальные API-данные → не estimated.
//
// Идемпотентно (OnConflict UpdateAll), уважает подтверждённые вручную снимки. Дни пишутся по
// возрастанию даты, чтобы continuity-guard видел baseline предыдущего дня.
// gapOnly=true: пишем ТОЛЬКО дни без существующего (non-deleted) снимка — для gap-fill, чтобы
// одна дыра не перезатёрла весь период (Codex#1). gapOnly=false: перезалив всего (manual «за период»).
func (s *WialonPartnerSnapshotService) GenerateForContractPeriod(tenantDB *gorm.DB, contractID uint, from, to time.Time, gapOnly bool) (created int, failed int, err error) {
	var c models.Contract
	if err := tenantDB.First(&c, contractID).Error; err != nil {
		return 0, 0, fmt.Errorf("договор %d: %w", contractID, err)
	}
	if c.ContractType != "partner" || c.PartnerSource != "wialon" || c.PartnerExternalID == "" {
		return 0, 0, fmt.Errorf("договор %d не Wialon-партнёрский (type=%s source=%s ext=%q)",
			contractID, c.ContractType, c.PartnerSource, c.PartnerExternalID)
	}
	if c.TariffPlanID == nil {
		return 0, 0, fmt.Errorf("договор %d без тарифа", contractID)
	}
	var plan models.BillingPlan
	if err := s.db.Table("public.billing_plans").Where("id = ?", *c.TariffPlanID).First(&plan).Error; err != nil {
		return 0, 0, fmt.Errorf("тариф %d: %w", *c.TariffPlanID, err)
	}

	userID, err := strconv.ParseInt(c.PartnerExternalID, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("partner_external_id %q не число: %w", c.PartnerExternalID, err)
	}

	// seasonal(now) = objects_total − objects_active (заморожен) из wialon_account_statuses.
	// fresh учитываем: total истории — live (get_statistics), а seasonal — из таблицы синка;
	// если таблица устарела (>24ч), seasonal на старых данных → needs_review (паритет forward).
	totalNow, activeNow, fresh, ok := s.aggregateAccount(c.PartnerConnectionID, c.PartnerExternalID)
	if !ok {
		return 0, 0, fmt.Errorf("аккаунт %s не найден в wialon_account_statuses", c.PartnerExternalID)
	}
	seasonalNow := totalNow - activeNow
	if seasonalNow < 0 {
		seasonalNow = 0
	}

	// Коннект + login в Wialon.
	var conn models.WialonConnection
	if err := s.db.First(&conn, c.PartnerConnectionID).Error; err != nil {
		return 0, 0, fmt.Errorf("коннект %d: %w", c.PartnerConnectionID, err)
	}
	ws := NewWialonService()
	ss := NewWialonStatsService()
	hs := NewWialonHistoryService(s.db, ws, ss)
	login, err := ws.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return 0, 0, fmt.Errorf("login conn %d: %w", conn.ID, err)
	}
	defer func() { _ = ws.LogoutWithHost(conn.Host, login.Eid) }()
	apiURL := conn.Host + "/wialon/ajax.html"

	bactRes, err := ss.ResolveBactForUser(conn.Host, login.Eid, userID)
	if err != nil || bactRes == 0 {
		return 0, 0, fmt.Errorf("resolve bact user=%d: %v (bact=%d)", userID, err, bactRes)
	}

	// get_statistics recursive=0 → avl_unit_total per-date (M1: НЕ recursive=1).
	stats, err := hs.GetStatistics(apiURL, login.Eid, bactRes, from, to)
	if err != nil {
		return 0, 0, fmt.Errorf("get_statistics(bact=%d): %w", bactRes, err)
	}
	totalByDate := make(map[string]int, len(stats))
	windowHasChurn := false // Codex#1: были ли created/deleted в окне — признак нестабильности
	for _, st := range stats {
		k := time.Unix(st.Timestamp, 0).UTC().Format("2006-01-02")
		totalByDate[k] = st.UnitTotal
		if st.UnitCreated > 0 || st.UnitDeleted > 0 {
			windowHasChurn = true
		}
	}

	// gapOnly (Codex#1): даты с уже существующим non-deleted снимком — НЕ перезаписываем
	// (одна дыра не должна перетереть весь период). deleted_at IS NULL (Codex#2: soft-deleted
	// не маскируют дыру — raw Table() обходит default-scope GORM).
	existingDates := map[string]bool{}
	if gapOnly {
		var dates []time.Time
		tenantDB.Table("partner_daily_snapshots").
			Where("partner_source = ? AND connection_id = ? AND partner_external_id = ? AND contract_id = ? AND deleted_at IS NULL AND snapshot_date >= ? AND snapshot_date <= ?",
				"wialon", c.PartnerConnectionID, c.PartnerExternalID, c.ID, from, to).
			Pluck("snapshot_date", &dates)
		for _, d := range dates {
			existingDates[d.UTC().Format("2006-01-02")] = true
		}
	}

	// Codex#6: согласованность текущих данных. activeNow>totalNow → seasonal клемпнут в 0 и
	// «структурно точно» стало бы ложным → авто-verify запрещён (данные синка противоречивы).
	consistentNow := totalNow >= activeNow

	// Codex H5: партнёр должен быть ПРЯМЫМ дилером — recursive=0 на bact даёт его поддерево.
	// Если external_id указывает на вложенного субдилера, поддерево не то что ждёт биллинг →
	// помечаем снимки SourceWarn (→ needs_review), не авто-биллим сомнительное.
	baseWarn := ""
	if !fresh {
		baseWarn = "wialon stats устарели (sync не обновлял аккаунт >24ч) — seasonal на устаревших данных"
		log.Printf("⚠️ Wialon backfill договор %d: stats аккаунта %s устарели → needs_review", c.ID, c.PartnerExternalID)
	}
	var isDirect bool
	// Codex D2: проверяем .Error — строку уже подтвердил aggregateAccount (ok=true), поэтому
	// ошибка тут транзиентная; не штрафуем ложным needs_review. Warn только при ПОДТВЕРЖДЁННОМ
	// not-direct (запрос прошёл, is_direct_dealer=false).
	if e := s.db.Table("public.wialon_account_statuses").
		Select("is_direct_dealer").
		Where("connection_id = ? AND wialon_user_id = ?", c.PartnerConnectionID, userID).
		Scan(&isDirect).Error; e != nil {
		log.Printf("⚠️ Wialon backfill договор %d: чтение is_direct_dealer: %v (warn пропущен)", c.ID, e)
	} else if !isDirect {
		if baseWarn != "" {
			baseWarn += "; "
		}
		baseWarn += "партнёр не прямой дилер (recursive=0 покрывает лишь его поддерево)"
		log.Printf("⚠️ Wialon backfill договор %d: аккаунт %s не is_direct_dealer → needs_review", c.ID, c.PartnerExternalID)
	}

	// Codex C2/H6: backfill пишет ТОЛЬКО прошлое. Сегодняшним днём владеет forward-cron
	// (account_data) — иначе гонка last-writer на ключе (contract,today) и флап verify_status.
	today := time.Now().UTC().Truncate(24 * time.Hour)

	autoVerifyMax := wialonAutoVerifyMaxDaily() // Вариант 4: порог авто-verify (₽/день)

	var lastTotal int
	haveAnchor := false
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
		if !day.Before(today) {
			break // сегодня и будущее — не наши (forward-cron); даты по возрастанию → break
		}
		// Энфорс срока договора: не реконструируем снимки вне [start_date, end_date].
		if !c.IsActiveOn(day) {
			continue
		}
		key := day.Format("2006-01-02")
		if gapOnly && existingDates[key] {
			continue // день уже есть (non-deleted) — gap-fill не перезаписывает (Codex#1)
		}

		// Codex C1: has=true (API вернул точку, даже UnitTotal=0 — реальный простой) → доверяем
		// значению, не подменяем carry. Carry только на gap (has=false): API день не вернул.
		total, has := totalByDate[key]
		if has {
			lastTotal = total
			haveAnchor = true
		} else {
			if !haveAnchor {
				continue // gap до первой known-точки — аккаунт ещё не существовал, zero-снимок не пишем
			}
			total = lastTotal // gap после данных — несём последний known (день всё равно estimated)
		}

		active := total - seasonalNow
		if active < 0 {
			active = 0
		}

		// Подтверждённый вручную снимок заморожен — не перезатираем (pre-check, паритет forward-cron).
		if partnerSnapshotIsApproved(tenantDB, "wialon", c.PartnerConnectionID, c.PartnerExternalID, c.ID, day) {
			continue
		}

		// Вариант 1 (smart-gate): день авто-verified только при СТРОГОЙ структурной точности:
		//  - seasonalNow==0 (active=total, нет сезонного дрейфа)
		//  - has (реальная точка get_statistics, не gap-fill carry)
		//  - consistentNow (active≤total — данные синка не противоречивы; Codex#6)
		//  - !windowHasChurn (в окне нет created/deleted → набор юнитов стабилен, seasonal вряд ли
		//    менялся → frozen-now seasonal=0 надёжно для прошлого; Codex#1).
		// Иначе estimated. baseWarn (не-дилер/протух) → needs_review поверх.
		isEstimated := !(seasonalNow == 0 && has && consistentNow && !windowHasChurn)
		// Вариант 4: даже точный, но дорогой день (> порога) → estimated (крупное через оператора).
		if !isEstimated && autoVerifyMax.IsPositive() {
			dim := decimal.NewFromInt(int64(time.Date(day.Year(), day.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()))
			estDaily := plan.Price.Div(dim).Mul(decimal.NewFromInt(int64(active)))
			if estDaily.GreaterThan(autoVerifyMax) {
				isEstimated = true
			}
		}

		snap := models.PartnerDailySnapshot{
			AdminAccountID:       c.AdminAccountID,
			CompanyID:            c.CompanyID,
			ContractID:           c.ID,
			SnapshotDate:         day,
			PartnerCompanyID:     0,
			PartnerSource:        "wialon",
			ConnectionID:         c.PartnerConnectionID,
			PartnerExternalID:    c.PartnerExternalID,
			TariffPlanID:         plan.ID,
			MonthlyPrice:         plan.Price,
			TotalObjectsCount:    total,
			ActiveObjectsCount:   active,
			DiscountType:         c.DiscountType,
			DiscountPercent:      c.GetDiscountPercent(active),
			DiscountFixed:        c.GetDiscountFixed(),
			Status:               "completed",
			VerifySecondaryCount: -1,
			// Вариант 1+4: estimated только когда есть неопределённость (seasonal>0 дрейф / gap /
			// дорогой день > порога). Структурно точные дешёвые дни (seasonal=0, реальные данные)
			// → IsEstimated=false → guard ставит verified → авто-биллинг без оператора.
			IsEstimated: isEstimated,
			SourceWarn:  baseWarn,
			Notes:       "Wialon backfill (M2): get_statistics recursive=0 avl_unit_total, active=total−seasonal(now)",
		}
		if e := tenantDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "partner_source"}, {Name: "connection_id"}, {Name: "partner_external_id"},
				{Name: "contract_id"}, {Name: "snapshot_date"},
			},
			UpdateAll: true,
		}).Create(&snap).Error; e != nil {
			log.Printf("⚠️ Wialon backfill договор %d %s: %v", c.ID, key, e)
			failed++ // Codex M8: частичные провалы видимы вызывающему, не молчим
			continue
		}
		created++
	}

	log.Printf("✅ Wialon backfill договор %d (%s): создано %d, провалов %d [%s..%s), seasonal(now)=%d direct=%v",
		c.ID, c.PartnerExternalID, created, failed, from.Format("2006-01-02"), today.Format("2006-01-02"),
		seasonalNow, isDirect)
	return created, failed, nil
}
