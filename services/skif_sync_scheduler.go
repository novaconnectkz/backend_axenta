package services

import (
	"log"
	"time"

	"github.com/robfig/cron/v3"

	"backend_axenta/database"
	"backend_axenta/models"
)

// SkifSyncScheduler — фоновый sync объектов SKIF.PRO.
//
// Тикает каждые `tickIntervalMin` минут (по умолчанию 5). На каждый тик
// перебирает все skif_connections где is_active=true AND auto_sync_enabled=true
// и для каждой проверяет: `last_sync_at + connection.sync_interval >= now`.
// Если пора — вызывает SkifService.SyncUnits().
//
// Это позволяет иметь per-connection sync_interval (15 / 30 / 60 мин)
// без отдельного cron-job'а на каждое подключение.
type SkifSyncScheduler struct {
	cron            *cron.Cron
	tickInterval    time.Duration
	isRunning       bool
	entryID         cron.EntryID
}

func NewSkifSyncScheduler(tickIntervalMin int) *SkifSyncScheduler {
	if tickIntervalMin <= 0 {
		tickIntervalMin = 5
	}
	return &SkifSyncScheduler{
		cron: cron.New(
			cron.WithLocation(time.UTC),
			cron.WithChain(cron.Recover(cron.DefaultLogger)),
		),
		tickInterval: time.Duration(tickIntervalMin) * time.Minute,
	}
}

func (s *SkifSyncScheduler) Start() error {
	cronExpr := "@every " + s.tickInterval.String()
	id, err := s.cron.AddFunc(cronExpr, s.tick)
	if err != nil {
		return err
	}
	s.entryID = id

	// Первый прогон сразу в goroutine.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в первичном SkifSyncScheduler: %v", r)
			}
		}()
		log.Printf("🚀 SkifSyncScheduler: первичный прогон (background)")
		s.tick()
	}()

	s.cron.Start()
	s.isRunning = true
	log.Printf("✅ SkifSyncScheduler запущен (тик %s)", s.tickInterval)
	return nil
}

func (s *SkifSyncScheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
		s.isRunning = false
		log.Printf("🛑 SkifSyncScheduler остановлен")
	}
}

func (s *SkifSyncScheduler) IsRunning() bool { return s.isRunning }

// tick — основная функция тика: загружает enabled connections и синкает те,
// у которых истёк sync_interval с последнего sync.
func (s *SkifSyncScheduler) tick() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ ПАНИКА в SkifSyncScheduler.tick: %v", r)
		}
	}()

	if database.DB == nil {
		log.Printf("⚠️ SkifSyncScheduler: database.DB == nil, пропускаем тик")
		return
	}

	var conns []models.SkifConnection
	if err := database.DB.
		Where("is_active = ? AND auto_sync_enabled = ?", true, true).
		Find(&conns).Error; err != nil {
		log.Printf("⚠️ SkifSyncScheduler: ошибка загрузки connections: %v", err)
		return
	}

	if len(conns) == 0 {
		return
	}

	now := time.Now()
	svc := NewSkifService(database.DB)
	synced := 0
	skipped := 0
	failed := 0

	for i := range conns {
		conn := &conns[i]
		if conn.SyncInterval <= 0 {
			conn.SyncInterval = 15
		}
		// Per-connection due-check
		if conn.LastSyncAt != nil {
			elapsed := now.Sub(*conn.LastSyncAt)
			if elapsed < time.Duration(conn.SyncInterval)*time.Minute {
				skipped++
				continue
			}
		}

		log.Printf("🔄 SkifSyncScheduler: sync conn=%d (%s) interval=%dм", conn.ID, conn.Name, conn.SyncInterval)
		if _, err := svc.SyncUnits(conn); err != nil {
			log.Printf("⚠️ SkifSyncScheduler: conn=%d sync error: %v", conn.ID, err)
			failed++
			continue
		}
		// Sync billing-статусов компаний (для UI Активна/Заблокирована).
		// Не критично — ошибки не валят основной sync_units цикл.
		if _, err := svc.SyncCompanyStatuses(conn); err != nil {
			log.Printf("⚠️ SkifSyncScheduler: conn=%d statuses sync error: %v", conn.ID, err)
		}
		// Sync субинтеграторов для отображения как partner-account на /accounts.
		if _, err := svc.SyncSubdealers(conn); err != nil {
			log.Printf("⚠️ SkifSyncScheduler: conn=%d subdealers sync error: %v", conn.ID, err)
		}
		synced++
	}

	log.Printf("✅ SkifSyncScheduler: тик завершён, total=%d synced=%d skipped=%d failed=%d за %v",
		len(conns), synced, skipped, failed, time.Since(now).Round(time.Millisecond))
}
