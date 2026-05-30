package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

// zone#2: shared per-token Axenta rate-limiter.
func TestAxentaRatePerSec_DefaultAndEnv(t *testing.T) {
	t.Setenv("AXENTA_RATE_PER_SEC", "")
	assert.Equal(t, 7, axentaRatePerSec(), "дефолт 7 req/sec (420/min под 500 ceiling)")
	t.Setenv("AXENTA_RATE_PER_SEC", "5")
	assert.Equal(t, 5, axentaRatePerSec(), "env override")
	t.Setenv("AXENTA_RATE_PER_SEC", "0")
	assert.Equal(t, 7, axentaRatePerSec(), "невалидный 0 → дефолт")
	t.Setenv("AXENTA_RATE_PER_SEC", "abc")
	assert.Equal(t, 7, axentaRatePerSec(), "невалидный текст → дефолт")
	t.Setenv("AXENTA_RATE_PER_SEC", "9")
	assert.Equal(t, 8, axentaRatePerSec(), "clamp ≤8 (9 дал бы 549/min > 500 ceiling)")
	t.Setenv("AXENTA_RATE_PER_SEC", "100")
	assert.Equal(t, 8, axentaRatePerSec(), "clamp ≤8 (env не отключает защиту)")
}

func TestAxentaTokenLimiter_SharedPerToken(t *testing.T) {
	// Один токен → ОДИН shared limiter (ключевое для zone#2: cron+manual делят инстанс).
	l1 := axentaTokenLimiter("zone2-tok-A")
	l2 := axentaTokenLimiter("zone2-tok-A")
	assert.Same(t, l1, l2, "тот же токен → тот же *rate.Limiter (shared между путями)")
	// Разные токены → разные limiter'ы.
	l3 := axentaTokenLimiter("zone2-tok-B")
	assert.NotSame(t, l1, l3, "разные токены изолированы")
	// Limit/Burst соответствуют конфигу.
	assert.Equal(t, rate.Limit(7), l1.Limit(), "limit = 7 req/sec")
	assert.Equal(t, 7, l1.Burst(), "burst = 7")
}

func TestWaitAxentaToken_EmptyTokenNoop(t *testing.T) {
	// Пустой токен → no-op (нечего лимитировать), не блокирует.
	assert.NoError(t, waitAxentaToken(context.Background(), ""))
	// Непустой токен с Background → возвращается (первый запрос в burst мгновенный).
	assert.NoError(t, waitAxentaToken(context.Background(), "zone2-tok-noop"))
}
