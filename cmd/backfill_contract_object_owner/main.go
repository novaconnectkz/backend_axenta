// Datafix B0 (enforcement): наполняет contract_objects.source/owner_external_id для Axenta —
// owner-учётка объекта (account_external_id) из axenta_object_snapshots ТОЙ ЖЕ tenant-схемы.
// Это КЭШ для группировки/blast-radius; enforcement резолвит владельца на лету (MoveAccount).
//
// БЕЗОПАСНОСТЬ: read-only по сути (заполняет денорм-кэш), DRY-RUN по умолчанию, --apply для записи.
// Идемпотентно: трогает только строки с пустым owner_external_id. Снапшоты тенант-специфичны
// (IsGlobal:false) → читаем из той же схемы, что и contract_objects, НЕ из public.
// wialon/skif/gelios НЕ наполняем (нет связи client-договор→юнит) — остаются source='axenta'-дефолтом.
//
// Запуск:
//   go run ./cmd/backfill_contract_object_owner            # dry-run все схемы
//   go run ./cmd/backfill_contract_object_owner --schema tenant_186
//   go run ./cmd/backfill_contract_object_owner --apply
package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"flag"
	"fmt"
	"log"
	"regexp"

	"gorm.io/gorm"
)

var tenantSchemaRegex = regexp.MustCompile(`^tenant_[a-zA-Z0-9_]+$`)

func main() {
	apply := flag.Bool("apply", false, "выполнить запись (по умолчанию dry-run)")
	onlySchema := flag.String("schema", "", "ограничить одной tenant-схемой")
	flag.Parse()

	log.Println("🚀 Datafix B0: contract_objects.owner_external_id ← axenta_object_snapshots (per tenant)")
	if *apply {
		log.Println("⚠️  РЕЖИМ ЗАПИСИ (--apply). Денорм-кэш; pg_dump желателен.")
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

	total := 0
	for _, schema := range schemas {
		total += processSchema(db, schema, *apply)
	}
	verb := "к заполнению"
	if *apply {
		verb = "заполнено"
	}
	log.Printf("✅ Итог: %s contract_objects (axenta owner)=%d", verb, total)
}

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
			if !co.IsActive || co.DatabaseSchema == "" || seen[co.DatabaseSchema] || !tenantSchemaRegex.MatchString(co.DatabaseSchema) {
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

// processSchema возвращает число contract_objects, заполненных owner_external_id (или к заполнению).
func processSchema(db *gorm.DB, schema string, apply bool) int {
	coTbl := schema + ".contract_objects"
	aosTbl := schema + ".axenta_object_snapshots"

	// Снапшот-таблица может отсутствовать в схеме (нет интеграции Axenta) → пропуск.
	var hasAOS bool
	db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = ? AND table_name = 'axenta_object_snapshots')`, schema).Scan(&hasAOS)
	if !hasAOS {
		return 0
	}

	// Кандидаты: source='axenta' (дефолт) с пустым owner_external_id, чей объект есть в снапшоте.
	var n int64
	countSQL := fmt.Sprintf(`SELECT count(*) FROM %s co JOIN %s aos
		ON aos.external_object_id = co.object_id AND aos.axenta_deleted_at IS NULL
		WHERE co.deleted_at IS NULL AND (co.owner_external_id IS NULL OR co.owner_external_id = '')
		  AND (co.source IS NULL OR co.source = 'axenta')`, coTbl, aosTbl)
	db.Raw(countSQL).Scan(&n)
	if n == 0 {
		return 0
	}
	log.Printf("  • %s: %d contract_objects к заполнению owner", schema, n)

	if apply {
		updSQL := fmt.Sprintf(`UPDATE %s co
			SET source = 'axenta', owner_external_id = aos.account_external_id::text
			FROM %s aos
			WHERE aos.external_object_id = co.object_id AND aos.axenta_deleted_at IS NULL
			  AND co.deleted_at IS NULL AND (co.owner_external_id IS NULL OR co.owner_external_id = '')`, coTbl, aosTbl)
		if e := db.Exec(updSQL).Error; e != nil {
			log.Printf("  ❌ %s UPDATE: %v", schema, e)
			return 0
		}
	}
	return int(n)
}
