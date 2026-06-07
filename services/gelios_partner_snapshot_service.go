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

// GeliosPartnerSnapshotService — Ф3 partner billing для GELIOS-дилеров.
//
// Модель данных GELIOS (разведка 2026-05-23, см.
// wiki/concepts/gelios-partner-billing-ph3-recon): сущности «аккаунт» нет —
// всё дерево users, биллинг на юзере. Дилер = is_admin || units_count>0.
// units_count хранится прямо в gelios_users (sync_users заполняет с
// inclbillinf=true). Иерархия creator резолвится (3/4 deep), но per-account
// модель (как Wialon Ф2) не учитывает поддерево — каждый дилер биллится за
// СВОИ прямые units_count, не за объекты вложенных под-дилеров.
//
// 100% guaranteed-data режим (по запросу пользователя):
//  1. Перед чтением gelios_users делаем force SyncGeliosUsers для каждого
//     уникального connection_id среди partner-договоров тенанта — данные
//     максимально свежие на момент снимка.
//  2. ВСЕГДА пишем строку в partner_daily_snapshots, даже при units_count=0
//     или отсутствующем user. status='completed' если данные есть,
//     status='warning' + поясняющие notes иначе — KPI получает корректное
//     состояние «сегодня 0 юнитов» (не подменяет старым валидным снимком).
//
// Снимок в общую partner_daily_snapshots с partner_source='gelios' (Ф0).
// partner_external_id = gelios_user_id (varchar, как в БД).
type GeliosPartnerSnapshotService struct {
	db   *gorm.DB // public: gelios_users, gelios_connections, billing_plans, companies
	cron *cron.Cron
}

func NewGeliosPartnerSnapshotService(db *gorm.DB) *GeliosPartnerSnapshotService {
	return &GeliosPartnerSnapshotService{db: db}
}

// Start запускает daily-cron (00:55 UTC — после Wialon 00:50, до billing 01:00).
func (s *GeliosPartnerSnapshotService) Start() error {
	c := cron.New(cron.WithLocation(time.UTC), cron.WithChain(cron.Recover(cron.DefaultLogger)))
	_, err := c.AddFunc("55 0 * * *", func() {
		today := time.Now().UTC()
		if _, e := s.GenerateForAllTenants(today); e != nil {
			log.Printf("⚠️ GELIOS partner snapshot cron: %v", e)
		}
	})
	if err != nil {
		return err
	}
	c.Start()
	s.cron = c
	return nil
}

// forceSyncConnections запускает SyncGeliosUsers для каждого уникального
// connection_id переданных partner-договоров — гарантирует свежесть
// gelios_users.units_count перед snapshot. Ошибки sync не блокируют snapshot
// (пишем warning), per-conn lock внутри SyncGeliosUsers защищает от race с
// общим scheduler'ом.
func (s *GeliosPartnerSnapshotService) forceSyncConnections(contracts []models.Contract) {
	uniqConn := make(map[uint]struct{}, len(contracts))
	for i := range contracts {
		uniqConn[contracts[i].PartnerConnectionID] = struct{}{}
	}
	if len(uniqConn) == 0 {
		return
	}
	geliosSvc := NewGeliosService(s.db)
	for connID := range uniqConn {
		var conn models.GeliosConnection
		if err := s.db.Table("public.gelios_connections").
			Where("id = ? AND is_active = ?", connID, true).
			First(&conn).Error; err != nil {
			log.Printf("⚠️ GELIOS partner snapshot: conn %d не найдена/неактивна, пропуск sync: %v", connID, err)
			continue
		}
		n, err := geliosSvc.SyncGeliosUsers(&conn)
		if err != nil {
			log.Printf("⚠️ GELIOS partner snapshot: force-sync conn %d упал: %v (продолжаем со старыми данными)", connID, err)
			continue
		}
		log.Printf("✅ GELIOS partner snapshot: force-sync conn %d — %d юзеров", connID, n)
	}
}

// aggregateUser возвращает (units_count, ok, notes) GELIOS-партнёра по
// (connection_id, gelios_user_id). ok=false → user не найден в gelios_users
// (удалён в GELIOS / не синканный) → caller пишет warning-снимок с 0.
// total = active = units_count: per-account модель, breakdown активных по
// юнитам не доступен (как Wialon).
func (s *GeliosPartnerSnapshotService) aggregateUser(connID uint, geliosUserID string) (units int, ok bool, notes string) {
	var u models.GeliosUser
	err := s.db.Table("public.gelios_users").
		Where("connection_id = ? AND gelios_user_id = ?", connID, geliosUserID).
		First(&u).Error
	if err != nil {
		return 0, false, "пользователь не найден в gelios_users после re-sync (удалён в GELIOS или conn не синканный)"
	}
	if u.GeliosDeletedAt != nil {
		return 0, false, "пользователь soft-deleted в gelios_users (исчез из выгрузки)"
	}
	return u.UnitsCount, true, ""
}

// GenerateForAllTenants — daily проход по всем активным тенантам.
func (s *GeliosPartnerSnapshotService) GenerateForAllTenants(date time.Time) (int, error) {
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
			log.Printf("⚠️ GELIOS partner snapshots tenant %d: %v", company.ID, err)
			continue
		}
		total += n
	}
	s.db.Exec("SET search_path TO public")
	log.Printf("✅ GELIOS partner snapshots: создано/обновлено %d за %s", total, date.Format("2006-01-02"))
	return total, nil
}

// GenerateForTenant — снимки за date для active GELIOS-партнёрских договоров тенанта.
// 100% guarantee: ВСЕГДА пишет строку (status='warning' если данных нет).
func (s *GeliosPartnerSnapshotService) GenerateForTenant(tenantDB *gorm.DB, date time.Time) (int, error) {
	var contracts []models.Contract
	if err := tenantDB.
		Where("contract_type = ? AND status = ? AND partner_source = ?", "partner", "active", "gelios").
		Where("partner_external_id <> ''").
		Find(&contracts).Error; err != nil {
		return 0, err
	}

	// Force-sync gelios_users для всех уникальных connection_id — данные на момент
	// snapshot максимально свежие.
	s.forceSyncConnections(contracts)

	day := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	created := 0
	for i := range contracts {
		c := &contracts[i]
		// Энфорс срока договора: не пишем снимок вне [start_date, end_date] (end_date NULL = open).
		if !c.IsActiveOn(day) {
			continue
		}
		if c.TariffPlanID == nil {
			log.Printf("⚠️ GELIOS договор %d без тарифа, пропуск", c.ID)
			continue
		}
		var plan models.BillingPlan
		if err := s.db.Table("public.billing_plans").Where("id = ?", *c.TariffPlanID).First(&plan).Error; err != nil {
			log.Printf("⚠️ GELIOS договор %d: тариф %d не найден: %v", c.ID, *c.TariffPlanID, err)
			continue
		}

		// Подтверждённый вручную снимок заморожен — cron его не перезатирает.
		if partnerSnapshotIsApproved(tenantDB, "gelios", c.PartnerConnectionID, c.PartnerExternalID, c.ID, day) {
			log.Printf("🔒 GELIOS снимок договора %d на %s подтверждён вручную — пропуск", c.ID, day.Format("2006-01-02"))
			continue
		}

		units, ok, warnNotes := s.aggregateUser(c.PartnerConnectionID, c.PartnerExternalID)
		status := "completed"
		notes := "GELIOS partner snapshot (Ф3): прямой units_count юзера (per-account, без поддерева)"
		sourceWarn := ""
		if !ok {
			status = "warning"
			notes = "GELIOS partner snapshot (Ф3) WARNING: " + warnNotes + " — daily_cost=0"
			sourceWarn = warnNotes // Ф1: guard переведёт снимок в needs_review
		}

		snap := models.PartnerDailySnapshot{
			AdminAccountID:     c.AdminAccountID,
			CompanyID:          c.CompanyID,
			ContractID:         c.ID,
			SnapshotDate:       day,
			PartnerCompanyID:   0, // у GELIOS нет Axenta company id
			PartnerSource:      "gelios",
			ConnectionID:       c.PartnerConnectionID,
			PartnerExternalID:  c.PartnerExternalID,
			TariffPlanID:       plan.ID,
			MonthlyPrice:       plan.Price,
			TotalObjectsCount:  units,
			ActiveObjectsCount: units,
			DiscountType:       c.DiscountType,
			DiscountPercent:    c.GetDiscountPercent(units),
			DiscountFixed:      c.GetDiscountFixed(),
			Status:             status,
			// Ф1: cross-source -1 (нет чистого join units→user; continuity-guard + warning-сигнал). Ф2: события.
			VerifySecondaryCount: -1,
			SourceWarn:           sourceWarn,
			Notes:                notes,
		}

		if err := tenantDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "partner_source"}, {Name: "connection_id"}, {Name: "partner_external_id"},
				{Name: "contract_id"}, {Name: "snapshot_date"},
			},
			UpdateAll: true,
		}).Create(&snap).Error; err != nil {
			log.Printf("⚠️ GELIOS снимок договора %d: %v", c.ID, err)
			continue
		}
		created++
		log.Printf("✅ GELIOS снимок: договор %d юзер %s — %d units (status=%s)", c.ID, c.PartnerExternalID, units, status)
	}
	return created, nil
}
