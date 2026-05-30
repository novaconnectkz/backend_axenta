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
