// Б4 §5.2: data-rollback — свернуть N активных per-договор debt-suspension контрагента в ОДНУ
// pooled (contract_id=0) при откате флага per_contract_gating. Зеркало того, что делал бы старый
// pooled-sweep. Статусы договоров НЕ трогает (остаются suspended — pooled их покрывает).
//
// ПЕРЕ-Enforce от новой pooled-строки ПОСЛЕ создания, ДО гашения старых (heldByOther на старых
// suspension_id перестанет держать общую учётку при их resolve → без пере-Enforce следующий
// Reactivate ошибочно разблокировал бы). Enforcement в shadow → логирует, не трогает Axenta.
//
// Индекс НЕ меняем: idx_susp_debt_cp_ct сам держит pooled (cp,contract_id=0) уникальной per cp.
// Полный Б3-revert индекса (DROP cp_ct) — отдельная операция (и ensureBillingSchemaIntegrity
// пересоздаст cp_ct на следующем boot, так что ручной revert индекса бессмыслен без отката кода).
//
// DRY-RUN по умолчанию (печатает план + assert), запись — --apply.
// Запуск НА хосте (читает .env): go build → scp → ./bin [--company N] [--apply]
package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

func joinCSV(ids []uint) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(parts, ",")
}

func main() {
	companyFlag := flag.Uint("company", 0, "конкретная company_id (0 = все)")
	apply := flag.Bool("apply", false, "выполнить запись (по умолчанию dry-run)")
	flag.Parse()

	if _, err := config.LoadConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("db: %v", err)
	}
	db := database.DB

	// Enforcement (тот же singleton-паттерн, что scheduler/main). В shadow/off — Enforce логирует.
	enf := services.NewEnforcementService(
		services.NewAxentaServerToken(db, services.NewAxetnaClient("https://axenta.cloud/api", nil)),
	)

	// Активные per-договор debt-строки (contract_id<>0, cp<>0) — кандидаты на свёртку.
	type grp struct {
		admin, company, cp uint
	}
	var rows []models.BillingSuspension
	q := db.Table("public.billing_suspensions").
		Where("active=true AND deleted_at IS NULL AND reason='billing_debt' AND counterparty_id<>0 AND contract_id<>0")
	if *companyFlag != 0 {
		q = q.Where("company_id=?", *companyFlag)
	}
	if err := q.Find(&rows).Error; err != nil {
		log.Fatalf("выборка per-договор suspension: %v", err)
	}
	if len(rows) == 0 {
		log.Printf("✅ per-договор debt-suspension не найдено — сворачивать нечего (apply=%v)", *apply)
		return
	}

	byGrp := map[grp][]models.BillingSuspension{}
	for _, r := range rows {
		g := grp{r.AdminAccountID, r.CompanyID, r.CounterpartyID}
		byGrp[g] = append(byGrp[g], r)
	}
	log.Printf("📋 rollback N→1: групп (admin,company,cp)=%d, строк=%d, apply=%v", len(byGrp), len(rows), *apply)

	now := time.Now().UTC()
	collapsed := 0
	for g, susps := range byGrp {
		// Союз affected-договоров всех строк группы (для pooled.AffectedContractIDs).
		affSet := map[uint]bool{}
		for _, s := range susps {
			affSet[s.ContractID] = true
			for _, p := range strings.Split(s.AffectedContractIDs, ",") {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				var id uint
				if _, e := fmt.Sscanf(p, "%d", &id); e == nil && id != 0 {
					affSet[id] = true
				}
			}
		}
		affected := make([]uint, 0, len(affSet))
		for id := range affSet {
			affected = append(affected, id)
		}

		// DebtAmount pooled = глубина худшего суб-баланса cp (как считал pooled-sweep, §5.2 —
		// НЕ max per-contract Coverage).
		var subRows []struct {
			Currency string
			Bal      decimal.Decimal
		}
		db.Table("public.ledger_entries").
			Where("admin_account_id=? AND company_id=? AND counterparty_id=? AND deleted_at IS NULL", g.admin, g.company, g.cp).
			Select("currency, COALESCE(SUM(amount),0) AS bal").Group("currency").Scan(&subRows)
		worst := decimal.Zero
		for _, sr := range subRows {
			if sr.Bal.LessThan(worst) {
				worst = sr.Bal
			}
		}

		log.Printf("  cp=%d (admin=%d company=%d): %d per-договор строк → pooled, affected=[%s], debt=%s",
			g.cp, g.admin, g.company, len(susps), joinCSV(affected), worst.Abs().StringFixed(2))

		if !*apply {
			continue
		}

		// tenantDB обязателен ДО любых правок: resolveAccountsForSuspension читает tenant-таблицы.
		// Нет коннекта → пропускаем группу целиком (fail-closed: старые per-договор строки остаются
		// active, клиент заблокирован — безопаснее, чем разблок). Оператор повторит.
		schema := fmt.Sprintf("tenant_%d", g.company)
		var comp models.Company
		if db.Table("public.companies").Select("database_schema").First(&comp, g.company).Error == nil && comp.DatabaseSchema != "" {
			schema = comp.DatabaseSchema
		}
		tdb, e := database.ConnectToTenant(schema)
		if e != nil {
			log.Printf("  ⚠️ cp=%d: tenant %s недоступен — группа пропущена (старые строки целы)", g.cp, schema)
			continue
		}

		// ПОРЯДОК fail-closed (Gemini CRITICAL): create pooled → Enforce(pooled) → ТОЛЬКО ПОТОМ
		// resolve старых. Иначе при сбое Enforce старые block-actions осиротели бы (resolved
		// suspension_id, heldByOther их не видит), а pooled без block-action → ошибочный разблок.
		// Шаг 1: создать pooled (если уже есть от прерванного прошлого прогона — берём его, идемпотентность).
		var pooled models.BillingSuspension
		hasPooled := db.Table("public.billing_suspensions").
			Where("admin_account_id=? AND company_id=? AND counterparty_id=? AND contract_id=0 AND reason='billing_debt' AND active=true AND deleted_at IS NULL",
				g.admin, g.company, g.cp).First(&pooled).Error == nil
		if !hasPooled {
			pooled = models.BillingSuspension{
				AdminAccountID: g.admin, CompanyID: g.company, ContractID: 0, CounterpartyID: g.cp,
				Reason: "billing_debt", PreviousStatus: "active", DebtAmount: worst.Abs(),
				Active: true, SuspendedBy: "rollback", AffectedContractIDs: joinCSV(affected),
			}
			if e := db.Table("public.billing_suspensions").Create(&pooled).Error; e != nil {
				log.Printf("  ❌ cp=%d: создание pooled не удалось (дубль/гонка?): %v", g.cp, e)
				continue
			}
		}
		// Шаг 2: пере-Enforce от pooled ДО гашения старых (устанавливает block-action на новый id).
		enf.Enforce(pooled, tdb)
		// Шаг 3: погасить старые N (active=false) — теперь pooled держит блокировку.
		ids := make([]uint, len(susps))
		for i, s := range susps {
			ids[i] = s.ID
		}
		if e := db.Table("public.billing_suspensions").Where("id IN ?", ids).
			Updates(map[string]interface{}{"active": false, "resolved_at": now}).Error; e != nil {
			log.Printf("  ⚠️ cp=%d: pooled создан+enforced, но гашение старых не удалось: %v (повтори --apply)", g.cp, e)
			continue
		}
		collapsed++
	}

	// Assert: нет suspended client-договора без покрывающей активной debt-suspension.
	if *apply {
		log.Printf("🏁 свёрнуто групп: %d", collapsed)
	} else {
		log.Printf("🏁 DRY-RUN: к свёртке групп %d (запись --apply)", len(byGrp))
	}
}
