// Datafix Ф1 контрагент-ЛС: заводит сущности Counterparty из существующих договоров и
// проставляет contracts.counterparty_id + ledger_entries.counterparty_id (денорм).
//
// БЕЗОПАСНОСТЬ:
//   - по умолчанию DRY-RUN (только печатает план, ничего не пишет); запись — флаг --apply;
//   - идемпотентно: повторный прогон не дублит (lookup по идентичности + skip уже привязанных);
//   - per-schema транзакция с финальным assert (0 договоров и 0 проводок без counterparty_id),
//     при провале — rollback;
//   - ПЕРЕД --apply на проде обязателен pg_dump (см. предупреждение). Гонять сперва на
//     staging-копии прод-БД.
//
// Запуск:
//   go run ./cmd/backfill_counterparties               # dry-run по всем tenant-схемам
//   go run ./cmd/backfill_counterparties --schema tenant_186   # dry-run одной схемы
//   go run ./cmd/backfill_counterparties --apply        # запись (после бэкапа!)
package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"flag"
	"fmt"
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

	log.Println("🚀 Datafix Ф1: контрагенты из договоров (counterparties + counterparty_id)")
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

	// Схема должна существовать (создаётся boot-миграцией нового бинаря) — иначе fatal.
	verifySchema(db)

	schemas := targetSchemas(db, *onlySchema)
	if len(schemas) == 0 {
		log.Println("⚠️ Нет tenant-схем для обработки")
		return
	}
	log.Printf("📋 Схем к обработке: %d → %v", len(schemas), schemas)

	var totalContracts, totalCounterparties, totalManual, totalLedger int
	for _, schema := range schemas {
		cReq, cpNew, manual, ledger := processSchema(db, schema, *apply)
		totalContracts += cReq
		totalCounterparties += cpNew
		totalManual += manual
		totalLedger += ledger
	}

	log.Println("════════════════════════════════════════════")
	log.Printf("ИТОГО: договоров обработано %d → контрагентов %d (из них ручная проверка %d); проводок размечено %d",
		totalContracts, totalCounterparties, totalManual, totalLedger)
	if !*apply {
		log.Println("🔍 Это был DRY-RUN. Изменения НЕ внесены. Запусти с --apply после бэкапа.")
	} else {
		log.Println("✅ Готово. Изменения применены.")
	}
}

// verifySchema — fatal, если схема (таблица/колонки) ещё не создана boot-миграцией.
func verifySchema(db *gorm.DB) {
	checks := []struct {
		table, column, sql string
	}{
		{"counterparties", "", `SELECT EXISTS(SELECT FROM information_schema.tables WHERE table_schema='public' AND table_name='counterparties')`},
		{"ledger_entries", "counterparty_id", `SELECT EXISTS(SELECT FROM information_schema.columns WHERE table_schema='public' AND table_name='ledger_entries' AND column_name='counterparty_id')`},
	}
	for _, ch := range checks {
		var ok bool
		if err := db.Raw(ch.sql).Scan(&ok).Error; err != nil || !ok {
			log.Fatalf("❌ Не найдено %s%s в public — сначала задеплой новый бинарь (boot-миграция создаёт схему). err=%v", ch.table, ifColumn(ch.column), err)
		}
	}
	log.Println("✅ Схема public готова (counterparties + ledger_entries.counterparty_id)")
}

func ifColumn(c string) string {
	if c == "" {
		return ""
	}
	return "." + c
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

// identity — ключ группировки контрагента + производные поля.
type identity struct {
	key          string // уникальный ключ группы в рамках (admin, company)
	idType       string // inn|other (kz-расширение позже)
	taxID        string // нормализованный ИНН; "" → группировка по имени
	manualReview bool
}

// contractIdentity вычисляет идентичность по договору. Прод = РФ (country=ru):
// есть ИНН → id_type=inn, ключ по ИНН; нет ИНН → id_type=other, ключ по нормализованному имени
// + флаг ручной проверки.
func contractIdentity(adminID, companyID uint, clientName, clientINN string) identity {
	inn := strings.TrimSpace(clientINN)
	if inn != "" {
		return identity{
			key:    fmt.Sprintf("%d|%d|inn|%s", adminID, companyID, inn),
			idType: "inn",
			taxID:  inn,
		}
	}
	name := strings.ToLower(strings.Join(strings.Fields(clientName), " "))
	return identity{
		key:          fmt.Sprintf("%d|%d|name|%s", adminID, companyID, name),
		idType:       "other",
		taxID:        "",
		manualReview: true,
	}
}

// processSchema обрабатывает одну tenant-схему. Возвращает (договоров обработано,
// контрагентов создано, из них manual_review, проводок размечено).
func processSchema(db *gorm.DB, schema string, apply bool) (int, int, int, int) {
	contractsTbl := schema + ".contracts"

	// Только client-договоры: единый ЛС — клиентская концепция (charge per подписка→договор,
	// баланс/платёж/приостановка). Partner-договоры биллятся отдельной осью (partner-snapshots)
	// → counterparty_id остаётся 0 (вне модели контрагентов).
	var contracts []models.Contract
	if err := db.Table(contractsTbl).Where("contract_type = ? AND deleted_at IS NULL", "client").
		Order("id ASC").Find(&contracts).Error; err != nil {
		log.Printf("⚠️ [%s] не удалось прочитать договоры: %v", schema, err)
		return 0, 0, 0, 0
	}
	if len(contracts) == 0 {
		return 0, 0, 0, 0
	}

	run := func(tx *gorm.DB) (int, int, int, int, error) {
		keyToID := map[string]uint{}      // identity.key → counterparty_id (в т.ч. уже привязанные)
		conflicts := map[string]string{}  // лог расхождений лимита/режима у братьев
		created, manual := 0, 0

		for _, ct := range contracts {
			id := contractIdentity(ct.AdminAccountID, ct.CompanyID, ct.ClientName, ct.ClientINN)

			// Уже привязан (re-run) — зафиксировать связь key→id, не трогать.
			if ct.CounterpartyID != 0 {
				if _, ok := keyToID[id.key]; !ok {
					keyToID[id.key] = ct.CounterpartyID
				}
				continue
			}

			cpID, ok := keyToID[id.key]
			if !ok {
				// Поиск существующего (идемпотентность повторного прогона). Через tx (видит
				// незакоммиченное в рамках этой транзакции) + проверка .Error (денежный путь).
				var existingID uint
				var lerr error
				if id.taxID != "" {
					lerr = tx.Table("public.counterparties").Select("id").
						Where("admin_account_id = ? AND company_id = ? AND id_type = ? AND tax_id = ? AND deleted_at IS NULL",
							ct.AdminAccountID, ct.CompanyID, id.idType, id.taxID).Scan(&existingID).Error
				} else {
					// Exact-match нормализованного имени (case-preserved): обходит lc_collate=C
					// на проде, где LOWER() не понижает кириллицу. Совпадает с тем, что пишет
					// buildCounterparty (Name = normalizeName).
					lerr = tx.Table("public.counterparties").Select("id").
						Where(`admin_account_id = ? AND company_id = ? AND (tax_id = '' OR tax_id IS NULL) AND name = ? AND deleted_at IS NULL`,
							ct.AdminAccountID, ct.CompanyID, normalizeName(ct.ClientName)).Scan(&existingID).Error
				}
				if lerr != nil {
					return 0, 0, 0, 0, fmt.Errorf("lookup counterparty (contract %d): %w", ct.ID, lerr)
				}
				if existingID != 0 {
					cpID = existingID
				} else {
					cp := buildCounterparty(ct, id)
					if apply {
						if err := tx.Table("public.counterparties").Create(&cp).Error; err != nil {
							return 0, 0, 0, 0, fmt.Errorf("create counterparty (contract %d): %w", ct.ID, err)
						}
						cpID = cp.ID
					} else {
						cpID = 0 // dry-run: id неизвестен
					}
					created++
					if id.manualReview {
						manual++
					}
					log.Printf("   [%s] + контрагент %q (id_type=%s tax_id=%q manual=%v) ← договор #%d",
						schema, cp.Name, cp.IDType, cp.TaxID, cp.ManualReview, ct.ID)
				}
				keyToID[id.key] = cpID
			} else {
				// Брат той же группы — проверим расхождение лимита/режима (для отчёта Ф2).
				logSiblingConflict(conflicts, id.key, ct)
			}

			if apply && cpID != 0 {
				if err := tx.Table(contractsTbl).Where("id = ?", ct.ID).
					Update("counterparty_id", cpID).Error; err != nil {
					return 0, 0, 0, 0, fmt.Errorf("update contract %d: %w", ct.ID, err)
				}
			}
		}

		for k, msg := range conflicts {
			log.Printf("   ⚠️ [%s] расхождение реквизитов биллинга в группе %s: %s (решить в Ф2)", schema, k, msg)
		}

		// Backfill проводок: денорм counterparty_id из договоров. JOIN по
		// (contract_id, admin_account_id, company_id) — company_id ОБЯЗАТЕЛЕН: admin_account_id
		// в legacy-sync общий для всех компаний, а contract_id коллизятся между tenant-схемами;
		// без company_id разметишь проводки чужого тенанта (Codex critical #1).
		ledgerMarked := 0
		if apply {
			res := tx.Exec(fmt.Sprintf(`
				UPDATE public.ledger_entries le
				SET counterparty_id = c.counterparty_id
				FROM %s c
				WHERE le.contract_id = c.id
				  AND le.admin_account_id = c.admin_account_id
				  AND le.company_id = c.company_id
				  AND c.contract_type = 'client'
				  AND le.counterparty_id = 0
				  AND c.counterparty_id <> 0
				  AND le.deleted_at IS NULL`, contractsTbl))
			if res.Error != nil {
				return 0, 0, 0, 0, fmt.Errorf("backfill ledger: %w", res.Error)
			}
			ledgerMarked = int(res.RowsAffected)

			// ASSERT: не осталось договоров и проводок без counterparty_id.
			var orphanContracts int64
			if e := tx.Table(contractsTbl).Where("contract_type = 'client' AND counterparty_id = 0 AND deleted_at IS NULL").Count(&orphanContracts).Error; e != nil {
				return 0, 0, 0, 0, fmt.Errorf("assert contracts query: %w", e)
			}
			if orphanContracts > 0 {
				return 0, 0, 0, 0, fmt.Errorf("assert fail: %d client-договоров без counterparty_id", orphanContracts)
			}
			var orphanLedger int64
			if e := tx.Raw(fmt.Sprintf(`
				SELECT COUNT(*) FROM public.ledger_entries le
				JOIN %s c ON le.contract_id = c.id AND le.admin_account_id = c.admin_account_id AND le.company_id = c.company_id
				WHERE c.contract_type = 'client' AND le.counterparty_id = 0 AND le.deleted_at IS NULL`, contractsTbl)).Scan(&orphanLedger).Error; e != nil {
				return 0, 0, 0, 0, fmt.Errorf("assert ledger query: %w", e)
			}
			if orphanLedger > 0 {
				return 0, 0, 0, 0, fmt.Errorf("assert fail: %d проводок без counterparty_id", orphanLedger)
			}
		} else {
			// dry-run: сколько проводок было бы размечено (через tx==db).
			var cnt int64
			if e := tx.Raw(fmt.Sprintf(`
				SELECT COUNT(*) FROM public.ledger_entries le
				JOIN %s c ON le.contract_id = c.id AND le.admin_account_id = c.admin_account_id AND le.company_id = c.company_id
				WHERE c.contract_type = 'client' AND le.counterparty_id = 0 AND le.deleted_at IS NULL`, contractsTbl)).Scan(&cnt).Error; e != nil {
				return 0, 0, 0, 0, fmt.Errorf("dry-run ledger count: %w", e)
			}
			ledgerMarked = int(cnt)
		}

		return len(contracts), created, manual, ledgerMarked, nil
	}

	var rc, rcp, rm, rl int
	if apply {
		err := db.Transaction(func(tx *gorm.DB) error {
			var e error
			rc, rcp, rm, rl, e = run(tx)
			return e
		})
		if err != nil {
			log.Fatalf("❌ [%s] откат транзакции: %v", schema, err)
		}
	} else {
		rc, rcp, rm, rl, _ = run(db)
	}
	log.Printf("📊 [%s] договоров %d → контрагентов %d (ручная проверка %d); проводок %d",
		schema, rc, rcp, rm, rl)
	return rc, rcp, rm, rl
}

// buildCounterparty — новый контрагент по репрезентативному договору (min id группы).
func buildCounterparty(ct models.Contract, id identity) models.Counterparty {
	return models.Counterparty{
		AdminAccountID:           ct.AdminAccountID,
		CompanyID:                ct.CompanyID,
		Country:                  "ru",
		IDType:                   id.idType,
		TaxID:                    id.taxID,
		Name:                     normalizeName(ct.ClientName),
		ClientType:               ct.ClientType,
		ShortName:                ct.ClientShortName,
		KPP:                      ct.ClientKPP,
		Email:                    ct.ClientEmail,
		Phone:                    ct.ClientPhone,
		Address:                  ct.ClientAddress,
		LegalAddress:             ct.ClientLegalAddress,
		PostalAddress:            ct.ClientPostalAddress,
		OGRN:                     ct.ClientOGRN,
		OKPO:                     ct.ClientOKPO,
		Director:                 ct.ClientDirector,
		BasedOn:                  ct.ClientBasedOn,
		Website:                  ct.ClientWebsite,
		BankName:                 ct.ClientBankName,
		BankBIK:                  ct.ClientBankBIK,
		BankCorrespondentAccount: ct.ClientBankCorrespondentAccount,
		BankAccount:              ct.ClientBankAccount,
		BankRecipient:            ct.ClientBankRecipient,
		PassportSeries:           ct.ClientPassportSeries,
		PassportNumber:           ct.ClientPassportNumber,
		PassportIssuedBy:         ct.ClientPassportIssuedBy,
		PassportIssueDate:        ct.ClientPassportIssueDate,
		PassportDepartmentCode:   ct.ClientPassportDepartmentCode,
		RegistrationAddress:      ct.ClientRegistrationAddress,
		ActualAddress:            ct.ClientActualAddress,
		SNILS:                    ct.ClientSNILS,
		OGRNIP:                   ct.ClientOGRNIP,
		BillingMode:              defaultStr(ct.BillingMode, "prepaid"),
		CreditLimit:              ct.CreditLimit,
		ManualReview:             id.manualReview,
		CreatedBy:                "datafix_backfill",
	}
}

func defaultStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// normalizeName — каноничное имя контрагента: схлопывает все пробельные прогоны в один
// пробел + trim, регистр сохраняет. Пишется в Counterparty.Name И используется для
// exact-match идемпотентного lookup'а (обходит lc_collate=C, где LOWER() не трогает кириллицу).
func normalizeName(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// logSiblingConflict фиксирует расхождение credit_limit/billing_mode у договоров одной группы.
func logSiblingConflict(conflicts map[string]string, key string, ct models.Contract) {
	if !ct.CreditLimit.Equal(decimal.Zero) || (ct.BillingMode != "" && ct.BillingMode != "prepaid") {
		conflicts[key] = fmt.Sprintf("договор #%d: limit=%s mode=%s", ct.ID, ct.CreditLimit.String(), ct.BillingMode)
	}
}
