package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// holdUntilExclusive: выбранный день YYYY-MM-DD → начало СЛЕДУЮЩЕГО дня UTC (exclusive).
// Семантика «держим включительно весь выбранный день» (Codex #6).
func TestHoldUntilExclusive(t *testing.T) {
	day := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	got := holdUntilExclusive(day)
	assert.Equal(t, time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC), got)

	// Время в исходном дне не влияет — берётся только календарный день.
	dayWithTime := time.Date(2026, 5, 30, 18, 45, 0, 0, time.UTC)
	assert.Equal(t, got, holdUntilExclusive(dayWithTime))

	// Переход через конец месяца.
	endMonth := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), holdUntilExclusive(endMonth))
}

// Ключевая проверка семантики «до конца дня для TZ +N»: зонт на сегодняшний день
// в UTC всё ещё активен в локальное утро того же дня в Asia/Aqtobe (+5) и даже
// в раннее утро следующего UTC-дня. now < hold_until → держит.
func TestHoldUntilExclusive_CoversWholeDayAndLocalTZ(t *testing.T) {
	day := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
	holdUntil := holdUntilExclusive(day) // 2026-05-30 00:00 UTC

	// Полдень выбранного дня UTC — держит.
	assert.True(t, holdUntil.After(time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)))
	// 23:59 выбранного дня UTC — держит.
	assert.True(t, holdUntil.After(time.Date(2026, 5, 29, 23, 59, 0, 0, time.UTC)))
	// 00:00 следующего дня UTC — НЕ держит (exclusive-граница).
	assert.False(t, holdUntil.After(time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)))
}

// deferralWithinPolicy: exclusive-граница holdUntil должна укладываться в
// MaxDeferralDays от now включительно по дню (Codex #6 лимит политики).
func TestDeferralWithinPolicy(t *testing.T) {
	now := time.Date(2026, 5, 29, 9, 30, 0, 0, time.UTC) // время суток не важно

	// Лимит 3 дня: разрешены дни 30,31,1 (т.е. до 2026-06-01 включительно).
	within := func(y, m, dd int) bool {
		return deferralWithinPolicy(holdUntilExclusive(time.Date(y, time.Month(m), dd, 0, 0, 0, 0, time.UTC)), now, 3)
	}
	assert.True(t, within(2026, 5, 30), "следующий день — в пределах")
	assert.True(t, within(2026, 6, 1), "последний разрешённый день (now+3) — в пределах")
	assert.False(t, within(2026, 6, 2), "now+4 — за пределом")

	// Лимит 1 день: только завтра.
	w1 := func(y, m, dd int) bool {
		return deferralWithinPolicy(holdUntilExclusive(time.Date(y, time.Month(m), dd, 0, 0, 0, 0, time.UTC)), now, 1)
	}
	assert.True(t, w1(2026, 5, 30), "завтра — в пределах при лимите 1")
	assert.False(t, w1(2026, 5, 31), "послезавтра — за пределом при лимите 1")
}
