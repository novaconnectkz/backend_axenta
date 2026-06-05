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
// Модель данных Wialon (разведка 2026-05-23, см.
// wiki/concepts/wialon-partner-billing-ph2): дилер-дерево НЕ строится —
// поля bpact нет, crt (создатель) схлопывается под root-интеграторов, юниты
// биллятся на общий мастер-ресурс. Надёжно работает только ПРЯМОЙ счёт юнитов
// аккаунта: wialon_account_statuses.units_count (units.billing_id →
// object_stats.resource_id → user_id, покрытие 100%).
//
// Поэтому модель A (per-account): Wialon-партнёр = аккаунт с dealer_rights,
// биллится за СВОИ прямые units_count (не за поддерево). total = active =
// units_count (per-account breakdown активных недоступен; аккаунт целиком
// активен/нет по is_active).
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

// aggregateAccount возвращает (total, active, ok) объектов Wialon-партнёра:
// прямой units_count аккаунта. ok=false если аккаунт не найден/невалиден —
// caller ПРОПУСКАЕТ снимок (не пишет zero-cost «completed», Codex C2).
//
// total = active = units_count: per-account модель бьёт прямые юниты, breakdown
// активных по юнитам недоступен; is_active — состояние логина аккаунта, а не
// юнитов, поэтому не обнуляет биллинг (Codex H1, консистентно с unified-дропдауном).
//
// public.wialon_account_statuses явно: s.db — pooled conn, search_path мог быть
// переключён на tenant-схему GetTenantDBByID (Codex H2).
func (s *WialonPartnerSnapshotService) aggregateAccount(connID uint, userIDStr string) (total, active int, ok bool) {
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	var acc models.WialonAccountStatus
	if err := s.db.Table("public.wialon_account_statuses").
		Where("connection_id = ? AND wialon_user_id = ?", connID, userID).
		First(&acc).Error; err != nil {
		return 0, 0, false
	}
	return acc.UnitsCount, acc.UnitsCount, true
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

		total, active, ok := s.aggregateAccount(c.PartnerConnectionID, c.PartnerExternalID)
		if !ok {
			log.Printf("⚠️ Wialon договор %d: аккаунт %s не найден в wialon_account_statuses, пропуск (без zero-снимка)", c.ID, c.PartnerExternalID)
			continue
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
			// Ф1: cross-source -1 (account_statuses.units_count уже выведен из wialon_units —
			// тот же синк, независимого второго контура нет; сверка через continuity-guard). Ф2: Z-отчёт.
			VerifySecondaryCount: -1,
			Notes:                "Wialon partner snapshot (Ф2): прямой units_count аккаунта (per-account, без дерева)",
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
