package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

// WialonBillingPlansScheduler синхронизирует тарифные планы Wialon в БД @every 1h.
// Лёгкий запрос (1-2с на connection), но нет смысла дёргать его на каждое открытие
// формы создания аккаунта — тарифы меняются редко.
type WialonBillingPlansScheduler struct {
	cron      *cron.Cron
	service   *WialonAccountService
	interval  time.Duration
	isRunning bool
	entryID   cron.EntryID
}

func NewWialonBillingPlansScheduler(intervalMinutes int) *WialonBillingPlansScheduler {
	if intervalMinutes <= 0 {
		intervalMinutes = 60
	}
	return &WialonBillingPlansScheduler{
		cron: cron.New(
			cron.WithLocation(time.UTC),
			cron.WithChain(cron.Recover(cron.DefaultLogger)),
		),
		service:  NewWialonAccountService(),
		interval: time.Duration(intervalMinutes) * time.Minute,
	}
}

func (s *WialonBillingPlansScheduler) Start() error {
	cronExpr := "@every " + s.interval.String()
	id, err := s.cron.AddFunc(cronExpr, func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в WialonBillingPlansScheduler: %v", r)
			}
		}()
		log.Printf("🕐 WialonBillingPlansScheduler: запуск sync тарифов")
		s.syncAll()
	})
	if err != nil {
		return err
	}
	s.entryID = id

	// Первичный sync при старте сервера (background, не блокируем main)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в первичном WialonBillingPlansScheduler: %v", r)
			}
		}()
		log.Printf("🚀 WialonBillingPlansScheduler: первичный sync тарифов (background)")
		s.syncAll()
	}()

	s.cron.Start()
	s.isRunning = true
	log.Printf("✅ WialonBillingPlansScheduler запущен (интервал %s)", s.interval)
	return nil
}

func (s *WialonBillingPlansScheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
		s.isRunning = false
		log.Printf("🛑 WialonBillingPlansScheduler остановлен")
	}
}

func (s *WialonBillingPlansScheduler) IsRunning() bool { return s.isRunning }

func (s *WialonBillingPlansScheduler) syncAll() {
	t0 := time.Now()
	var conns []models.WialonConnection
	if err := database.DB.Where("is_active = ?", true).Find(&conns).Error; err != nil {
		log.Printf("⚠️ WialonBillingPlansScheduler: ошибка получения connections: %v", err)
		return
	}
	ok, fail := 0, 0
	for _, c := range conns {
		if _, err := s.service.SyncBillingPlans(c.ID); err != nil {
			log.Printf("⚠️ billing-plans sync conn=%d (%s): %v", c.ID, c.Name, err)
			fail++
			continue
		}
		ok++
	}
	log.Printf("✅ WialonBillingPlansScheduler: %d/%d ok, fail=%d, за %s", ok, len(conns), fail, time.Since(t0))
}
