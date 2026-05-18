package services

import (
	"log"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"

	"backend_axenta/database"
	"backend_axenta/models"
)

// geliosSyncWorkers — размер worker-pool тика (env GELIOS_SYNC_WORKERS,
// дефолт 4). Распараллеливает sync РАЗНЫХ connections; один conn
// защищён per-conn TryLock в SyncUnits.
func geliosSyncWorkers() int {
	w := 4
	if v := os.Getenv("GELIOS_SYNC_WORKERS"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			w = n
		}
	}
	return w
}

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
	// tickRunning — self-guard tick(): закрывает overlap НЕЗАВИСИМО от
	// вызывающего (cron-chain SkipIfStillRunning не покрывает первичный
	// `go s.tick()` из Start()). Belt-and-suspenders с cron-chain.
	tickRunning atomic.Bool
}

func NewGeliosSyncScheduler(tickIntervalMin int) *GeliosSyncScheduler {
	if tickIntervalMin <= 0 {
		tickIntervalMin = 5
	}
	return &GeliosSyncScheduler{
		cron: cron.New(
			cron.WithLocation(time.UTC),
			// SkipIfStillRunning: долгий тик не плодит внахлёст новые
			// pool'ы → нет ложных TryLock-failed++ и pool/DB-churn.
			cron.WithChain(
				cron.SkipIfStillRunning(cron.DefaultLogger),
				cron.Recover(cron.DefaultLogger),
			),
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
		// cron.Stop() возвращает ctx, закрываемый когда running job
		// завершился. Ждём bounded (graceful), не висим вечно.
		ctx := s.cron.Stop()
		select {
		case <-ctx.Done():
		case <-time.After(35 * time.Second):
			log.Printf("⚠️ GeliosSyncScheduler: Stop timeout 35s — тик ещё бежит")
		}
		s.isRunning = false
		log.Printf("🛑 GeliosSyncScheduler остановлен")
	}
}

func (s *GeliosSyncScheduler) IsRunning() bool { return s.isRunning }

func (s *GeliosSyncScheduler) tick() {
	// Self-guard: только один tick одновременно (cron-тик ИЛИ первичный
	// go-тик из Start()). Иначе — скип (overlap = ложные failed/churn).
	if !s.tickRunning.CompareAndSwap(false, true) {
		log.Printf("⏭ GeliosSyncScheduler: предыдущий тик ещё бежит — скип")
		return
	}
	defer s.tickRunning.Store(false)

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
	svc := NewGeliosService(database.DB) // *gorm.DB и http.Client concurrency-safe

	// 1. Due-фильтр последовательно (без I/O, дёшево).
	due := make([]*models.GeliosConnection, 0, len(conns))
	skipped := 0
	for i := range conns {
		conn := &conns[i]
		if conn.SyncInterval <= 0 {
			conn.SyncInterval = 15
		}
		if conn.LastSyncAt != nil &&
			now.Sub(*conn.LastSyncAt) < time.Duration(conn.SyncInterval)*time.Minute {
			skipped++
			continue
		}
		due = append(due, conn)
	}

	// 2. Bounded worker-pool по due-conns. Разные conn параллельны
	// безопасно (SyncUnits держит per-conn TryLock; conn-структуры
	// различны — каждый своя строка БД). Counters атомарны.
	workers := geliosSyncWorkers()
	if len(due) > 0 && workers > len(due) {
		workers = len(due)
	}
	var synced, failed int64
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, conn := range due {
		wg.Add(1)
		sem <- struct{}{}
		go func(c *models.GeliosConnection) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&failed, 1)
					log.Printf("❌ ПАНИКА GeliosSyncScheduler worker conn=%d: %v", c.ID, r)
				}
			}()
			log.Printf("🔄 GeliosSyncScheduler: sync conn=%d (%s) interval=%dм", c.ID, c.Name, c.SyncInterval)
			if _, err := svc.SyncUnits(c); err != nil {
				log.Printf("⚠️ GeliosSyncScheduler: conn=%d sync error: %v", c.ID, err)
				atomic.AddInt64(&failed, 1)
				return
			}
			// Users sync — best-effort: ошибка не валит весь conn-тик
			// (units уже синканы). SyncUsersFlag-гейт.
			if c.SyncUsersFlag {
				if _, err := svc.SyncGeliosUsers(c); err != nil {
					log.Printf("⚠️ GeliosSyncScheduler: conn=%d users sync error: %v", c.ID, err)
				}
			}
			atomic.AddInt64(&synced, 1)
		}(conn)
	}
	wg.Wait()

	log.Printf("✅ GeliosSyncScheduler: тик завершён, total=%d synced=%d skipped=%d failed=%d workers=%d за %v",
		len(conns), atomic.LoadInt64(&synced), skipped, atomic.LoadInt64(&failed), workers,
		time.Since(now).Round(time.Millisecond))
}
