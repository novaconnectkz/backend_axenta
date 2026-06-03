// Одноразовый per-contract backfill партнёрских снапшотов за период (заполнение дыры).
// Создаёт недостающие дни через штатный генератор (защита "already exists" — существующие
// не трогает). Object count — текущий из БД (db-источник не хранит подневную историю).
//
// DRY-RUN по умолчанию (печатает план + текущий count), запись — --apply.
// Запуск НА хосте (читает .env): go build → scp → ./bin --contract N --from .. --to .. --schema tenant_186 --apply
package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"flag"
	"log"
	"time"
)

func main() {
	contractID := flag.Uint("contract", 0, "contract_id")
	schema := flag.String("schema", "", "tenant-схема (например tenant_186)")
	fromStr := flag.String("from", "", "YYYY-MM-DD")
	toStr := flag.String("to", "", "YYYY-MM-DD")
	apply := flag.Bool("apply", false, "выполнить запись (по умолчанию dry-run)")
	flag.Parse()

	if *contractID == 0 || *schema == "" || *fromStr == "" || *toStr == "" {
		log.Fatalf("нужны --contract --schema --from --to")
	}
	from, err := time.Parse("2006-01-02", *fromStr)
	if err != nil {
		log.Fatalf("--from: %v", err)
	}
	to, err := time.Parse("2006-01-02", *toStr)
	if err != nil {
		log.Fatalf("--to: %v", err)
	}
	if from.After(to) {
		log.Fatalf("--from позже --to")
	}

	if _, err := config.LoadConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("db: %v", err)
	}
	tenantDB, err := database.ConnectToTenant(*schema)
	if err != nil {
		log.Fatalf("tenant %s: %v", *schema, err)
	}

	var contract models.Contract
	if err := tenantDB.First(&contract, *contractID).Error; err != nil {
		log.Fatalf("договор %d в %s не найден: %v", *contractID, *schema, err)
	}
	if contract.ContractType != "partner" || contract.PartnerCompanyID == nil {
		log.Fatalf("договор %d не партнёрский / нет partner_company_id", *contractID)
	}
	log.Printf("📋 Договор %d (%s) partner_company_id=%d source=%s, период %s..%s, apply=%v",
		contract.ID, contract.Number, *contract.PartnerCompanyID, contract.PartnerSource,
		from.Format("2006-01-02"), to.Format("2006-01-02"), *apply)

	svc := services.NewPartnerSnapshotService()
	// Текущий count (db) — для информации в dry-run.
	tot, act, _ := svc.CountPartnerObjectsFromDB(*contract.PartnerCompanyID, to, tenantDB)
	log.Printf("ℹ️ Текущий count из БД: всего=%d активных=%d (применится ко всем backfill-дням)", tot, act)

	created, skipped, failed := 0, 0, 0
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if !*apply {
			log.Printf("  [dry] %s — создал бы снапшот", d.Format("2006-01-02"))
			continue
		}
		if err := svc.CreateSnapshotForContractWithTokenAndDBAndSource(&contract, d, "", tenantDB, "db"); err != nil {
			if err.Error() == "snapshot already exists" {
				skipped++
				continue
			}
			failed++
			log.Printf("  ❌ %s: %v", d.Format("2006-01-02"), err)
			continue
		}
		created++
	}
	log.Printf("✅ Итог: создано=%d, пропущено(existing)=%d, ошибок=%d", created, skipped, failed)
}
