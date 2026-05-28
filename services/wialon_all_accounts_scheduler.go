package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// WialonAllAccountsRefreshFunc — указатель на функцию refresh, регистрируется из main.go
// (логика живёт в api package, чтобы не делать circular import services↔api).
//
// Сигнатура: func(companyID uint, db *gorm.DB) ([]byte, error)
// Возвращаемый payload игнорируется scheduler'ом — он просто кэшируется в Redis в самой функции.
type WialonAllAccountsRefreshFunc func(companyID uint) error

// WialonAllAccountsScheduler — cron @every N мин, дёргает refresh для каждой active company.
// Цель: всегда держать Redis cache /api/wialon/all-accounts свежим, чтобы F5 у пользователя
// всегда был cache-hit (~50ms) вместо live fetch (~18s).
type WialonAllAccountsScheduler struct {
	cron      *cron.Cron
	interval  time.Duration
	refresh   WialonAllAccountsRefreshFunc
	isRunning bool
	entryID   cron.EntryID
	name      string     // для логов: "WialonAllAccountsScheduler" / "WialonAllUnitsScheduler"
	runMu     sync.Mutex // guard от пересечения первичного background-refresh и cron-тика
}

func NewWialonAllAccountsScheduler(intervalMinutes int, refresh WialonAllAccountsRefreshFunc) *WialonAllAccountsScheduler {
	return NewWialonRefreshScheduler("WialonAllAccountsScheduler", intervalMinutes, refresh)
}

// NewWialonRefreshScheduler — generic-планировщик прогрева Redis-кэша Wialon.
// name только для логов; логика идентична (итерирует active companies, дёргает refresh).
func NewWialonRefreshScheduler(name string, intervalMinutes int, refresh WialonAllAccountsRefreshFunc) *WialonAllAccountsScheduler {
	if intervalMinutes <= 0 {
		intervalMinutes = 5
	}
	if name == "" {
		name = "WialonRefreshScheduler"
	}
	return &WialonAllAccountsScheduler{
		cron: cron.New(
			cron.WithLocation(time.UTC),
			// SkipIfStillRunning: если предыдущий refresh ещё идёт (холодный live-fetch
			// может занять минуты), пропускаем тик вместо наложения (Codex: overlap →
			// удвоение нагрузки на Wialon rate-limit).
			cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger), cron.Recover(cron.DefaultLogger)),
		),
		interval: time.Duration(intervalMinutes) * time.Minute,
		refresh:  refresh,
		name:     name,
	}
}

func (s *WialonAllAccountsScheduler) Start() error {
	cronExpr := "@every " + s.interval.String()
	id, err := s.cron.AddFunc(cronExpr, func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в %s: %v", s.name, r)
			}
		}()
		s.refreshAll()
	})
	if err != nil {
		return err
	}
	s.entryID = id

	// Первичный refresh при старте (background, не блокируем)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в первичном %s: %v", s.name, r)
			}
		}()
		// Стартовый сдвиг (anti-stampede): разносим первичные прогоны schedulers,
		// чтобы после рестарта не бить один Wialon-токен одновременно. См. wialon_limiter.go.
		time.Sleep(wialonStartupDelay(s.name))
		log.Printf("🚀 %s: первичный refresh (background)", s.name)
		s.refreshAll()
	}()

	s.cron.Start()
	s.isRunning = true
	log.Printf("✅ %s запущен (интервал %s)", s.name, s.interval)
	return nil
}

func (s *WialonAllAccountsScheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
		s.isRunning = false
		log.Printf("🛑 %s остановлен", s.name)
	}
}

func (s *WialonAllAccountsScheduler) IsRunning() bool { return s.isRunning }

// refreshAll итерирует все active companies, дёргает refresh-callback для каждой.
// Сериализует — чтобы не упереться в Wialon rate limit (5 concurrent sessions per token).
func (s *WialonAllAccountsScheduler) refreshAll() {
	if s.refresh == nil {
		log.Printf("⚠️ %s: refresh callback не зарегистрирован", s.name)
		return
	}

	// Anti-overlap: первичный background-refresh и cron-тик не должны идти параллельно
	// (cron.SkipIfStillRunning покрывает только cron-тики между собой).
	if !s.runMu.TryLock() {
		log.Printf("ℹ️ %s: предыдущий refresh ещё идёт — пропускаем", s.name)
		return
	}
	defer s.runMu.Unlock()

	t0 := time.Now()
	// Берём уникальные company_id из активных wialon_connections — компании без подключений не нужны
	var companyIDs []uint
	if err := database.DB.Model(&models.WialonConnection{}).
		Where("is_active = ?", true).
		Distinct("company_id").
		Pluck("company_id", &companyIDs).Error; err != nil {
		log.Printf("⚠️ %s: ошибка получения company_ids: %v", s.name, err)
		return
	}
	if len(companyIDs) == 0 {
		log.Printf("ℹ️ %s: нет компаний с активными wialon connections", s.name)
		return
	}

	ok, fail := 0, 0
	for _, cid := range companyIDs {
		if err := s.refresh(cid); err != nil {
			log.Printf("⚠️ %s: refresh company=%d: %v", s.name, cid, err)
			fail++
			continue
		}
		ok++
	}
	log.Printf("✅ %s: %d/%d ok, fail=%d, за %s", s.name, ok, len(companyIDs), fail, time.Since(t0))
}
