package services

import (
	"context"
	"log"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
)

// Лимитер конкурентных тяжёлых операций Wialon ПО ОДНОМУ ТОКЕНУ.
//
// Проблема: после рестарта несколько schedulers (all-accounts, stats,
// billing-plans, ...) одновременно бьют один и тот же Wialon-токен (напр.
// glomoskz). Wialon throttles при множестве параллельных сессий/поисков на
// токен — даже быстрые force=0 страницы начинают занимать 7-17s вместо 0.5s.
//
// Семафор cap=2 на токен → максимум 2 тяжёлых операции на токен одновременно,
// остальные ждут (или пропускают цикл по таймауту). Разные токены не мешают.
//
// КОНТРАКТ (leak-proof, без re-entrancy):
//   - acquire вызывать ТОЛЬКО в top-level per-connection функциях
//     (GetAccountsBatchFromHost, StatsService.collectForConnection, billing sync);
//   - НЕ вызывать во вложенных helper'ах, переиспользующих ту же сессию (eid),
//     и НЕ в LoginWithHost — иначе одна горутина возьмёт слот дважды → deadlock;
//   - всегда `release, ok := acquireWialonToken(...); if !ok { return }; defer release()`.
const (
	wialonTokenMaxConcurrent    = 2
	wialonTokenAcquireTimeout   = 8 * time.Minute // > httpClient timeout (300s); слот держится не дольше одной операции
)

var (
	wialonTokenSemsMu sync.Mutex
	wialonTokenSems   = map[string]*semaphore.Weighted{}
)

func wialonTokenSem(token string) *semaphore.Weighted {
	wialonTokenSemsMu.Lock()
	defer wialonTokenSemsMu.Unlock()
	s, ok := wialonTokenSems[token]
	if !ok {
		s = semaphore.NewWeighted(wialonTokenMaxConcurrent)
		wialonTokenSems[token] = s
	}
	return s
}

// acquireWialonToken берёт слот для тяжёлой операции по токену. Возвращает
// release-функцию и ok=false если слот не получен за таймаут (тогда вызывающий
// пропускает операцию — следующий cron-тик повторит, кэш/TTL подстрахуют).
func acquireWialonToken(token, label string) (func(), bool) {
	if token == "" {
		return func() {}, true // нет токена — нечего сериализовать
	}
	sem := wialonTokenSem(token)
	ctx, cancel := context.WithTimeout(context.Background(), wialonTokenAcquireTimeout)
	defer cancel()
	if err := sem.Acquire(ctx, 1); err != nil {
		log.Printf("⚠️ Wialon limiter: %s не дождался слота за %s (токен занят) — пропуск цикла", label, wialonTokenAcquireTimeout)
		return func() {}, false
	}
	var once sync.Once
	return func() { once.Do(func() { sem.Release(1) }) }, true
}

// wialonStartupDelay — детерминированный стартовый сдвиг первичного прогона каждого
// scheduler'а, чтобы после рестарта 5 schedulers не стартовали одновременно по одному
// токену (anti-stampede на старте). Семафор всё равно сериализует, но stagger убирает
// синхронный thundering-herd на Acquire и разносит работу по разным токенам.
func wialonStartupDelay(label string) time.Duration {
	switch label {
	case "WialonAllAccountsScheduler":
		return 7 * time.Second
	case "WialonAllUnitsScheduler":
		return 35 * time.Second
	case "WialonStatsScheduler":
		return 65 * time.Second
	case "WialonBillingPlansScheduler":
		return 95 * time.Second
	default:
		return 15 * time.Second
	}
}
