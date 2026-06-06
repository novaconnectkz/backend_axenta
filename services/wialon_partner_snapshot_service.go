package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"log"
	"strconv"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
func (s *WialonPartnerSnapshotService) Start() error {
	c := cron.New(cron.WithLocation(time.UTC), cron.WithChain(cron.Recover(cron.DefaultLogger)))
	_, err := c.AddFunc("50 0 * * *", func() {
		today := time.Now().UTC()
		if _, e := s.GenerateForAllTenants(today); e != nil {
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
		n, err := s.GenerateForTenant(tenantDB, date)
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
func (s *WialonPartnerSnapshotService) GenerateForTenant(tenantDB *gorm.DB, date time.Time) (int, error) {
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
	return created, nil
}
