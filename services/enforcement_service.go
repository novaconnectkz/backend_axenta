package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// B1: EnforcementService — физическая блокировка/разблокировка учётки в GPS-системе за неоплату
// (МЕТОД 1: учётка целиком). SHADOW-FIRST: при mode='shadow' резолвит цель, пишет строку
// billing_enforcement_actions «БЫ заблокировал X» + лог, но НЕ дёргает внешний API. live —
// реальный вызов (отдельный гейт после ревью shadow-логов). Вызывается из sweep хуками.
//
// blast radius: реально режет доступ клиентам в GPS, почти необратимо. Любая ошибка резолва =
// заблокировали не ту учётку. Поэтому: shadow по умолчанию, ForeignObjects-метрика, резолв
// владельца на лету (не из кэша). concepts/billing-enforcement-layer.
type EnforcementService struct {
	enabled         bool
	mode            string // shadow | live
	decisionVersion int
	serverTok       *AxentaServerToken // инжектируется из main.go (тот же singleton, что в api)
}

// NewEnforcementService — флаги из env (как ENABLE_LEDGER_CHARGE_SCHEDULER). По умолчанию
// enabled=false (полный no-op), при enabled — mode=shadow (ноль реальных вызовов).
func NewEnforcementService(serverTok *AxentaServerToken) *EnforcementService {
	mode := os.Getenv("BILLING_ENFORCEMENT_MODE")
	if mode == "" {
		mode = "shadow"
	}
	es := &EnforcementService{
		enabled:         os.Getenv("ENABLE_BILLING_ENFORCEMENT") == "true",
		mode:            mode,
		decisionVersion: 1,
		serverTok:       serverTok,
	}
	if es.enabled {
		log.Printf("🔒 EnforcementService: ВКЛЮЧЕН, mode=%s", es.mode)
	}
	return es
}

// Enabled — для re-assert гейта в sweep.
func (s *EnforcementService) Enabled() bool { return s != nil && s.enabled }
func (s *EnforcementService) ModeLive() bool { return s != nil && s.mode == "live" }

// enforcementTarget — резолвленная цель блокировки (одна учётка GPS).
type enforcementTarget struct {
	System            string
	ConnectionID      uint
	ExternalAccountID string
	ObjectsInContract int
	ForeignObjects    int  // чужие объекты той же учётки (blast radius — гасим и их при МЕТОД 1)
	Resolved          bool
	CacheMismatch     bool // co.owner_external_id разошёлся с текущим снапшотом (MoveAccount)
}

var enfTargetCols = []clause.Column{
	{Name: "suspension_id"}, {Name: "system"}, {Name: "connection_id"}, {Name: "external_account_id"},
}

func (s *EnforcementService) tenantDBForCompany(companyID uint) (*gorm.DB, error) {
	var schema string
	database.GetDB().Table("public.companies").Where("id = ?", companyID).Limit(1).Pluck("database_schema", &schema)
	if schema == "" {
		return nil, fmt.Errorf("нет database_schema для company %d", companyID)
	}
	return database.ConnectToTenant(schema)
}

// resolveAccountsForSuspension — приостановленный договор(ы) → GPS-учётки (МЕТОД 1).
// Резолв владельца НА ЛЕТУ из tenant-снапшота (стабилен при MoveAccount); денорм-колонка
// contract_objects.owner_external_id лишь сверяется (CacheMismatch). non-axenta → manual_review.
func (s *EnforcementService) resolveAccountsForSuspension(tenantDB *gorm.DB, susp models.BillingSuspension) []enforcementTarget {
	contractIDs := parseUintCSV(susp.AffectedContractIDs) // отбрасывает 0
	if susp.ContractID > 0 {
		contractIDs = append(contractIDs, susp.ContractID)
	}
	if len(contractIDs) == 0 {
		log.Printf("⚠️ ENFORCE: susp=%d active, но contractIDs пуст (битый AffectedContractIDs) — manual_review", susp.ID)
		return nil
	}

	var cos []models.ContractObject
	tenantDB.Where("contract_id IN ? AND deleted_at IS NULL", contractIDs).Find(&cos)

	var out []enforcementTarget
	acc := map[string]*enforcementTarget{}
	seenObj := map[uint]bool{} // Codex #1: дедуп object_id (дубли contract_objects → двойной счёт)
	nonAxenta := map[string]bool{}
	for _, co := range cos {
		if co.Source != "axenta" {
			if !nonAxenta[co.Source] {
				nonAxenta[co.Source] = true
				out = append(out, enforcementTarget{System: co.Source, Resolved: false})
			}
			continue
		}
		if seenObj[co.ObjectID] {
			continue
		}
		seenObj[co.ObjectID] = true
		// текущий владелец из снапшота той же tenant-схемы (НЕ public, НЕ кэш-колонка)
		var liveOwner string
		tenantDB.Table("axenta_object_snapshots").
			Where("external_object_id = ? AND axenta_deleted_at IS NULL", co.ObjectID).
			Limit(1).Pluck("account_external_id", &liveOwner)
		if liveOwner == "" {
			continue // объект исчез из снапшота — нечего блокировать
		}
		mismatch := co.OwnerExternalID != "" && co.OwnerExternalID != liveOwner
		t := acc[liveOwner]
		if t == nil {
			t = &enforcementTarget{System: "axenta", ConnectionID: 0, ExternalAccountID: liveOwner, Resolved: true, CacheMismatch: mismatch}
			acc[liveOwner] = t
		}
		t.CacheMismatch = t.CacheMismatch || mismatch
		t.ObjectsInContract++
	}
	for extID, t := range acc {
		var total int64
		tenantDB.Table("axenta_object_snapshots").
			Where("account_external_id = ? AND axenta_deleted_at IS NULL", extID).Count(&total)
		t.ForeignObjects = int(total) - t.ObjectsInContract
		if t.ForeignObjects < 0 {
			t.ForeignObjects = 0 // Codex #1: не уходить в минус при рассинхроне снапшота
		}
		out = append(out, *t)
	}
	// Codex #1: договор охватывает несколько Axenta-учёток — МЕТОД 1 погасит ВСЕ; логируем для видимости.
	if len(acc) > 1 {
		log.Printf("⚠️ ENFORCE: susp=%d резолвится в %d Axenta-учёток (МЕТОД 1 заблокирует все)", susp.ID, len(acc))
	}
	return out
}

// Enforce — после suspend-tx: заблокировать учётки приостановленного контрагента.
// shadow: лог + строка (physical_ok=false), без API. Не возвращает error — провал физики
// не откатывает CRM-статус (дотягивается re-assert по physical_ok=false).
func (s *EnforcementService) Enforce(susp models.BillingSuspension, tenantDB *gorm.DB) {
	if !s.Enabled() || tenantDB == nil {
		return
	}
	// tenantDB передаётся из sweep (та же per-company сессия) — не открываем новый пул
	// на каждый Enforce (Codex #5).
	targets := s.resolveAccountsForSuspension(tenantDB, susp)
	pub := database.GetDB()
	for _, t := range targets {
		if !t.Resolved {
			log.Printf("⚠️ ENFORCE manual_review: susp=%d system=%s (junction пуст/резолв не реализован)", susp.ID, t.System)
			continue
		}
		act := models.BillingEnforcementAction{
			SuspensionID: susp.ID, CompanyID: susp.CompanyID, Level: "account",
			System: t.System, ConnectionID: t.ConnectionID, ExternalAccountID: t.ExternalAccountID,
			Action: "block", Mode: s.mode, PhysicalOK: false, DecisionVersion: s.decisionVersion,
			ForeignObjects: t.ForeignObjects, CacheMismatch: t.CacheMismatch,
		}
		pub.Table("public.billing_enforcement_actions").
			Clauses(clause.OnConflict{Columns: enfTargetCols, DoNothing: true}).Create(&act)

		if s.mode == "shadow" {
			log.Printf("🟡 SHADOW WOULD BLOCK: system=%s conn=%d account=%s (debt=%s, в_договоре=%d, ПОСТОРОННИХ=%d, cache_mismatch=%v)",
				t.System, t.ConnectionID, t.ExternalAccountID, susp.DebtAmount.StringFixed(2), t.ObjectsInContract, t.ForeignObjects, t.CacheMismatch)
			continue
		}
		if act.ID == 0 { // OnConflict DoNothing → строка уже была: подтянуть для tryPhysicalBlock
			pub.Table("public.billing_enforcement_actions").
				Where("suspension_id=? AND system=? AND connection_id=? AND external_account_id=?",
					susp.ID, t.System, t.ConnectionID, t.ExternalAccountID).First(&act)
		}
		s.tryPhysicalBlock(pub, &act, susp.CompanyID, t, false)
	}
}

// Reactivate — после restore-tx: разблокировать ТОЛЬКО наши block-строки (physical_ok=true,
// не manual_override). shadow: только лог.
func (s *EnforcementService) Reactivate(susp models.BillingSuspension) {
	if !s.Enabled() {
		return
	}
	pub := database.GetDB()
	var acts []models.BillingEnforcementAction
	pub.Table("public.billing_enforcement_actions").
		Where("suspension_id = ? AND action = 'block' AND physical_ok = ? AND manual_override = ?", susp.ID, true, false).
		Find(&acts)
	for _, a := range acts {
		t := enforcementTarget{System: a.System, ConnectionID: a.ConnectionID, ExternalAccountID: a.ExternalAccountID}
		if s.mode == "shadow" {
			log.Printf("🟡 SHADOW WOULD UNBLOCK: system=%s account=%s", a.System, a.ExternalAccountID)
			continue
		}
		// Codex #4: НЕ разблокировать учётку, которую держит ДРУГАЯ активная suspension
		// (две неоплаты делят один Axenta-аккаунт → объекты из разных договоров).
		var heldByOther int64
		pub.Table("public.billing_enforcement_actions AS bea").
			Joins("JOIN billing_suspensions bs ON bs.id = bea.suspension_id").
			Where("bea.suspension_id <> ? AND bea.action='block' AND bea.physical_ok=? "+
				"AND bea.system=? AND bea.connection_id=? AND bea.external_account_id=? "+
				"AND bs.active=true AND bs.deleted_at IS NULL", a.SuspensionID, true, a.System, a.ConnectionID, a.ExternalAccountID).
			Count(&heldByOther)
		if heldByOther > 0 {
			log.Printf("⏭️ REACTIVATE skip: учётка system=%s account=%s держится др. активной suspension (%d) — не разблокируем", a.System, a.ExternalAccountID, heldByOther)
			continue
		}
		if err := s.physicalBlockAccount(susp.CompanyID, t, true); err != nil {
			log.Printf("❌ REACTIVATE FAIL: system=%s account=%s err=%v", a.System, a.ExternalAccountID, err)
			continue
		}
		now := time.Now()
		pub.Table("public.billing_enforcement_actions").Create(&models.BillingEnforcementAction{
			SuspensionID: susp.ID, CompanyID: susp.CompanyID, System: a.System, Level: "account",
			ConnectionID: a.ConnectionID, ExternalAccountID: a.ExternalAccountID,
			Action: "unblock", Mode: "live", PhysicalOK: true, EnforcedAt: &now,
		})
	}
}

// ReassertPending — отдельный проход (НЕ привязан к транзиту active→suspended): дотягивает
// live-блоки с physical_ok=false по всё ещё активным suspension. Вызывается в конце sweep.
func (s *EnforcementService) ReassertPending() {
	if !s.Enabled() || !s.ModeLive() {
		return
	}
	pub := database.GetDB()
	var pending []models.BillingEnforcementAction
	// Codex #2: НЕ фильтруем по mode — иначе строки, созданные в shadow-эпоху (mode='shadow',
	// physical_ok=false), после перехода на live никогда не исполнятся → должник останется
	// незаблокирован (silent leak). Берём любые недотянутые block-строки активных suspension;
	// tryPhysicalBlock в live их исполнит (промоут shadow→live).
	pub.Table("public.billing_enforcement_actions").
		Joins("JOIN billing_suspensions bs ON bs.id = billing_enforcement_actions.suspension_id").
		Where("billing_enforcement_actions.action='block' "+
			"AND billing_enforcement_actions.physical_ok=false AND billing_enforcement_actions.manual_override=false "+
			"AND bs.deleted_at IS NULL AND bs.active=true").
		Find(&pending)
	for i := range pending {
		a := &pending[i]
		t := enforcementTarget{System: a.System, ConnectionID: a.ConnectionID, ExternalAccountID: a.ExternalAccountID, ForeignObjects: a.ForeignObjects}
		// TODO(B1-live, Codex #8): перед live-вызовом атомарно claim'ить строку
		// (UPDATE ... WHERE physical_ok=false RETURNING id / advisory-lock), чтобы параллельные
		// проходы sweep не дёрнули Axenta API дважды на одну учётку. Гейт перед включением live.
		s.tryPhysicalBlock(pub, a, a.CompanyID, t, false)
	}
}

// tryPhysicalBlock — реальный вызов (live) с pre-check внешнего состояния (не перетирать ручное).
func (s *EnforcementService) tryPhysicalBlock(pub *gorm.DB, act *models.BillingEnforcementAction, companyID uint, t enforcementTarget, enable bool) {
	if act.ID == 0 {
		return
	}
	if cur, err := s.externalState(companyID, t); err == nil && cur == enable {
		now := time.Now()
		pub.Table("public.billing_enforcement_actions").Where("id = ?", act.ID).
			Updates(map[string]any{"physical_ok": true, "enforced_at": &now})
		return
	}
	if err := s.physicalBlockAccount(companyID, t, enable); err != nil {
		pub.Table("public.billing_enforcement_actions").Where("id = ?", act.ID).
			Update("last_error", err.Error())
		log.Printf("❌ ENFORCE live FAIL: system=%s account=%s err=%v", t.System, t.ExternalAccountID, err)
		return
	}
	now := time.Now()
	pub.Table("public.billing_enforcement_actions").Where("id = ?", act.ID).
		Updates(map[string]any{"physical_ok": true, "enforced_at": &now, "last_error": ""})
	log.Printf("✅ ENFORCE blocked: system=%s account=%s", t.System, t.ExternalAccountID)
}

// physicalBlockAccount — диспетчер. B1: реален только Axenta; skif/wialon/gelios — manual_review
// (нет связи client-договор→юнит, B-link позже) → ошибка, до них в B1 не доходит (Resolved=false).
func (s *EnforcementService) physicalBlockAccount(companyID uint, t enforcementTarget, enable bool) error {
	switch t.System {
	case "axenta":
		return s.blockAxentaAccount(context.Background(), companyID, t.ExternalAccountID, enable)
	default:
		return fmt.Errorf("enforcement для system=%q не реализован в B1 (B-link)", t.System)
	}
}

// externalState — фактическое «активен ли» во внешней системе (pre-check). B1: не реализован
// → ошибка → tryPhysicalBlock пропускает pre-check и пытается заблокировать (идемпотентно).
func (s *EnforcementService) externalState(companyID uint, t enforcementTarget) (bool, error) {
	return false, fmt.Errorf("externalState не реализован в B1")
}
