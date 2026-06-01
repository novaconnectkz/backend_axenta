// Б2 (Фаза 1): нормализующий backfill billing_mode/credit_limit с контрагента на договор.
// Закрывает исторический случай §1.4 «лимит жил ТОЛЬКО на cp» перед снятием mirror: договор
// prepaid/limit=0 при cp postpaid/limit>0 стал бы при чтении с Contract мгновенным prepaid-должником.
//
// УЗКИЙ предикат (решение §1.5(в) ГИБРИД — НЕ копируем cp→все договоры, иначе Σthreshold ×N):
//
//	c.credit_limit=0 AND c.billing_mode='prepaid' AND cp.credit_limit>0 AND cp.billing_mode='postpaid'
//
// → выставляем договору billing_mode='postpaid', credit_limit=cp.credit_limit (нормализ. как write-путь).
//
// Идемпотентно (после прогона предикат пуст). DRY-RUN по умолчанию, запись — --apply.
// Запуск НА хосте (читает .env): go build → scp → ./bin [--schema tenant_186] [--apply]
package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"flag"
	"log"
)

func main() {
	schemaFlag := flag.String("schema", "", "конкретная tenant-схема (пусто = все tenant_%)")
	apply := flag.Bool("apply", false, "выполнить запись (по умолчанию dry-run)")
	flag.Parse()

	if _, err := config.LoadConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("db: %v", err)
	}

	schemas := []string{}
	if *schemaFlag != "" {
		schemas = append(schemas, *schemaFlag)
	} else {
		if err := database.DB.Raw(
			"SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tenant_%' ORDER BY 1",
		).Scan(&schemas).Error; err != nil {
			log.Fatalf("список схем: %v", err)
		}
	}
	log.Printf("📋 backfill cp→contract: схем=%d apply=%v", len(schemas), *apply)

	// Кандидат — mode-mismatch «cp postpaid / договор prepaid» (cp исторически был авторитетом).
	// НЕ ограничиваем cp.credit_limit>0: postpaid с лимитом 0 валиден, и режим влияет на charge-
	// тайминг сам по себе (Codex HIGH) → выравниваем mode даже при нулевом лимите. Tenant-скоуп
	// (admin+company) в условии связки — defensive против stale/cross-tenant counterparty_id (Codex LOW).
	const candMatch = `cp.id=c.counterparty_id AND cp.admin_account_id=c.admin_account_id AND cp.company_id=c.company_id`
	const candJoin = `JOIN public.counterparties cp ON ` + candMatch
	const candPredicate = `c.contract_type='client' AND c.deleted_at IS NULL
		AND c.billing_mode='prepaid' AND cp.billing_mode='postpaid'`

	totalCand, totalUpdated := 0, 0
	for _, sch := range schemas {
		tdb, err := database.ConnectToTenant(sch)
		if err != nil {
			log.Printf("⚠️ %s: connect: %v — пропуск", sch, err)
			continue
		}

		// 1) Сколько кандидатов
		var cand int64
		if err := tdb.Raw(`SELECT count(*) FROM contracts c ` + candJoin + `
			WHERE ` + candPredicate).Scan(&cand).Error; err != nil {
			log.Printf("⚠️ %s: count кандидатов: %v", sch, err)
			continue
		}
		if cand == 0 {
			log.Printf("✅ %s: кандидатов 0 — no-op", sch)
			continue
		}
		log.Printf("🔧 %s: кандидатов %d", sch, cand)
		totalCand += int(cand)

		// перечислить для протокола
		type row struct {
			ID      uint
			CpLimit string
			CpMode  string
		}
		var rows []row
		_ = tdb.Raw(`SELECT c.id, cp.credit_limit AS cp_limit, cp.billing_mode AS cp_mode
			FROM contracts c ` + candJoin + `
			WHERE ` + candPredicate).Scan(&rows).Error
		for _, r := range rows {
			log.Printf("    договор %d → postpaid, limit=%s (с cp)", r.ID, r.CpLimit)
		}

		if !*apply {
			continue
		}

		// 2) UPDATE — нормализует как write-путь (mode=postpaid → лимит сохраняется)
		res := tdb.Exec(`UPDATE contracts c
			SET billing_mode='postpaid', credit_limit=cp.credit_limit
			FROM public.counterparties cp
			WHERE ` + candMatch + ` AND ` + candPredicate)
		if res.Error != nil {
			log.Printf("❌ %s: UPDATE: %v", sch, res.Error)
			continue
		}
		log.Printf("✅ %s: обновлено %d", sch, res.RowsAffected)
		totalUpdated += int(res.RowsAffected)

		// 3) Assert: mode-mismatch погашен (0 договоров prepaid при cp postpaid)
		var left int64
		if err := tdb.Raw(`SELECT count(*) FROM contracts c ` + candJoin + `
			WHERE ` + candPredicate).Scan(&left).Error; err != nil {
			log.Printf("⚠️ %s: assert: %v", sch, err)
		} else if left != 0 {
			log.Printf("❌ %s: ASSERT FAIL — осталось %d договоров prepaid при cp postpaid", sch, left)
		} else {
			log.Printf("✅ %s: assert OK (0 остаточных)", sch)
		}
	}

	if *apply {
		log.Printf("🏁 ИТОГО: кандидатов %d, обновлено %d", totalCand, totalUpdated)
	} else {
		log.Printf("🏁 DRY-RUN: кандидатов %d (запись через --apply)", totalCand)
	}
}
