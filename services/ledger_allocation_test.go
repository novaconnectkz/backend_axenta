package services

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func adec(v string) decimal.Decimal { x, _ := decimal.NewFromString(v); return x }

func pm(s string) time.Time { t, _ := time.Parse("2006-01", s); return t }

func cov(t *testing.T, r AllocResult, cid uint, want string) {
	t.Helper()
	got := r.Coverage[cid]
	if !got.Equal(adec(want)) {
		t.Errorf("coverage[%d] = %s, want %s", cid, got.StringFixed(2), want)
	}
}

// Earmark-изоляция: оплаченный B не страдает за долг A (главный failure-mode).
func TestAllocate_EarmarkIsolation(t *testing.T) {
	e := []AllocEntry{
		{ID: 1, ContractID: 10, EntryType: "charge", Amount: adec("-5000"), PeriodStart: pm("2026-01"), PeriodOK: true},
		{ID: 2, ContractID: 11, EntryType: "charge", Amount: adec("-3000"), PeriodStart: pm("2026-01"), PeriodOK: true},
		{ID: 3, ContractID: 11, EntryType: "payment", Amount: adec("3000")},
	}
	r := AllocateCoverage(e, map[uint]bool{10: true, 11: true}, AllocStrictEarmark)
	cov(t, r, 11, "0.00")     // earmark 3000 гасит свой долг 3000
	cov(t, r, 10, "-5000.00") // A не покрыт чужими деньгами
	if r.Quarantined {
		t.Fatal("не должно быть quarantine")
	}
}

// FIFO: untagged-пул гасит СТАРЕЙШИЙ долг первым.
func TestAllocate_FIFOOldestFirst(t *testing.T) {
	e := []AllocEntry{
		{ID: 1, ContractID: 10, EntryType: "charge", Amount: adec("-5000"), PeriodStart: pm("2026-01"), PeriodOK: true},
		{ID: 2, ContractID: 11, EntryType: "charge", Amount: adec("-3000"), PeriodStart: pm("2026-02"), PeriodOK: true},
		{ID: 3, ContractID: 0, EntryType: "payment", Amount: adec("4000")}, // untagged
	}
	r := AllocateCoverage(e, map[uint]bool{10: true, 11: true}, AllocStrictEarmark)
	cov(t, r, 10, "-1000.00") // старейший (2026-01) погашен на 4000 → -1000
	cov(t, r, 11, "-3000.00") // младший не тронут
	if !r.WalletFreePool.IsZero() {
		t.Errorf("pool остаток = %s, want 0", r.WalletFreePool)
	}
}

// Частичное покрытие: пул больше долга → остаток в wallet_free_pool.
func TestAllocate_PoolSurplusToFreePool(t *testing.T) {
	e := []AllocEntry{
		{ID: 1, ContractID: 10, EntryType: "charge", Amount: adec("-1000"), PeriodStart: pm("2026-01"), PeriodOK: true},
		{ID: 2, ContractID: 0, EntryType: "payment", Amount: adec("1500")},
	}
	r := AllocateCoverage(e, map[uint]bool{10: true}, AllocStrictEarmark)
	cov(t, r, 10, "0.00")
	if !r.WalletFreePool.Equal(adec("500")) {
		t.Errorf("pool = %s, want 500", r.WalletFreePool)
	}
}

// Quarantine: непарсируемый период у charge → состояние не менять.
func TestAllocate_QuarantineUnresolvedPeriod(t *testing.T) {
	e := []AllocEntry{
		{ID: 1, ContractID: 10, EntryType: "charge", Amount: adec("-1000"), PeriodOK: false},
	}
	r := AllocateCoverage(e, map[uint]bool{10: true}, AllocStrictEarmark)
	if !r.Quarantined {
		t.Fatal("ожидался quarantine при нерезолвленном периоде debt")
	}
	if len(r.Coverage) != 0 {
		t.Errorf("при quarantine Coverage должен быть пуст, got %v", r.Coverage)
	}
}

// Детерминизм: результат НЕ зависит от порядка проводок (эквивалент ORDER BY RANDOM).
func TestAllocate_DeterministicUnderShuffle(t *testing.T) {
	base := []AllocEntry{
		{ID: 1, ContractID: 10, EntryType: "charge", Amount: adec("-5000"), PeriodStart: pm("2026-01"), PeriodOK: true},
		{ID: 2, ContractID: 11, EntryType: "charge", Amount: adec("-3000"), PeriodStart: pm("2026-02"), PeriodOK: true},
		{ID: 3, ContractID: 0, EntryType: "payment", Amount: adec("4000")},
		{ID: 4, ContractID: 11, EntryType: "payment", Amount: adec("1000")},
	}
	rev := make([]AllocEntry, len(base))
	for i := range base {
		rev[i] = base[len(base)-1-i]
	}
	open := map[uint]bool{10: true, 11: true}
	r1 := AllocateCoverage(base, open, AllocStrictEarmark)
	r2 := AllocateCoverage(rev, open, AllocStrictEarmark)
	for _, cid := range []uint{10, 11} {
		if !r1.Coverage[cid].Equal(r2.Coverage[cid]) {
			t.Errorf("недетерминизм cid %d: %s vs %s", cid, r1.Coverage[cid], r2.Coverage[cid])
		}
	}
	if !r1.WalletFreePool.Equal(r2.WalletFreePool) {
		t.Errorf("недетерминизм pool: %s vs %s", r1.WalletFreePool, r2.WalletFreePool)
	}
	// Проверка ожидаемых значений: B earmark 1000 → долг 3000→-2000; пул 4000 на A(-5000)→-1000.
	cov(t, r1, 10, "-1000.00")
	cov(t, r1, 11, "-2000.00")
}

// strict vs wallet_release: переплата earmark по B либо заперта, либо идёт в FIFO на A.
func TestAllocate_SurplusPolicy(t *testing.T) {
	mk := func() []AllocEntry {
		return []AllocEntry{
			{ID: 1, ContractID: 11, EntryType: "charge", Amount: adec("-1000"), PeriodStart: pm("2026-02"), PeriodOK: true},
			{ID: 2, ContractID: 11, EntryType: "payment", Amount: adec("1500")}, // переплата 500 по B
			{ID: 3, ContractID: 10, EntryType: "charge", Amount: adec("-800"), PeriodStart: pm("2026-01"), PeriodOK: true},
		}
	}
	open := map[uint]bool{10: true, 11: true}

	strict := AllocateCoverage(mk(), open, AllocStrictEarmark)
	cov(t, strict, 11, "0.00")     // переплата 500 заперта за B
	cov(t, strict, 10, "-800.00")  // A не получил чужую переплату
	if !strict.WalletFreePool.IsZero() {
		t.Errorf("strict pool = %s, want 0", strict.WalletFreePool)
	}

	rel := AllocateCoverage(mk(), open, AllocWalletRelease)
	cov(t, rel, 11, "0.00")
	cov(t, rel, 10, "-300.00") // переплата 500 → пул → FIFO на A: -800+500
	if !rel.WalletFreePool.IsZero() {
		t.Errorf("release pool = %s, want 0", rel.WalletFreePool)
	}
}

// transfer_out уменьшает earmark/пул, НЕ создаёт фантомный долг.
func TestAllocate_TransferOutNotObligation(t *testing.T) {
	e := []AllocEntry{
		{ID: 1, ContractID: 11, EntryType: "charge", Amount: adec("-500"), PeriodStart: pm("2026-01"), PeriodOK: true},
		{ID: 2, ContractID: 11, EntryType: "payment", Amount: adec("1000")},
		{ID: 3, ContractID: 11, EntryType: "transfer_out", Amount: adec("-400")},
	}
	r := AllocateCoverage(e, map[uint]bool{11: true}, AllocStrictEarmark)
	// earmark = 1000-400=600; obligation=500 → cov=+100 → strict capped 0. Не долг, не quarantine.
	cov(t, r, 11, "0.00")
	if r.Quarantined {
		t.Fatal("transfer_out не должен вызывать quarantine")
	}
}

// Закрытый договор (не в openContracts) исключён из FIFO-погашения.
func TestAllocate_ClosedExcludedFromFIFO(t *testing.T) {
	e := []AllocEntry{
		{ID: 1, ContractID: 10, EntryType: "charge", Amount: adec("-1000"), PeriodStart: pm("2026-01"), PeriodOK: true}, // закрыт
		{ID: 2, ContractID: 11, EntryType: "charge", Amount: adec("-1000"), PeriodStart: pm("2026-02"), PeriodOK: true}, // открыт
		{ID: 3, ContractID: 0, EntryType: "payment", Amount: adec("1000")},
	}
	r := AllocateCoverage(e, map[uint]bool{11: true}, AllocStrictEarmark) // только 11 открыт
	cov(t, r, 11, "0.00")      // пул ушёл живому, не закрытому 10
	cov(t, r, 10, "-1000.00")  // закрытый остался в долге (списывается отдельно)
}

// Codex 5.1: adjustment<0 без периода — обязательство → quarantine (не проскакивает).
func TestAllocate_AdjustmentNegativeNoPeriodQuarantines(t *testing.T) {
	e := []AllocEntry{
		{ID: 1, ContractID: 10, EntryType: "adjustment", Amount: adec("-1000"), PeriodOK: false},
	}
	r := AllocateCoverage(e, map[uint]bool{10: true}, AllocStrictEarmark)
	if !r.Quarantined {
		t.Fatal("adjustment<0 без периода должен квантоваться")
	}
	if len(r.Coverage) != 0 {
		t.Errorf("при quarantine Coverage пуст, got %v", r.Coverage)
	}
}

// Codex 7.1: долг без датированного начисления (reversal обнулил earmark в минус) не должен
// вытеснять датированный долг из головы FIFO — он идёт в ХВОСТ.
func TestAllocate_UndatedDebtGoesToFIFOTail(t *testing.T) {
	e := []AllocEntry{
		// Договор 10: payment +1000, затем reversal −2000 → earmark −1000 (долг БЕЗ периода).
		{ID: 1, ContractID: 10, EntryType: "payment", Amount: adec("1000")},
		{ID: 2, ContractID: 10, EntryType: "reversal", Amount: adec("-2000")},
		// Договор 11: датированный charge −1000 (2026-01).
		{ID: 3, ContractID: 11, EntryType: "charge", Amount: adec("-1000"), PeriodStart: pm("2026-01"), PeriodOK: true},
		// Пул 1000 (хватает на один долг).
		{ID: 4, ContractID: 0, EntryType: "payment", Amount: adec("1000")},
	}
	r := AllocateCoverage(e, map[uint]bool{10: true, 11: true}, AllocStrictEarmark)
	// Пул гасит ДАТИРОВАННЫЙ долг 11 (голова FIFO), бездатный 10 — в хвосте.
	cov(t, r, 11, "0.00")
	cov(t, r, 10, "-1000.00")
}

func TestParseChargePeriod(t *testing.T) {
	cases := []struct {
		ext  string
		gran string
		ok   bool
	}{
		{"autocharge:sub:42:2026-01-15", "day", true},
		{"autocharge:sub:42:2026-01", "month", true},
		{"autocharge:sub:42:y:2026-01", "year", true},
		{"autocharge:sub:42:2026-01:garbage", "", false}, // мусорный хвост (Codex 5.2)
		{"autocharge:sub:42:y:2026-01:x", "", false},     // лишний сегмент в y-форме
		{"manual-payment-ref-777", "", false},
		{"autocharge:sub:42:y:bad", "", false},
		{"autocharge:sub:42", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		_, gran, ok := ParseChargePeriod(c.ext)
		if ok != c.ok || gran != c.gran {
			t.Errorf("ParseChargePeriod(%q) = (%q,%v), want (%q,%v)", c.ext, gran, ok, c.gran, c.ok)
		}
	}
}
