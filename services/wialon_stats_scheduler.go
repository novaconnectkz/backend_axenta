package services

import (
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

// WialonStatsScheduler запускает WialonStatsService.CollectAll по расписанию.
// Дефолт: каждые 15 минут. Период настраивается через ENV WIALON_STATS_INTERVAL_MIN.
type WialonStatsScheduler struct {
	cron      *cron.Cron
	service   *WialonStatsService
	interval  time.Duration
	isRunning bool
	entryID   cron.EntryID
}

func NewWialonStatsScheduler(intervalMinutes int) *WialonStatsScheduler {
	if intervalMinutes <= 0 {
		intervalMinutes = 15
	}
	return &WialonStatsScheduler{
		cron: cron.New(
			cron.WithLocation(time.UTC),
			cron.WithChain(cron.Recover(cron.DefaultLogger)),
		),
		service:  NewWialonStatsService(),
		interval: time.Duration(intervalMinutes) * time.Minute,
	}
}

func (s *WialonStatsScheduler) Start() error {
	cronExpr := "@every " + s.interval.String()
	id, err := s.cron.AddFunc(cronExpr, func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в WialonStatsScheduler: %v", r)
			}
		}()
		log.Printf("🕐 WialonStatsScheduler: запуск сбора статистики Wialon")
		if _, err := s.service.CollectAll(); err != nil {
			log.Printf("⚠️ WialonStatsScheduler: ошибка CollectAll: %v", err)
		}
	})
	if err != nil {
		return err
	}
	s.entryID = id

	// Запускаем первый сбор сразу — иначе UI до первого тика scheduler'а будет показывать
	// пустую stats-таблицу. Делаем в goroutine, чтобы не блокировать старт сервера.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в первичном WialonStatsScheduler: %v", r)
			}
		}()
		// Стартовый сдвиг (anti-stampede), см. wialon_limiter.go.
		time.Sleep(wialonStartupDelay("WialonStatsScheduler"))
		log.Printf("🚀 WialonStatsScheduler: первичный сбор статистики Wialon (background)")
		if _, err := s.service.CollectAll(); err != nil {
			log.Printf("⚠️ WialonStatsScheduler: ошибка первичного CollectAll: %v", err)
		}
	}()

	s.cron.Start()
	s.isRunning = true
	log.Printf("✅ WialonStatsScheduler запущен (интервал %s)", s.interval)
	return nil
}

func (s *WialonStatsScheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
		s.isRunning = false
		log.Printf("🛑 WialonStatsScheduler остановлен")
	}
}

func (s *WialonStatsScheduler) IsRunning() bool { return s.isRunning }
