package services

import (
	"context"
	"os"
	"strconv"
	"sync"

	"golang.org/x/time/rate"
)

// Shared per-token rate-limiter исходящих запросов к Axenta Cloud (zone#2 hardening).
//
// Проблема: Axenta лимитит 500 req/min НА ТОКЕН (~8.3 req/sec). Токен в ACRM ОДИН
// глобальный (SnapshotSettings.AxentaToken, company_id=1). Несколько путей бьют его
// параллельно: cron PartnerSnapshotScheduler (00:30 UTC) + cron AxentaSync (~10 мин,
// goroutines) + manual backfill (POST /snapshot-jobs/trigger) + /axenta-sync/trigger.
// Точечные sleep'ы (200/100ms) внутри каждого пути НЕ shared → суммарный rate двух
// путей складывается → >500/min → 429.
//
// Фикс: ПРОЦЕСС-ШИРОКИЙ per-token rate.Limiter (lazy map, как wialon_limiter семафор).
// Каждый исходящий Axenta-вызов делает waitAxentaToken(ctx, token) ПЕРЕД запросом —
// limiter сериализует совокупный rate всех путей под один потолок независимо от их числа.
// Дефолт 7 req/sec (420/min) — запас под 500 ceiling; override env AXENTA_RATE_PER_SEC.
//
// SCOPE (per-token, Codex-ревью): limiter ключуется по СТРОКЕ ТОКЕНА. Обёрнуты ВСЕ
// Axenta call-sites, использующие ГЛОБАЛЬНЫЙ snapshot-токен (5 в partner_snapshot, 1
// accumulation, 2 account_hierarchy, 2 axenta_sync) — это и есть коллизия zone#2. ДРУГИЕ
// Axenta-вызовы (api/* proxy-мутации login_as/create, axenta_user_service CRUD, auth-login
// в axetna_client, axenta_server_token) идут на ПЕР-ЮЗЕР/ПЕР-КОМПАНИЯ токенах = ОТДЕЛЬНЫЙ
// 500/min budget → НЕ коллизируют со снимками, осознанно вне scope. Если кто-то на ТОТ ЖЕ
// токен сделает необёрнутый вызов — он не посчитается и rate может превысить потолок.
//
// Wait блокирует (Background, без timeout) — для фоновых snapshot/sync-джобов (триггеры
// async, HTTP-запрос не висит) ОЖИДАНИЕ = ПРАВИЛЬНОЕ поведение: backfill идёт ровно так
// медленно, чтобы держаться под 500/min, а не падать в 429. Шатдаун убьёт процесс.
const (
	axentaTokenRatePerSecDefault = 7
	axentaTokenBurstDefault      = 7
)

var (
	axentaTokenLimitersMu sync.Mutex
	axentaTokenLimiters   = map[string]*rate.Limiter{}
)

// axentaRatePerSec — целевой rate (req/sec) на токен; env AXENTA_RATE_PER_SEC переопределяет.
// Clamp ≤8: 8 req/sec + burst = ~8*60+8 = 488/min < 500 ceiling. 9+ сломало бы инвариант
// (549/min → 429), поэтому env не может отключить защиту (Codex).
func axentaRatePerSec() int {
	if v := os.Getenv("AXENTA_RATE_PER_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 8 {
				n = 8
			}
			return n
		}
	}
	return axentaTokenRatePerSecDefault
}

func axentaTokenLimiter(token string) *rate.Limiter {
	axentaTokenLimitersMu.Lock()
	defer axentaTokenLimitersMu.Unlock()
	l, ok := axentaTokenLimiters[token]
	if !ok {
		r := axentaRatePerSec() // ≥1 (дефолт 7, env только при n>0)
		l = rate.NewLimiter(rate.Limit(r), r)
		axentaTokenLimiters[token] = l
	}
	return l
}

// waitAxentaToken блокирует до доступного слота в shared per-token rate-limiter.
// Пустой токен → no-op (нечего лимитировать). Возвращает ошибку только если ctx
// отменён/истёк (вызывающий обрабатывает как сетевую ошибку запроса).
func waitAxentaToken(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return axentaTokenLimiter(token).Wait(ctx)
}
