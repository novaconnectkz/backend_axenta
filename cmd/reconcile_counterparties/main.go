// Phase C / C1 — reconciliation-гейт (READ-ONLY). Проверяет целостность связки
// контрагент↔договор↔ledger ПЕРЕД флипом авторитета идентичности с Contract.client_* на
// Counterparty. Ничего не пишет, только SELECT. Exit-код != 0 при блокирующих расхождениях ИЛИ
// при ошибке любой проверки/подключения (fail-closed) — чтобы работать как гейт в runbook/CI.
//
// Раскладка: contracts — в tenant-схемах (ConnectToTenant ставит search_path); counterparties +
// ledger_entries — в public. Схема tenant_<companyID> = одна company → ledger скоупим к компаниям,
// чьи договоры живут в текущей схеме (`l.company_id IN (SELECT company_id FROM contracts)`), иначе
// public-ledger чужой компании ложно матчит одноимённый contract.id в этой схеме (cross-tenant id-
// коллизия) и даёт дубль-репорт. Джойн ledger↔contract — ВСЕГДА с company+admin совпадением.
//
// Проверки (BLOCK закрывает гейт, WARN — нет):
//   1. STAMP-mismatch (BLOCK): ledger.counterparty_id <> contract.counterparty_id (тот же company+admin).
//      Денорм-штамп ledger разошёлся с текущим cp договора → per-cp баланс соврёт после флипа.
//   2. ORPHAN client-договоры (BLOCK): contract_type='client' AND counterparty_id=0 — перевести
//      читателя не на что.
//   3. UNSTAMPED ledger (BLOCK): ledger.counterparty_id=0 при contract.counterparty_id<>0 — backfill-дыра.
//   4. ORPHAN ledger (BLOCK): ledger.contract_id<>0, но договора с таким id в его company нет —
//      inner-join её прячет, а после флипа баланс по contract.cp потеряет эти строки.
//   5. MISSING cp по договору (BLOCK): client-договор с counterparty_id<>0, но active cp того же
//      (admin,company) нет → читатель после флипа получит NULL/чужую идентичность.
//   6. MISSING cp по ledger (BLOCK): ledger.counterparty_id<>0, но active cp того же (admin,company) нет.
//   7. SNAPSHOT-drift (WARN): contract.client_inn<>cp.tax_id или client_name<>cp.name — денорм
//      разошёлся с источником, риск отображения (баланс цел).
//   8. DUP cp по идентичности (BLOCK, public): (admin,company,id_type,tax_id) при tax_id<>'' → >1 cp.
//   9. DUP cp legacy по имени (WARN, public): tax_id='' → один lower(trim(name)) на (admin,company) >1.
//
// Запуск НА хосте (читает .env): go build → scp → ./bin [--schema tenant_186] [--company N]
package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/shopspring/decimal"
)

var companyFlag uint

// companyFilter — опциональный фильтр по company для алиаса таблицы (uint → инъекции нет).
func companyFilter(alias string) string {
	if companyFlag == 0 {
		return ""
	}
	if alias == "" {
		return fmt.Sprintf(" AND company_id=%d", companyFlag)
	}
	return fmt.Sprintf(" AND %s.company_id=%d", alias, companyFlag)
}

func main() {
	schemaFlag := flag.String("schema", "", "конкретная tenant-схема (пусто = все tenant_%)")
	cf := flag.Uint("company", 0, "конкретная company_id (0 = все)")
	driftSample := flag.Int("drift-sample", 10, "сколько примеров snapshot-drift печатать на схему")
	flag.Parse()
	companyFlag = *cf

	if _, err := config.LoadConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("db: %v", err)
	}
	gdb := database.DB

	schemas := []string{}
	if *schemaFlag != "" {
		schemas = append(schemas, *schemaFlag)
	} else if err := gdb.Raw(
		"SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tenant_%' ORDER BY 1",
	).Scan(&schemas).Error; err != nil {
		log.Fatalf("список схем: %v", err)
	}
	log.Printf("📋 reconcile_counterparties (READ-ONLY): схем=%d company=%d", len(schemas), companyFlag)

	blocking := 0
	warns := 0
	// errBlock — любая ошибка проверки/подключения закрывает гейт (fail-closed), а не пропускает молча.
	errBlock := func(where string, err error) {
		log.Printf("  ❌ ОШИБКА %s: %v — гейт закрыт (fail-closed)", where, err)
		blocking++
	}

	// ── [8] дубли cp по идентичности (public, глобально) ──
	{
		type dup struct {
			Admin, Company uint
			IDType, TaxID  string
			N              int
		}
		var dups []dup
		q := `SELECT admin_account_id AS admin, company_id AS company, id_type, tax_id, count(*) AS n
			FROM public.counterparties
			WHERE deleted_at IS NULL AND tax_id <> ''` + companyFilter("") + `
			GROUP BY admin_account_id, company_id, id_type, tax_id
			HAVING count(*) > 1 ORDER BY n DESC`
		if err := gdb.Raw(q).Scan(&dups).Error; err != nil {
			errBlock("[8] дубли cp", err)
		} else if len(dups) == 0 {
			log.Printf("✅ [8] дубли cp по идентичности: нет")
		} else {
			log.Printf("❌ [8] ДУБЛИ cp (admin,company,id_type,tax_id) → merge (C2):")
			for _, d := range dups {
				log.Printf("      admin=%d company=%d %s=%s → %d cp", d.Admin, d.Company, d.IDType, d.TaxID, d.N)
				blocking++
			}
		}
	}

	// ── [9] дубли cp legacy по имени (public, WARN) ──
	{
		type dup struct {
			Admin, Company uint
			Norm           string
			N              int
		}
		var dups []dup
		q := `SELECT admin_account_id AS admin, company_id AS company, lower(trim(name)) AS norm, count(*) AS n
			FROM public.counterparties
			WHERE deleted_at IS NULL AND (tax_id = '' OR tax_id IS NULL) AND trim(name) <> ''` + companyFilter("") + `
			GROUP BY admin_account_id, company_id, lower(trim(name))
			HAVING count(*) > 1 ORDER BY n DESC`
		if err := gdb.Raw(q).Scan(&dups).Error; err != nil {
			errBlock("[9] дубли cp по имени", err)
		} else if len(dups) == 0 {
			log.Printf("✅ [9] дубли cp legacy (по имени, tax_id=''): нет")
		} else {
			log.Printf("⚠️ [9] ДУБЛИ cp legacy по имени (tax_id=''): %d групп — проверить перед флипом:", len(dups))
			for _, d := range dups {
				log.Printf("      admin=%d company=%d name~%q → %d cp", d.Admin, d.Company, d.Norm, d.N)
				warns++
			}
		}
	}

	// ledgerScope — скоуп public-ledger к компаниям текущей схемы (cross-tenant защита) + опц. --company.
	const ledgerScope = ` AND l.company_id IN (SELECT DISTINCT company_id FROM contracts WHERE deleted_at IS NULL)`
	// joinLC — джойн ledger↔contract по id И company И admin (запрет cross-tenant id-коллизии).
	const joinLC = ` AND c.company_id=l.company_id AND c.admin_account_id=l.admin_account_id`

	for _, sch := range schemas {
		tdb, err := database.ConnectToTenant(sch)
		if err != nil {
			log.Printf("❌ %s: connect: %v — гейт закрыт (fail-closed, не пропуск)", sch, err)
			blocking++
			continue
		}
		log.Printf("── %s ──", sch)

		// ── [1] stamp-mismatch ──
		{
			type row struct {
				Company  uint
				Currency string
				Entries  int
				MoveAbs  decimal.Decimal
			}
			var rows []row
			q := `SELECT l.company_id AS company, l.currency, count(*) AS entries,
					COALESCE(SUM(abs(l.amount)),0) AS move_abs
				FROM public.ledger_entries l
				JOIN contracts c ON c.id = l.contract_id` + joinLC + `
				WHERE l.deleted_at IS NULL AND c.deleted_at IS NULL AND l.contract_id <> 0
					AND l.counterparty_id <> c.counterparty_id` + ledgerScope + companyFilter("l") + `
				GROUP BY l.company_id, l.currency ORDER BY l.company_id, l.currency`
			if err := tdb.Raw(q).Scan(&rows).Error; err != nil {
				errBlock(sch+" [1] stamp-mismatch", err)
			} else if len(rows) == 0 {
				log.Printf("  ✅ [1] ledger stamp == contract.cp")
			} else {
				for _, r := range rows {
					log.Printf("  ❌ [1] STAMP-MISMATCH company=%d %s: %d проводок, |переезд|=%s",
						r.Company, r.Currency, r.Entries, r.MoveAbs.StringFixed(2))
					blocking++
				}
			}
		}

		// ── [2] orphan client-договоры (cp=0) ──
		{
			type row struct {
				Company uint
				N       int
				IDs     string
			}
			var rows []row
			q := `SELECT company_id AS company, count(*) AS n,
					string_agg(id::text, ',' ORDER BY id) AS ids
				FROM contracts
				WHERE deleted_at IS NULL AND contract_type='client' AND counterparty_id=0` + companyFilter("") + `
				GROUP BY company_id ORDER BY company_id`
			if err := tdb.Raw(q).Scan(&rows).Error; err != nil {
				errBlock(sch+" [2] orphan-договоры", err)
			} else if len(rows) == 0 {
				log.Printf("  ✅ [2] orphan client-договоров (cp=0): нет")
			} else {
				for _, r := range rows {
					ids := r.IDs
					if len(ids) > 200 {
						ids = ids[:200] + "…"
					}
					log.Printf("  ❌ [2] ORPHAN company=%d: %d договоров без cp [%s]", r.Company, r.N, ids)
					blocking++
				}
			}
		}

		// ── [3] unstamped ledger (entry cp=0, contract cp<>0) ──
		{
			type row struct {
				Company  uint
				Currency string
				Entries  int
				Abs      decimal.Decimal
			}
			var rows []row
			q := `SELECT l.company_id AS company, l.currency, count(*) AS entries,
					COALESCE(SUM(abs(l.amount)),0) AS abs
				FROM public.ledger_entries l
				JOIN contracts c ON c.id = l.contract_id` + joinLC + `
				WHERE l.deleted_at IS NULL AND c.deleted_at IS NULL AND l.contract_id <> 0
					AND l.counterparty_id = 0 AND c.counterparty_id <> 0` + ledgerScope + companyFilter("l") + `
				GROUP BY l.company_id, l.currency ORDER BY l.company_id, l.currency`
			if err := tdb.Raw(q).Scan(&rows).Error; err != nil {
				errBlock(sch+" [3] unstamped ledger", err)
			} else if len(rows) == 0 {
				log.Printf("  ✅ [3] unstamped ledger (cp=0 при cp договора<>0): нет")
			} else {
				for _, r := range rows {
					log.Printf("  ❌ [3] UNSTAMPED company=%d %s: %d проводок, |сумма|=%s (нужен backfill cp)",
						r.Company, r.Currency, r.Entries, r.Abs.StringFixed(2))
					blocking++
				}
			}
		}

		// ── [4] orphan ledger: contract_id<>0 без договора в его company ──
		{
			type row struct {
				Company  uint
				Currency string
				Entries  int
				Abs      decimal.Decimal
			}
			var rows []row
			q := `SELECT l.company_id AS company, l.currency, count(*) AS entries,
					COALESCE(SUM(abs(l.amount)),0) AS abs
				FROM public.ledger_entries l
				LEFT JOIN contracts c ON c.id = l.contract_id` + joinLC + ` AND c.deleted_at IS NULL
				WHERE l.deleted_at IS NULL AND l.contract_id <> 0 AND c.id IS NULL` + ledgerScope + companyFilter("l") + `
				GROUP BY l.company_id, l.currency ORDER BY l.company_id, l.currency`
			if err := tdb.Raw(q).Scan(&rows).Error; err != nil {
				errBlock(sch+" [4] orphan ledger", err)
			} else if len(rows) == 0 {
				log.Printf("  ✅ [4] orphan ledger (contract_id без договора): нет")
			} else {
				for _, r := range rows {
					log.Printf("  ❌ [4] ORPHAN-LEDGER company=%d %s: %d проводок, |сумма|=%s (договор отсутствует/удалён)",
						r.Company, r.Currency, r.Entries, r.Abs.StringFixed(2))
					blocking++
				}
			}
		}

		// ── [5] missing cp по договору: client cp<>0 без active cp того же (admin,company) ──
		{
			type row struct {
				Company uint
				N       int
				IDs     string
			}
			var rows []row
			q := `SELECT c.company_id AS company, count(*) AS n,
					string_agg(c.id::text, ',' ORDER BY c.id) AS ids
				FROM contracts c
				LEFT JOIN public.counterparties cp
					ON cp.id=c.counterparty_id AND cp.admin_account_id=c.admin_account_id
					AND cp.company_id=c.company_id AND cp.deleted_at IS NULL
				WHERE c.deleted_at IS NULL AND c.contract_type='client' AND c.counterparty_id<>0
					AND cp.id IS NULL` + companyFilter("c") + `
				GROUP BY c.company_id ORDER BY c.company_id`
			if err := tdb.Raw(q).Scan(&rows).Error; err != nil {
				errBlock(sch+" [5] missing cp договора", err)
			} else if len(rows) == 0 {
				log.Printf("  ✅ [5] cp договора существует (active, тот же scope)")
			} else {
				for _, r := range rows {
					ids := r.IDs
					if len(ids) > 200 {
						ids = ids[:200] + "…"
					}
					log.Printf("  ❌ [5] MISSING-CP company=%d: %d договоров на удалённый/чужой cp [%s]", r.Company, r.N, ids)
					blocking++
				}
			}
		}

		// ── [6] missing cp по ledger: cp<>0 без active cp того же (admin,company) ──
		{
			type row struct {
				Company  uint
				Currency string
				Entries  int
				Abs      decimal.Decimal
			}
			var rows []row
			q := `SELECT l.company_id AS company, l.currency, count(*) AS entries,
					COALESCE(SUM(abs(l.amount)),0) AS abs
				FROM public.ledger_entries l
				LEFT JOIN public.counterparties cp
					ON cp.id=l.counterparty_id AND cp.admin_account_id=l.admin_account_id
					AND cp.company_id=l.company_id AND cp.deleted_at IS NULL
				WHERE l.deleted_at IS NULL AND l.counterparty_id<>0 AND cp.id IS NULL` + ledgerScope + companyFilter("l") + `
				GROUP BY l.company_id, l.currency ORDER BY l.company_id, l.currency`
			if err := tdb.Raw(q).Scan(&rows).Error; err != nil {
				errBlock(sch+" [6] missing cp ledger", err)
			} else if len(rows) == 0 {
				log.Printf("  ✅ [6] cp ledger существует (active, тот же scope)")
			} else {
				for _, r := range rows {
					log.Printf("  ❌ [6] MISSING-CP-LEDGER company=%d %s: %d проводок, |сумма|=%s (cp удалён/чужой)",
						r.Company, r.Currency, r.Entries, r.Abs.StringFixed(2))
					blocking++
				}
			}
		}

		// ── [7] snapshot-drift — УДАЛЁН (C4b): денорм client_* дропнуты, дрейфовать
		// нечему (идентичность только на Counterparty). Чек оставлен бы 42P01 на
		// дропнутых колонках. Linkage-целостность покрыта чеками [1]-[6] выше.
		_ = driftSample
	}

	log.Printf("🏁 ИТОГО: блокирующих находок=%d, warn=%d", blocking, warns)
	if blocking > 0 {
		log.Printf("🔴 ГЕЙТ ЗАКРЫТ — флип Phase C нельзя. Разобрать блокеры выше (merge/backfill/привязка cp).")
		os.Exit(1)
	}
	log.Printf("🟢 ГЕЙТ ОТКРЫТ — блокеров нет. Drift/legacy-warn (если есть) — проверить, баланс цел.")
}
