package services

import (
	"log"
	"time"

	"github.com/robfig/cron/v3"

	"backend_axenta/database"
	"backend_axenta/models"
)

// GeliosSyncScheduler — фоновый sync объектов GELIOS GPS.
//
// Тикает каждые tickIntervalMin минут (по умолчанию 5). На каждый тик
// перебирает gelios_connections где is_active=true AND auto_sync_enabled=true
// и для каждой проверяет per-connection sync_interval с last_sync_at.
// Если пора — вызывает GeliosService.SyncUnits().
//
// Зеркалит SkifSyncScheduler. GELIOS проще: только SyncUnits (нет
// company-statuses/subdealers/users — этих API у GELIOS нет).
type GeliosSyncScheduler struct {
	cron         *cron.Cron
	tickInterval time.Duration
	isRunning    bool
	entryID      cron.EntryID
}

func NewGeliosSyncScheduler(tickIntervalMin int) *GeliosSyncScheduler {
	if tickIntervalMin <= 0 {
		tickIntervalMin = 5
	}
	return &GeliosSyncScheduler{
		cron: cron.New(
			cron.WithLocation(time.UTC),
			cron.WithChain(cron.Recover(cron.DefaultLogger)),
		),
		tickInterval: time.Duration(tickIntervalMin) * time.Minute,
	}
}

func (s *GeliosSyncScheduler) Start() error {
	id, err := s.cron.AddFunc("@every "+s.tickInterval.String(), s.tick)
	if err != nil {
		return err
	}
	s.entryID = id

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в первичном GeliosSyncScheduler: %v", r)
			}
		}()
		log.Printf("🚀 GeliosSyncScheduler: первичный прогон (background)")
		s.tick()
	}()

	s.cron.Start()
	s.isRunning = true
	log.Printf("✅ GeliosSyncScheduler запущен (тик %s)", s.tickInterval)
	return nil
}

func (s *GeliosSyncScheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
		s.isRunning = false
		log.Printf("🛑 GeliosSyncScheduler остановлен")
	}
}

func (s *GeliosSyncScheduler) IsRunning() bool { return s.isRunning }

func (s *GeliosSyncScheduler) tick() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ ПАНИКА в GeliosSyncScheduler.tick: %v", r)
		}
	}()

	if database.DB == nil {
		log.Printf("⚠️ GeliosSyncScheduler: database.DB == nil, пропускаем тик")
		return
	}

	var conns []models.GeliosConnection
	if err := database.DB.
		Where("is_active = ? AND auto_sync_enabled = ?", true, true).
		Find(&conns).Error; err != nil {
		log.Printf("⚠️ GeliosSyncScheduler: ошибка загрузки connections: %v", err)
		return
	}
	if len(conns) == 0 {
		return
	}

	now := time.Now()
	svc := NewGeliosService(database.DB)
	synced, skipped, failed := 0, 0, 0

	for i := range conns {
		conn := &conns[i]
		if conn.SyncInterval <= 0 {
			conn.SyncInterval = 15
		}
		if conn.LastSyncAt != nil {
			if now.Sub(*conn.LastSyncAt) < time.Duration(conn.SyncInterval)*time.Minute {
				skipped++
				continue
			}
		}
		log.Printf("🔄 GeliosSyncScheduler: sync conn=%d (%s) interval=%dм", conn.ID, conn.Name, conn.SyncInterval)
		if _, err := svc.SyncUnits(conn); err != nil {
			log.Printf("⚠️ GeliosSyncScheduler: conn=%d sync error: %v", conn.ID, err)
			failed++
			continue
		}
		synced++
	}

	log.Printf("✅ GeliosSyncScheduler: тик завершён, total=%d synced=%d skipped=%d failed=%d за %v",
		len(conns), synced, skipped, failed, time.Since(now).Round(time.Millisecond))
}
