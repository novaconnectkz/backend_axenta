// Phase C (C-партнёр) datafix: заполняет дом идентичности партнёра — partner_name/
// partner_inn/partner_requisites — из денорм client_* существующих партнёрских
// договоров. Нужен для строк, созданных ДО dual-write (runtime заполняет partner_*
// на Create/Update; этот backfill закрывает исторические 26 строк).
//
// БЕЗОПАСНОСТЬ:
//   - по умолчанию DRY-RUN (печатает план, не пишет); запись — флаг --apply;
//   - идемпотентно: обрабатывает только партнёрские договоры где partner_name пуст
//     (повторный прогон не трогает уже заполненные);
//   - переиспользует models.Contract.ApplyPartnerIdentityFromClient() — та же логика
//     что runtime, без расхождения SQL/Go;
//   - per-schema, search_path как в RunMigrations.
//
// Запуск:
//
//	go run ./cmd/backfill_partner_identity                    # dry-run все схемы
//	go run ./cmd/backfill_partner_identity --schema tenant_186
//	go run ./cmd/backfill_partner_identity --apply            # запись (после бэкапа!)
package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"flag"
	"log"
	"regexp"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

var tenantSchemaRegex = regexp.MustCompile(`^tenant_[a-zA-Z0-9_]+$`)

func main() {
	apply := flag.Bool("apply", false, "выполнить запись (по умолчанию dry-run)")
	onlySchema := flag.String("schema", "", "ограничить одной tenant-схемой (например tenant_186)")
	flag.Parse()

	log.Println("🚀 Phase C datafix: дом идентичности партнёра (partner_* ← client_*)")
	if *apply {
		log.Println("⚠️  РЕЖИМ ЗАПИСИ (--apply). Убедись, что сделан pg_dump бэкап БД!")
	} else {
		log.Println("🔍 DRY-RUN: только план, без изменений. Для записи добавь --apply")
	}

	if _, err := config.LoadConfig(); err != nil {
		log.Fatalf("❌ Не удалось загрузить конфигурацию: %v", err)
	}
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Не удалось подключиться к базе данных: %v", err)
	}
	db := database.GetDB()
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Fatalf("❌ Не удалось переключиться на public: %v", err)
	}

	schemas := targetSchemas(db, *onlySchema)
	if len(schemas) == 0 {
		log.Println("⚠️ Нет tenant-схем для обработки")
		return
	}
	log.Printf("📋 Схем к обработке: %d → %v", len(schemas), schemas)

	var total, updated int
	for _, schema := range schemas {
		t, u := processSchema(db, schema, *apply)
		total += t
		updated += u
	}

	log.Println("════════════════════════════════════════════")
	log.Printf("ИТОГО: партнёрских договоров без partner_name %d → заполнено %d", total, updated)
	if !*apply {
		log.Println("🔍 Это был DRY-RUN. Изменения НЕ внесены. Запусти с --apply после бэкапа.")
	} else {
		log.Println("✅ Готово. Изменения применены.")
	}
}

// processSchema — обрабатывает одну tenant-схему. Возвращает (кандидатов, заполнено).
func processSchema(db *gorm.DB, schema string, apply bool) (int, int) {
	if err := db.Exec("SET search_path TO " + pq.QuoteIdentifier(schema)).Error; err != nil {
		log.Printf("⚠️ %s: не удалось переключить search_path: %v", schema, err)
		return 0, 0
	}
	defer db.Exec("SET search_path TO public")

	// Кандидаты: партнёрские договоры, где дом идентичности ещё пуст.
	var contracts []models.Contract
	if err := db.Where("contract_type = ? AND coalesce(partner_name,'') = ''", "partner").
		Find(&contracts).Error; err != nil {
		log.Printf("⚠️ %s: не удалось выбрать договоры: %v", schema, err)
		return 0, 0
	}
	if len(contracts) == 0 {
		log.Printf("✅ %s: партнёрских договоров без partner_name нет", schema)
		return 0, 0
	}

	updated := 0
	for i := range contracts {
		ct := &contracts[i]
		ct.ApplyPartnerIdentityFromClient() // partner_* ← client_* (та же логика что runtime)
		log.Printf("   %s #%d: partner_name=%q partner_inn=%q req=%s",
			schema, ct.ID, ct.PartnerName, ct.PartnerINN, ct.PartnerRequisites)
		if !apply {
			continue
		}
		if err := db.Model(&models.Contract{}).Where("id = ?", ct.ID).
			Updates(ct.PartnerIdentityMap()).Error; err != nil {
			log.Printf("   ❌ %s #%d: ошибка записи: %v", schema, ct.ID, err)
			continue
		}
		updated++
	}
	log.Printf("📊 %s: кандидатов %d, заполнено %d", schema, len(contracts), updated)
	return len(contracts), updated
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
