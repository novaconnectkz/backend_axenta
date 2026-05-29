package services

import (
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// CurrencyRateScheduler — ежедневная загрузка курсов ЦБ РФ (П5 мультивалюта).
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
		if n, e := s.RunForDate(time.Now().UTC()); e != nil {
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
	log.Println("✅ CurrencyRateScheduler запущен (ежедневно 04:00 UTC, источник cbr_rf)")
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

// RunForDate загружает курсы ЦБ РФ за указанную дату. Возвращает (кол-во, error) —
// вызывающий (cron/API) отличает успех от сбоя (Codex #4). Сериализован mutex'ом:
// cron и ручной POST не бьют CBR одновременно (Codex #6).
func (s *CurrencyRateScheduler) RunForDate(date time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.svc.FetchCBRForDate(date)
	if err != nil {
		return 0, err
	}
	s.lastRun = time.Now()
	return n, nil
}

func (s *CurrencyRateScheduler) GetStatus() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]interface{}{
		"is_running": s.isRunning,
		"last_run":   s.lastRun,
	}
}
