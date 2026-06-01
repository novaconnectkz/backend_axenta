package api

import (
	"testing"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// Ф2: баланс per-контрагент = SUM(amount) по ВСЕМ договорам контрагента.
func TestCounterpartyBalanceAggregates(t *testing.T) {
	if err := database.SetupTestDatabase(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer database.CleanupTestDatabase()
	if err := database.DB.AutoMigrate(&models.LedgerEntry{}, &models.Counterparty{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const admin, company, cp = uint(1), uint(1), uint(7)
	mk := func(contractID uint, cpID uint, typ string, amount float64) {
		database.DB.Create(&models.LedgerEntry{
			AdminAccountID: admin, CompanyID: company, ContractID: contractID, CounterpartyID: cpID,
			EntryType: typ, Amount: decimal.NewFromFloat(amount), Currency: "RUB",
			Source: "manual", EntryDate: time.Now().UTC(),
		})
	}
	// Контрагент cp: 2 договора. Договор 101: +12000 миграция, -48.39 charge. Договор 102: +1000.
	mk(101, cp, "migration_balance", 12000)
	mk(101, cp, "charge", -48.39)
	mk(102, cp, "payment", 1000)
	// Чужой контрагент (8) и чужой договор — НЕ должны попасть в баланс cp.
	mk(200, 8, "payment", 99999)
	// Запись без контрагента (cp=0) на договоре 103.
	mk(103, 0, "charge", -500)

	// Баланс контрагента = 12000 - 48.39 + 1000 = 12951.61 (оба договора, без чужих).
	bal := counterpartyBalance(cp, admin, company)
	assert.Equal(t, "12951.61", bal.StringFixed(2))

	// balanceForContract по договору 101 (cp<>0) → агрегат контрагента, не только 101.
	assert.Equal(t, "12951.61", balanceForContract(101, cp, admin, company).StringFixed(2))
	// По договору 102 того же контрагента — тот же агрегат.
	assert.Equal(t, "12951.61", balanceForContract(102, cp, admin, company).StringFixed(2))
	// Договор 103 (cp=0) → legacy per-договор: только его -500.
	assert.Equal(t, "-500.00", balanceForContract(103, 0, admin, company).StringFixed(2))

	// Разбивка по контрагенту: charged=48.39, paid=13000.
	charged, paid := ledgerBreakdown(101, cp, admin, company)
	assert.Equal(t, "48.39", charged.StringFixed(2))
	assert.Equal(t, "13000.00", paid.StringFixed(2))
}

// Валюто-fix: USD-charge и RUB-payment НЕ схлопываются в плоский SUM.
// До фикса counterpartyBalance давал 0 (−100 USD + 100 RUB), скрывая долг.
func TestLedgerBalancePerCurrencyNoCollapse(t *testing.T) {
	if err := database.SetupTestDatabase(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer database.CleanupTestDatabase()
	if err := database.DB.AutoMigrate(&models.LedgerEntry{}, &models.Counterparty{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const admin, company, cp = uint(1), uint(1), uint(9)
	mk := func(contractID uint, typ string, amount float64, ccy string) {
		database.DB.Create(&models.LedgerEntry{
			AdminAccountID: admin, CompanyID: company, ContractID: contractID, CounterpartyID: cp,
			EntryType: typ, Amount: decimal.NewFromFloat(amount), Currency: ccy,
			Source: "manual", EntryDate: time.Now().UTC(),
		})
	}
	mk(301, "charge", -100, "USD")  // USD-долг
	mk(302, "payment", 100, "RUB")  // RUB-платёж (другой договор/валюта)

	// Per-currency разбивка: ДВЕ строки, НЕ схлопнуты в 0.
	subs := ledgerBreakdownByCurrency(0, cp, admin, company)
	byCcy := map[string]decimal.Decimal{}
	for _, s := range subs {
		byCcy[s.Currency] = s.Balance
	}
	assert.Len(t, subs, 2, "USD и RUB должны быть отдельными суб-балансами")
	assert.Equal(t, "-100.00", byCcy["USD"].StringFixed(2), "USD-долг виден")
	assert.Equal(t, "100.00", byCcy["RUB"].StringFixed(2), "RUB-переплата отдельно")

	// balanceForContractCcy фильтрует по валюте: USD-договор видит только свой USD-долг.
	assert.Equal(t, "-100.00", balanceForContractCcy(301, cp, admin, company, "USD").StringFixed(2))
	assert.Equal(t, "100.00", balanceForContractCcy(302, cp, admin, company, "RUB").StringFixed(2))
}

// Ф4: resolveOrCreateCounterparty — find-or-create по идентичности договора (закрывает HIGH-3).
func TestResolveOrCreateCounterparty(t *testing.T) {
	if err := database.SetupTestDatabase(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer database.CleanupTestDatabase()
	if err := database.DB.AutoMigrate(&models.Counterparty{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const admin, company = uint(1), uint(1)

	// С ИНН → создаётся inn-контрагент, manual_review=false.
	c1 := &models.Contract{AdminAccountID: admin, CompanyID: company, ClientName: "ООО Альфа", ClientINN: "7701234567", ClientType: "organization"}
	id1, err := resolveOrCreateCounterparty(admin, company, c1)
	assert.NoError(t, err)
	assert.NotZero(t, id1)

	// Второй договор того же клиента (тот же ИНН) → ТОТ ЖЕ контрагент (find, не дубль).
	c2 := &models.Contract{AdminAccountID: admin, CompanyID: company, ClientName: "ООО Альфа (другое написание)", ClientINN: "7701234567"}
	id2, err := resolveOrCreateCounterparty(admin, company, c2)
	assert.NoError(t, err)
	assert.Equal(t, id1, id2, "тот же ИНН → тот же контрагент")

	var cp models.Counterparty
	database.DB.First(&cp, id1)
	assert.Equal(t, "inn", cp.IDType)
	assert.False(t, cp.ManualReview)

	// Без ИНН → manual_review=true; повтор по имени → тот же.
	c3 := &models.Contract{AdminAccountID: admin, CompanyID: company, ClientName: "ИП Гамма"}
	id3, err := resolveOrCreateCounterparty(admin, company, c3)
	assert.NoError(t, err)
	c4 := &models.Contract{AdminAccountID: admin, CompanyID: company, ClientName: "ИП Гамма"}
	id4, _ := resolveOrCreateCounterparty(admin, company, c4)
	assert.Equal(t, id3, id4, "то же имя без ИНН → тот же контрагент")
	var cp3 models.Counterparty
	database.DB.First(&cp3, id3)
	assert.True(t, cp3.ManualReview)

	// Всего 2 контрагента (Альфа + Гамма), не 4.
	var total int64
	database.DB.Model(&models.Counterparty{}).Count(&total)
	assert.Equal(t, int64(2), total)
}

// Ф2: company_id обязателен в скоупе — одинаковый counterparty_id в разных компаниях
// (admin_account_id общий) не должен смешиваться.
func TestCounterpartyBalanceScopedByCompany(t *testing.T) {
	if err := database.SetupTestDatabase(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer database.CleanupTestDatabase()
	if err := database.DB.AutoMigrate(&models.LedgerEntry{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const admin = uint(1)
	add := func(company uint, amount float64) {
		database.DB.Create(&models.LedgerEntry{
			AdminAccountID: admin, CompanyID: company, ContractID: 1, CounterpartyID: 5,
			EntryType: "payment", Amount: decimal.NewFromFloat(amount), Currency: "RUB",
			Source: "manual", EntryDate: time.Now().UTC(),
		})
	}
	add(1, 100) // company 1
	add(2, 777) // company 2 — тот же cp=5, тот же admin
	assert.Equal(t, "100.00", counterpartyBalance(5, admin, 1).StringFixed(2))
	assert.Equal(t, "777.00", counterpartyBalance(5, admin, 2).StringFixed(2))
}
