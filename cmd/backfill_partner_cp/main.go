// Phase D datafix: заводит контрагентов kind='partner' (СПРАВОЧНИК идентичности) из
// существующих партнёрских договоров и проставляет contracts.counterparty_id. Партнёр
// НЕ субъект лицевого счёта (биллинг snapshot, не ledger) — поэтому ledger_entries НЕ
// размечаются (в отличие от backfill_counterparties), баланс к партнёру не применяется.
//
// БЕЗОПАСНОСТЬ:
//   - DRY-RUN по умолчанию (печатает план); запись — флаг --apply;
//   - идемпотентно: пропускает партнёрские договоры с уже проставленным counterparty_id;
//     lookup существующего partner-cp по (admin, company, kind='partner', id_type, tax_id);
//   - per-schema транзакция; cp в public.counterparties, договор в <schema>.contracts
//     (полные имена таблиц, без переключения search_path);
//   - ПЕРЕД --apply на проде — pg_dump.
//
// Запуск:
//
//	go run ./cmd/backfill_partner_cp                    # dry-run все схемы
//	go run ./cmd/backfill_partner_cp --schema tenant_186
//	go run ./cmd/backfill_partner_cp --apply            # запись (после бэкапа!)
package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"flag"
	"log"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var tenantSchemaRegex = regexp.MustCompile(`^tenant_[a-zA-Z0-9_]+$`)

func main() {
	apply := flag.Bool("apply", false, "выполнить запись (по умолчанию dry-run)")
	onlySchema := flag.String("schema", "", "ограничить одной tenant-схемой (например tenant_186)")
	flag.Parse()

	log.Println("🚀 Phase D datafix: контрагенты kind='partner' из партнёрских договоров")
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

	var totalPart, totalCreated, totalLinked int
	for _, schema := range schemas {
		p, cr, l := processSchema(db, schema, *apply)
		totalPart += p
		totalCreated += cr
		totalLinked += l
	}

	log.Println("════════════════════════════════════════════")
	log.Printf("ИТОГО: партнёрских договоров без cp %d → создано контрагентов %d, привязано договоров %d",
		totalPart, totalCreated, totalLinked)
	if !*apply {
		log.Println("🔍 Это был DRY-RUN. Изменения НЕ внесены. Запусти с --apply после бэкапа.")
	} else {
		log.Println("✅ Готово. Изменения применены.")
	}
}

// processSchema — одна tenant-схема. Возвращает (кандидатов, создано-cp, привязано-договоров).
func processSchema(db *gorm.DB, schema string, apply bool) (int, int, int) {
	contractsTbl := schema + ".contracts"

	var contracts []models.Contract
	if err := db.Table(contractsTbl).
		Where("contract_type = ? AND deleted_at IS NULL AND COALESCE(counterparty_id,0) = 0", "partner").
		Order("id ASC").Find(&contracts).Error; err != nil {
		log.Printf("⚠️ [%s] не удалось прочитать договоры: %v", schema, err)
		return 0, 0, 0
	}
	if len(contracts) == 0 {
		log.Printf("✅ [%s] партнёрских договоров без cp нет", schema)
		return 0, 0, 0
	}

	created, linked := 0, 0
	run := func(tx *gorm.DB) error {
		for _, ct := range contracts {
			name := normalizeName(firstNonEmpty(ct.PartnerName, ct.ClientName))
			inn := strings.TrimSpace(firstNonEmpty(ct.PartnerINN, ct.ClientINN))
			if name == "" {
				log.Printf("   ⚠️ [%s] договор #%d: пустое имя партнёра — пропуск", schema, ct.ID)
				continue
			}
			idType, manual := "other", true
			if inn != "" {
				idType, manual = "inn", false
			}

			// Lookup существующего partner-cp (идемпотентность). kind='partner' в ключе —
			// не матчит клиентского cp с тем же ИНН.
			var existingID uint
			q := tx.Table("public.counterparties").Select("id").
				Where("admin_account_id = ? AND company_id = ? AND kind = ? AND deleted_at IS NULL",
					ct.AdminAccountID, ct.CompanyID, "partner")
			if inn != "" {
				q = q.Where("id_type = ? AND tax_id = ?", idType, inn)
			} else {
				q = q.Where("(tax_id = '' OR tax_id IS NULL) AND name = ?", name)
			}
			if e := q.Scan(&existingID).Error; e != nil {
				return e
			}

			cpID := existingID
			if cpID == 0 {
				cp := models.Counterparty{
					AdminAccountID: ct.AdminAccountID, CompanyID: ct.CompanyID, Country: "ru", Kind: "partner",
					IDType: idType, TaxID: inn, Name: name, ClientType: ct.ClientType,
					ShortName: ct.ClientShortName, KPP: ct.ClientKPP, Email: ct.ClientEmail, Phone: ct.ClientPhone,
					Address: ct.ClientAddress, LegalAddress: ct.ClientLegalAddress, PostalAddress: ct.ClientPostalAddress,
					OGRN: ct.ClientOGRN, OKPO: ct.ClientOKPO, Director: ct.ClientDirector, BasedOn: ct.ClientBasedOn,
					Website: ct.ClientWebsite, BankName: ct.ClientBankName, BankBIK: ct.ClientBankBIK,
					BankCorrespondentAccount: ct.ClientBankCorrespondentAccount, BankAccount: ct.ClientBankAccount,
					BankRecipient: ct.ClientBankRecipient,
					BillingMode:   "prepaid", CreditLimit: decimal.Zero,
					ManualReview: manual, CreatedBy: "datafix:partner-cp",
				}
				if apply {
					if e := tx.Table("public.counterparties").Create(&cp).Error; e != nil {
						return e
					}
					cpID = cp.ID
				}
				created++
				log.Printf("   [%s] + партнёр-контрагент %q (id_type=%s tax_id=%q) ← договор #%d",
					schema, name, idType, inn, ct.ID)
			}

			if apply && cpID != 0 {
				if e := tx.Table(contractsTbl).Where("id = ? AND contract_type = ?", ct.ID, "partner").
					Update("counterparty_id", cpID).Error; e != nil {
					return e
				}
				linked++
			}
		}
		return nil
	}

	if apply {
		if err := db.Transaction(run); err != nil {
			log.Printf("❌ [%s] транзакция откатана: %v", schema, err)
			return len(contracts), 0, 0
		}
	} else {
		_ = run(db) // dry-run: без транзакции, без записи
	}
	log.Printf("📊 [%s] кандидатов %d, создано cp %d, привязано %d", schema, len(contracts), created, linked)
	return len(contracts), created, linked
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func normalizeName(s string) string { return strings.Join(strings.Fields(s), " ") }

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
