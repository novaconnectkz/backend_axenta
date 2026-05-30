package services

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// rateSources — источники курсов, загружаемые ежедневным прогоном (cbr_rf=RUB, nbk_kz=KZT).
var rateSources = []string{"cbr_rf", "nbk_kz"}

// CurrencyRateScheduler — ежедневная загрузка курсов валют (П5 мультивалюта).
// Идемпотентно (upsert по unique), поэтому повторный прогон безопасен.
type CurrencyRateScheduler struct {
	cron      *cron.Cron
	svc       *CurrencyRateService
	mu        sync.Mutex // защищает status-поля + сериализует RunForDate (cron+ручной, Codex #6)
	isRunning bool
	lastRun   time.Time
}

func NewCurrencyRateScheduler() *CurrencyRateScheduler {
	c := cron.New(
		cron.WithLocation(time.UTC),
		// SkipIfStillRunning — cron не запускает новый прогон поверх идущего (Codex #6).
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger), cron.Recover(cron.DefaultLogger)),
	)
	return &CurrencyRateScheduler{cron: c, svc: NewCurrencyRateService()}
}

// Start — ежедневно 04:00 UTC (после charge-движка 02:00; ЦБ публикует курс на след.
// рабочий день вечером предыдущего, к утру UTC доступен). Грузим за сегодня.
func (s *CurrencyRateScheduler) Start() error {
	_, err := s.cron.AddFunc("0 4 * * *", func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в CurrencyRateScheduler: %v", r)
			}
		}()
		if n, _, e := s.RunForDate(time.Now().UTC()); e != nil {
			log.Printf("⚠️ CurrencyRate cron: %v", e)
		} else {
			log.Printf("💱 CurrencyRate cron: загружено %d курсов", n)
		}
	})
	if err != nil {
		return err
	}
	s.cron.Start()
	s.mu.Lock()
	s.isRunning = true
	s.mu.Unlock()
	log.Printf("✅ CurrencyRateScheduler запущен (ежедневно 04:00 UTC, источники: %s)", strings.Join(rateSources, ", "))
	return nil
}

func (s *CurrencyRateScheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
		s.mu.Lock()
		s.isRunning = false
		s.mu.Unlock()
		log.Println("🛑 CurrencyRateScheduler остановлен")
	}
}

// RunForDate загружает курсы всех источников за указанную дату. Возвращает суммарное
// кол-во загруженных, список ошибок по источникам (для видимости partial-сбоя, Codex F2)
// и error. Partial-success: если хотя бы ОДИН источник отработал без ошибки — успех (один
// недоступный источник, напр. NBK из РФ, не валит весь прогон). error только если КАЖДЫЙ
// источник упал. Сериализован mutex'ом (cron+ручной POST, Codex #6).
func (s *CurrencyRateScheduler) RunForDate(date time.Time) (int, []string, error) {
	return s.runForSources(date, rateSources)
}

// RunForSource загружает курсы одного источника (ручной точечный fetch).
func (s *CurrencyRateScheduler) RunForSource(date time.Time, source string) (int, []string, error) {
	return s.runForSources(date, []string{source})
}

func (s *CurrencyRateScheduler) runForSources(date time.Time, sources []string) (int, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	successes := 0 // источников без ошибки (Codex F3: решение по successes, не по total)
	var errs []string
	for _, src := range sources {
		n, err := s.svc.FetchForDate(src, date)
		if err != nil {
			errs = append(errs, src+": "+err.Error())
			log.Printf("⚠️ CurrencyRate: источник %s — %v", src, err)
			continue
		}
		successes++
		total += n
		log.Printf("💱 CurrencyRate: источник %s — загружено %d курсов", src, n)
	}
	// lastRun обновляем только при ≥1 успешном источнике (Codex F1: полный провал не
	// должен выглядеть как успешный прогон).
	if successes > 0 {
		s.lastRun = time.Now()
	}
	// Частичный сбой (часть источников отработала, часть упала) — WARN, чтобы постоянно
	// недоступный источник был виден без раскопок (Codex F2).
	if successes > 0 && len(errs) > 0 {
		log.Printf("⚠️ CurrencyRate: частичный сбой — успешно %d/%d источников, ошибки: %s",
			successes, len(sources), strings.Join(errs, "; "))
	}
	// Ни один источник не отработал — это ошибка (Codex F3: по successes, не total —
	// источник может легитимно вернуть 0 строк без ошибки).
	if successes == 0 && len(errs) > 0 {
		return 0, errs, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return total, errs, nil
}

func (s *CurrencyRateScheduler) GetStatus() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]interface{}{
		"is_running": s.isRunning,
		"last_run":   s.lastRun,
	}
}
