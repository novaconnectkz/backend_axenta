package services

import (
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Б0: детерминированный per-договор аллокатор покрытия поверх неизменяемых проводок.
// READ-ONLY: ничего не пишет, в проде пока никем не вызывается (фундамент для Б4-блокировки).
//
// Модель: деньги контрагента — ОДИН пул (UX «один счёт»), но обязательство и блокировка —
// PER-ДОГОВОР. coverage(X) = earmark(X) − obligation(X) + FIFO-доля общего пула. Договор в
// долге ⇔ coverage(X) < −limit(X) (порог — забота вызывающего, в валюте договора, без конверсии).
//
// Один вызов = ОДНА валюта одного контрагента (вызывающий группирует по (cp, currency)).
// Подробности и инварианты — wiki/concepts/wallet-percontract-gating.

// Политики переплаты earmark (излишек после полного покрытия своего договора).
const (
	AllocStrictEarmark = "strict_earmark" // дефолт: излишек заперт за договором, в FIFO не идёт
	AllocWalletRelease = "wallet_release" // излишек → общий пул (ближе к «единый кошелёк»)
)

// AllocEntry — проводка для аллокатора (одна валюта). Amount знаковый, УЖЕ Round(2).
type AllocEntry struct {
	ID          uint
	ContractID  uint
	EntryType   string // charge|payment|adjustment|reversal|migration_balance|transfer_in|transfer_out
	Amount      decimal.Decimal
	PeriodStart time.Time // логический период (для FIFO-упорядочивания долгов)
	PeriodOK    bool      // период резолвлен; для debt-проводок (charge) обязателен, иначе quarantine
}

// AllocResult — итог аллокации одной валюты контрагента.
type AllocResult struct {
	Coverage       map[uint]decimal.Decimal // per договор: покрытие (отрицательное = непокрытый долг). НЕ содержит карантинных.
	WalletFreePool decimal.Decimal          // нераспределённый пул (предоплата без адресата)
	// QuarantinedContracts — договоры с обязательством без резолвленного периода: их состояние
	// НЕ меняется (ни suspend, ни restore, ни fulfill — needs_manual_reconcile). Решение №7:
	// карантин per-ДОГОВОР (раньше один такой charge морозил всю (cp,ccy)) — нерезолвленный
	// charge изолирует ТОЛЬКО свой договор, остальные договоры контрагента решаются нормально.
	QuarantinedContracts map[uint]bool
}

// farFutureFIFO — сентинел для FIFO-упорядочивания долгов БЕЗ датированного начисления
// (долг от reversal/transfer_out, обнулившего earmark): такой долг ставится в ХВОСТ очереди,
// а не в голову (zero-time иначе вытеснил бы датированные долги — Codex 7.1).
var farFutureFIFO = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// isObligation — проводка-обязательство (period-bound долг): charge или отрицательный
// adjustment (adjustment-charge по дизайну). Требует резолвленного периода (иначе quarantine)
// и участвует в FIFO-упорядочивании. transfer_out/reversal — НЕ obligation (движения/коррекции).
func isObligation(e AllocEntry) bool {
	return (e.EntryType == "charge" || e.EntryType == "adjustment") && e.ContractID != 0 && e.Amount.IsNegative()
}

// AllocateCoverage — чистая детерминированная аллокация. entries — все проводки (cp, ccy);
// openContracts — договоры в active|suspended (кандидаты на FIFO-погашение и suspend). Порядок
// entries НЕ влияет на результат (детерминизм по period_start, charge_id, contract_id).
//
// ВНИМАНИЕ вызывающему (Б4 sweep): карантин — PER-ДОГОВОР, НЕ глобальный (решение №7).
// Проверяй res.QuarantinedContracts[contractID] для КАЖДОГО договора: отсутствие договора в
// res.Coverage = не делать suspend/restore ТОЛЬКО по нему (needs_manual_reconcile), а НЕ
// фриз всей единицы (cp,ccy). Соседи контрагента решаются по своему Coverage как обычно.
func AllocateCoverage(entries []AllocEntry, openContracts map[uint]bool, policy string) AllocResult {
	res := AllocResult{Coverage: map[uint]decimal.Decimal{}, QuarantinedContracts: map[uint]bool{}}

	// Quarantine per-ДОГОВОР (решение №7): обязательство (charge/adjustment<0) с нерезолвленным
	// периодом морозит ТОЛЬКО свой договор (needs_manual_reconcile) — соседи контрагента решаются.
	// Codex 5.1: не только charge, adjustment<0 тоже обязательство.
	for _, e := range entries {
		if isObligation(e) && !e.PeriodOK {
			res.QuarantinedContracts[e.ContractID] = true
		}
	}

	obligation := map[uint]decimal.Decimal{} // >=0, по charge
	earmark := map[uint]decimal.Decimal{}    // целевые деньги договора (знаковые: + платёж, − transfer_out/reversal)
	pool := decimal.Zero                     // untagged (contract_id=0)
	earliest := map[uint]time.Time{}         // период старейшего charge договора (FIFO-ключ)
	earliestID := map[uint]uint{}            // id старейшего charge (tie-break)

	for _, e := range entries {
		// Карантинный договор изолируется ЦЕЛИКОМ: его проводки не влияют ни на coverage
		// соседей, ни на общий пул (без резолвленного периода достоверно посчитать его
		// покрытие нельзя). contract_id=0 (untagged-пул) карантину не подлежит.
		if e.ContractID != 0 && res.QuarantinedContracts[e.ContractID] {
			continue
		}
		switch {
		case isObligation(e): // charge или adjustment<0 на договор → обязательство (period-bound)
			obligation[e.ContractID] = obligation[e.ContractID].Add(e.Amount.Abs())
			if t, ok := earliest[e.ContractID]; !ok || e.PeriodStart.Before(t) ||
				(e.PeriodStart.Equal(t) && e.ID < earliestID[e.ContractID]) {
				earliest[e.ContractID] = e.PeriodStart
				earliestID[e.ContractID] = e.ID
			}
		case e.EntryType == "transfer_out":
			// Списание из earmark/пула источника (НЕ обязательство — иначе фантомный долг).
			if e.ContractID != 0 {
				earmark[e.ContractID] = earmark[e.ContractID].Add(e.Amount) // amount<0
			} else {
				pool = pool.Add(e.Amount)
			}
		case e.Amount.IsPositive() && e.ContractID != 0:
			// payment|adjustment|transfer_in|reversal(+) на договор → earmarked деньги договора.
			earmark[e.ContractID] = earmark[e.ContractID].Add(e.Amount)
		case e.Amount.IsPositive() && e.ContractID == 0:
			pool = pool.Add(e.Amount) // untagged платёж/пополнение
		case e.Amount.IsNegative() && e.ContractID != 0:
			// reversal/adjustment(−) на договор (не charge) → уменьшает earmark договора.
			earmark[e.ContractID] = earmark[e.ContractID].Add(e.Amount)
		default:
			// amount<0, contract_id==0 (reversal/сторно пула) → уменьшает пул.
			pool = pool.Add(e.Amount)
		}
	}

	// Шаг 1-2: coverage = earmark − obligation; переплату earmark по политике (НЕ в FIFO).
	all := map[uint]bool{}
	for c := range obligation {
		all[c] = true
	}
	for c := range earmark {
		all[c] = true
	}
	for c := range openContracts {
		if !res.QuarantinedContracts[c] { // карантинному договору coverage не считаем
			all[c] = true
		}
	}
	for c := range all {
		cov := earmark[c].Sub(obligation[c])
		if cov.IsPositive() {
			if policy == AllocWalletRelease {
				pool = pool.Add(cov) // излишек → общий пул
			}
			cov = decimal.Zero // strict: заперт за договором; release: уже ушёл в пул
		}
		res.Coverage[c] = cov
	}

	// Шаг 3: FIFO — общий пул гасит ОТКРЫТЫЕ договоры в долге по стабильному ключу
	// (старейший период charge, id старейшего charge, contract_id), частично-жадно.
	type deb struct {
		t   time.Time
		id  uint
		cid uint
	}
	debts := make([]deb, 0)
	for c, cov := range res.Coverage {
		if cov.IsNegative() && openContracts[c] {
			t, ok := earliest[c]
			if !ok {
				t = farFutureFIFO // долг без датированного начисления → в хвост FIFO (Codex 7.1)
			}
			debts = append(debts, deb{t, earliestID[c], c})
		}
	}
	sort.Slice(debts, func(i, j int) bool {
		if !debts[i].t.Equal(debts[j].t) {
			return debts[i].t.Before(debts[j].t)
		}
		if debts[i].id != debts[j].id {
			return debts[i].id < debts[j].id
		}
		return debts[i].cid < debts[j].cid
	})
	for _, d := range debts {
		if !pool.IsPositive() {
			break
		}
		need := res.Coverage[d.cid].Abs()
		apply := decimal.Min(pool, need)
		res.Coverage[d.cid] = res.Coverage[d.cid].Add(apply)
		pool = pool.Sub(apply)
	}
	res.WalletFreePool = pool
	return res
}

// ParseChargePeriod резолвит логический период charge из external_id (autocharge-форматы
// планировщика, ledger_charge_scheduler.go):
//
//	autocharge:sub:<id>:YYYY-MM-DD → day
//	autocharge:sub:<id>:YYYY-MM    → month
//	autocharge:sub:<id>:y:YYYY-MM  → year
//
// Возвращает (период, гранулярность, ok). Непарсируемый (ручной/legacy/чужой формат) → ok=false
// → debt-проводка уходит в quarantine (needs_manual_reconcile), авто-состояние не меняется.
func ParseChargePeriod(externalID string) (time.Time, string, bool) {
	const pfx = "autocharge:sub:"
	if !strings.HasPrefix(externalID, pfx) {
		return time.Time{}, "", false
	}
	parts := strings.Split(externalID[len(pfx):], ":") // [<id>, <date>] либо [<id>, y, <YYYY-MM>]
	if len(parts) < 2 {
		return time.Time{}, "", false
	}
	if parts[1] == "y" {
		// Годовой lump: РОВНО [<id>, y, <YYYY-MM>] — лишний хвост = мусор (Codex 5.2).
		if len(parts) != 3 {
			return time.Time{}, "", false
		}
		if t, err := time.Parse("2006-01", parts[2]); err == nil {
			return t, "year", true
		}
		return time.Time{}, "", false
	}
	// day/month: РОВНО [<id>, <date>] — лишний сегмент = мусор (Codex 5.2).
	if len(parts) != 2 {
		return time.Time{}, "", false
	}
	if t, err := time.Parse("2006-01-02", parts[1]); err == nil {
		return t, "day", true
	}
	if t, err := time.Parse("2006-01", parts[1]); err == nil {
		return t, "month", true
	}
	return time.Time{}, "", false
}
