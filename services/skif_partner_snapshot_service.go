package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"log"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SkifPartnerSnapshotService — Ф1 partner billing для SKIF-дилеров.
//
// Модель данных SKIF (разведка 2026-05-21):
//   - Партнёр = SKIF-дилер (skif_dealers, UUID = partner_external_id договора).
//   - Объекты дилера НЕ связаны с ним напрямую (skif_units.skif_company_id ≠
//     skif_dealer_id). Связь идёт через КОМПАНИИ: auth_admin_query возвращает
//     dealer_id у каждой компании → skif_company_statuses.skif_dealer_id.
//   - Объекты дилера = Σ units его компаний. «Активные» = Σ units компаний
//     где company_status='ACTIVE' (skif_units.is_active не наполняется — у всех false).
//
// Снимок пишется в общую partner_daily_snapshots с partner_source='skif'
// (схема Ф0). История forward-only (company_status за прошлые даты не хранится).
type SkifPartnerSnapshotService struct {
	db   *gorm.DB // public/main: skif_company_statuses, billing_plans, companies
	cron *cron.Cron
}

func NewSkifPartnerSnapshotService(db *gorm.DB) *SkifPartnerSnapshotService {
	return &SkifPartnerSnapshotService{db: db}
}

// Start запускает daily-cron (00:45 UTC — после Axenta-снимков 00:30, до billing 01:00).
// Использует database.DB как public-источник.
func (s *SkifPartnerSnapshotService) Start() error {
	c := cron.New(cron.WithLocation(time.UTC), cron.WithChain(cron.Recover(cron.DefaultLogger)))
	_, err := c.AddFunc("45 0 * * *", func() {
		// Снимок за ВЧЕРА — завершившийся день (сегодня ещё идёт, биллить нельзя). N=08.06→07.06.
		yesterday := time.Now().UTC().AddDate(0, 0, -1)
		if _, e := s.GenerateForAllTenants(yesterday); e != nil {
			log.Printf("⚠️ SKIF partner snapshot cron: %v", e)
		}
	})
	if err != nil {
		return err
	}
	c.Start()
	s.cron = c
	return nil
}

// aggregateDealer возвращает (total, active) объектов дилера за «сейчас»:
// total = Σ units всех компаний дилера, active = Σ units компаний в статусе ACTIVE.
func (s *SkifPartnerSnapshotService) aggregateDealer(connID uint, dealerUUID string) (total, active int) {
	s.db.Table("skif_company_statuses").
		Where("connection_id = ? AND skif_dealer_id = ?", connID, dealerUUID).
		Select("COALESCE(SUM(units_count), 0)").Row().Scan(&total)
	s.db.Table("skif_company_statuses").
		Where("connection_id = ? AND skif_dealer_id = ? AND company_status = ?", connID, dealerUUID, "ACTIVE").
		Select("COALESCE(SUM(units_count), 0)").Row().Scan(&active)
	return total, active
}

// secondaryActiveCount — НЕЗАВИСИМЫЙ второй счёт активных для cross-source сверки (Ф1).
// Основной active = Σ units_count из skif_company_statuses (поле API auth_admin_query).
// Второй = COUNT строк skif_units (отдельная выгрузка SyncUnits) активных компаний дилера —
// другой API-контур, ловит расхождение между «заявленным» и «фактическим» числом юнитов.
// -1 = посчитать не удалось → guard пропустит cross-source.
func (s *SkifPartnerSnapshotService) secondaryActiveCount(connID uint, dealerUUID string) int {
	activeCompanies := s.db.Table("skif_company_statuses").
		Select("skif_company_id").
		Where("connection_id = ? AND skif_dealer_id = ? AND company_status = ?", connID, dealerUUID, "ACTIVE")
	var cnt int64
	if err := s.db.Table("skif_units").
		Where("connection_id = ? AND skif_deleted_at IS NULL AND skif_company_id IN (?)", connID, activeCompanies).
		Count(&cnt).Error; err != nil {
		return -1
	}
	return int(cnt)
}

// GenerateForAllTenants — daily проход: пишет SKIF-снимки за date по всем активным тенантам.
func (s *SkifPartnerSnapshotService) GenerateForAllTenants(date time.Time) (int, error) {
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
			log.Printf("⚠️ SKIF partner snapshots tenant %d: %v", company.ID, err)
			continue
		}
		total += n
	}
	s.db.Exec("SET search_path TO public")
	log.Printf("✅ SKIF partner snapshots: создано/обновлено %d за %s", total, date.Format("2006-01-02"))
	return total, nil
}

// GenerateForTenant — снимки за date для active SKIF-партнёрских договоров одного тенанта.
func (s *SkifPartnerSnapshotService) GenerateForTenant(tenantDB *gorm.DB, date time.Time) (int, error) {
	var contracts []models.Contract
	if err := tenantDB.
		Where("contract_type = ? AND status = ? AND partner_source = ?", "partner", "active", "skif").
		Where("partner_external_id <> ''").
		Find(&contracts).Error; err != nil {
		return 0, err
	}

	day := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	created := 0
	for i := range contracts {
		c := &contracts[i]
		// Энфорс срока договора: не пишем снимок вне [start_date, end_date] (end_date NULL = open).
		if !c.IsActiveOn(day) {
			continue
		}
		if c.TariffPlanID == nil {
			log.Printf("⚠️ SKIF договор %d без тарифа, пропуск", c.ID)
			continue
		}
		// Тариф из public billing_plans.
		var plan models.BillingPlan
		if err := s.db.Where("id = ?", *c.TariffPlanID).First(&plan).Error; err != nil {
			log.Printf("⚠️ SKIF договор %d: тариф %d не найден: %v", c.ID, *c.TariffPlanID, err)
			continue
		}

		// Подтверждённый вручную снимок заморожен — cron его не перезатирает.
		if partnerSnapshotIsApproved(tenantDB, "skif", c.PartnerConnectionID, c.PartnerExternalID, c.ID, day) {
			log.Printf("🔒 SKIF снимок договора %d на %s подтверждён вручную — пропуск", c.ID, day.Format("2006-01-02"))
			continue
		}

		total, active := s.aggregateDealer(c.PartnerConnectionID, c.PartnerExternalID)

		snap := models.PartnerDailySnapshot{
			AdminAccountID:     c.AdminAccountID,
			CompanyID:          c.CompanyID,
			ContractID:         c.ID,
			SnapshotDate:       day,
			PartnerCompanyID:   0, // у SKIF нет Axenta company id
			PartnerSource:      "skif",
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
			// Ф1 двойная сверка: cross-source второй счёт из skif_units (другой контур).
			VerifySecondaryCount: s.secondaryActiveCount(c.PartnerConnectionID, c.PartnerExternalID),
			Notes:                "SKIF partner snapshot (Ф1): Σ units компаний дилера, active = company_status ACTIVE",
		}

		// BeforeCreate досчитает daily_price/cost_before_discount/discount_amount/daily_cost.
		// OnConflict UpdateAll = настоящий upsert (Codex F5), идемпотентно при ретраях.
		if err := tenantDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "partner_source"}, {Name: "connection_id"}, {Name: "partner_external_id"},
				{Name: "contract_id"}, {Name: "snapshot_date"},
			},
			UpdateAll: true,
		}).Create(&snap).Error; err != nil {
			log.Printf("⚠️ SKIF снимок договора %d: %v", c.ID, err)
			continue
		}
		created++
		log.Printf("✅ SKIF снимок: договор %d дилер %s — %d/%d объектов (active/total)", c.ID, c.PartnerExternalID, active, total)
	}
	return created, nil
}
