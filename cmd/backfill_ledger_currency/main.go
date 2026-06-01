// Datafix валюто-fix: приводит ledger_entries.currency к валюте договора для проводок,
// у которых валюта была захардкожена "RUB" (payment/migration_balance до фикса), а договор
// в другой валюте. Без этого валюто-слепой баланс схлопывал бы USD-charge и RUB-payment.
//
// БЕЗОПАСНОСТЬ:
//   - по умолчанию DRY-RUN (печатает план, ничего не пишет); запись — флаг --apply;
//   - идемпотентно: повтор не тронет уже исправленные (фильтр currency='RUB' против целевой);
//   - --apply делает snapshot затронутых строк в public.ledger_entries_bak_ccy ПЕРЕД UPDATE;
//   - scope строго (company_id из имени схемы) + per-contract (contract_id<>0). Платежи уровня
//     контрагента (contract_id=0) и мультивалютные cp НЕ трогаем — только репорт (валюта
//     неоднозначна, авто-UPDATE опасен);
//   - ПЕРЕД --apply на проде: pg_dump + остановить charge-крон (DISABLE_AXENTA_SYNC_SCHEDULER=true).
//     Гонять сперва на staging-копии прод-БД. На проде сегодня 0 не-RUB договоров → no-op.
//
// Запуск:
//   go run ./cmd/backfill_ledger_currency                  # dry-run по всем схемам
//   go run ./cmd/backfill_ledger_currency --schema tenant_186
//   go run ./cmd/backfill_ledger_currency --apply          # запись (после бэкапа!)
package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"flag"
	"log"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

var tenantSchemaRegex = regexp.MustCompile(`^tenant_[a-zA-Z0-9_]+$`)

func main() {
	apply := flag.Bool("apply", false, "выполнить запись (по умолчанию dry-run)")
	onlySchema := flag.String("schema", "", "ограничить одной tenant-схемой (например tenant_186)")
	flag.Parse()

	log.Println("🚀 Datafix: ledger_entries.currency ← валюта договора (валюто-fix)")
	if *apply {
		log.Println("⚠️  РЕЖИМ ЗАПИСИ (--apply). Убедись: pg_dump сделан, charge-крон остановлен!")
	} else {
		log.Println("🔍 DRY-RUN: только план. Для записи добавь --apply")
	}

	if _, err := config.LoadConfig(); err != nil {
		log.Fatalf("❌ Конфигурация: %v", err)
	}
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Подключение к БД: %v", err)
	}
	db := database.GetDB()
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Fatalf("❌ search_path public: %v", err)
	}

	schemas := targetSchemas(db, *onlySchema)
	if len(schemas) == 0 {
		log.Println("⚠️ Нет tenant-схем")
		return
	}
	log.Printf("📋 Схем к обработке: %d → %v", len(schemas), schemas)

	totalFixable, totalReportOnly := 0, 0
	for _, schema := range schemas {
		fixable, reportOnly := processSchema(db, schema, *apply)
		totalFixable += fixable
		totalReportOnly += reportOnly
	}
	verb := "к исправлению"
	if *apply {
		verb = "исправлено"
	}
	log.Printf("✅ Итог: %s проводок=%d; репорт-онли (contract_id=0/мультивалюта)=%d", verb, totalFixable, totalReportOnly)
	if totalReportOnly > 0 {
		log.Println("⚠️ Репорт-онли строки НЕ изменены — разнести валюту вручную (см. лог выше).")
	}
}

// targetSchemas — активные company-схемы + физические tenant_% (как RunMigrations).
func targetSchemas(db *gorm.DB, only string) []string {
	if only != "" {
		if !tenantSchemaRegex.MatchString(only) {
			log.Fatalf("❌ --schema=%q не проходит tenant_%%-валидацию", only)
		}
		return []string{only}
	}
	seen := map[string]bool{}
	out := []string{}
	var companies []models.Company
	if err := db.Find(&companies).Error; err == nil {
		for _, co := range companies {
			if !co.IsActive || co.DatabaseSchema == "" || seen[co.DatabaseSchema] {
				continue
			}
			if !tenantSchemaRegex.MatchString(co.DatabaseSchema) {
				continue
			}
			seen[co.DatabaseSchema] = true
			out = append(out, co.DatabaseSchema)
		}
	}
	var physical []string
	if err := db.Raw(`SELECT schema_name FROM information_schema.schemata
		WHERE schema_name LIKE 'tenant_%' AND schema_name ~ '^tenant_[a-zA-Z0-9_]+$' ORDER BY schema_name`).
		Scan(&physical).Error; err == nil {
		for _, s := range physical {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// processSchema возвращает (fixable, reportOnly) число проводок.
func processSchema(db *gorm.DB, schema string, apply bool) (int, int) {
	// company_id = числовой суффикс tenant_<id>. Нечисловой суффикс → пропускаем (нет связи с ledger.company_id).
	suffix := strings.TrimPrefix(schema, "tenant_")
	companyID, err := strconv.ParseUint(suffix, 10, 64)
	if err != nil {
		log.Printf("  ⏭️  %s: нечисловой суффикс, пропуск", schema)
		return 0, 0
	}
	contractsTbl := schema + ".contracts"

	// Не-RUB договоры схемы.
	var contracts []struct {
		ID       uint
		Currency string
	}
	if e := db.Table(contractsTbl).
		Select("id, currency").
		Where("currency NOT IN ('RUB','') AND currency IS NOT NULL AND deleted_at IS NULL").
		Scan(&contracts).Error; e != nil {
		log.Printf("  ⚠️ %s: чтение contracts: %v", schema, e)
		return 0, 0
	}
	if len(contracts) == 0 {
		return 0, 0
	}

	fixable, reportOnly := 0, 0
	for _, ct := range contracts {
		// Мис-стамповые per-договор проводки: payment/migration_balance с currency='RUB',
		// тогда как договор в ct.Currency. contract_id<>0 (per-договор однозначно).
		var ids []uint
		if e := db.Table("public.ledger_entries").
			Where("entry_type IN ('payment','migration_balance') AND contract_id = ? AND company_id = ? AND currency = 'RUB' AND deleted_at IS NULL",
				ct.ID, companyID).
			Pluck("id", &ids).Error; e != nil {
			log.Printf("  ⚠️ %s договор %d: чтение ledger: %v", schema, ct.ID, e)
			continue
		}
		if len(ids) == 0 {
			continue
		}
		log.Printf("  • %s договор %d (%s): %d проводок RUB→%s %v", schema, ct.ID, ct.Currency, len(ids), ct.Currency, ids)
		fixable += len(ids)
		if apply {
			if e := db.Transaction(func(tx *gorm.DB) error {
				// Snapshot затронутых строк (append в bak-таблицу) ПЕРЕД UPDATE — id-scoped откат.
				if e := tx.Exec(`CREATE TABLE IF NOT EXISTS public.ledger_entries_bak_ccy (LIKE public.ledger_entries INCLUDING ALL)`).Error; e != nil {
					return e
				}
				if e := tx.Exec(`INSERT INTO public.ledger_entries_bak_ccy SELECT * FROM public.ledger_entries WHERE id IN ? ON CONFLICT DO NOTHING`, ids).Error; e != nil {
					return e
				}
				return tx.Table("public.ledger_entries").Where("id IN ?", ids).
					Update("currency", ct.Currency).Error
			}); e != nil {
				log.Printf("  ❌ %s договор %d: UPDATE: %v", schema, ct.ID, e)
			}
		}
	}

	// Репорт-онли: contract_id=0 (платежи уровня контрагента) и мультивалютные cp — валюта
	// неоднозначна, авто-UPDATE опасен. Считаем не-RUB-? Здесь просто помечаем наличие cp-level
	// RUB-платежей у контрагентов, чьи договоры не-RUB — оператору разнести вручную.
	var cpLevel int64
	db.Table("public.ledger_entries").
		Where("contract_id = 0 AND company_id = ? AND entry_type IN ('payment','migration_balance') AND currency = 'RUB' AND deleted_at IS NULL", companyID).
		Count(&cpLevel)
	if cpLevel > 0 && len(contracts) > 0 {
		log.Printf("  ⚠️ %s: %d cp-level RUB-проводок (contract_id=0) при наличии не-RUB договоров — РЕПОРТ-ОНЛИ, разнести вручную", schema, cpLevel)
		reportOnly += int(cpLevel)
	}
	return fixable, reportOnly
}
