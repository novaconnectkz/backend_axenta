package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"log"
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
}

func NewWialonAllAccountsScheduler(intervalMinutes int, refresh WialonAllAccountsRefreshFunc) *WialonAllAccountsScheduler {
	if intervalMinutes <= 0 {
		intervalMinutes = 5
	}
	return &WialonAllAccountsScheduler{
		cron: cron.New(
			cron.WithLocation(time.UTC),
			cron.WithChain(cron.Recover(cron.DefaultLogger)),
		),
		interval: time.Duration(intervalMinutes) * time.Minute,
		refresh:  refresh,
	}
}

func (s *WialonAllAccountsScheduler) Start() error {
	cronExpr := "@every " + s.interval.String()
	id, err := s.cron.AddFunc(cronExpr, func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в WialonAllAccountsScheduler: %v", r)
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
				log.Printf("❌ ПАНИКА в первичном WialonAllAccountsScheduler: %v", r)
			}
		}()
		time.Sleep(7 * time.Second) // ждём пока поднимутся остальные сервисы
		log.Printf("🚀 WialonAllAccountsScheduler: первичный refresh (background)")
		s.refreshAll()
	}()

	s.cron.Start()
	s.isRunning = true
	log.Printf("✅ WialonAllAccountsScheduler запущен (интервал %s)", s.interval)
	return nil
}

func (s *WialonAllAccountsScheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
		s.isRunning = false
		log.Printf("🛑 WialonAllAccountsScheduler остановлен")
	}
}

func (s *WialonAllAccountsScheduler) IsRunning() bool { return s.isRunning }

// refreshAll итерирует все active companies, дёргает refresh-callback для каждой.
// Сериализует — чтобы не упереться в Wialon rate limit (5 concurrent sessions per token).
func (s *WialonAllAccountsScheduler) refreshAll() {
	if s.refresh == nil {
		log.Printf("⚠️ WialonAllAccountsScheduler: refresh callback не зарегистрирован")
		return
	}

	t0 := time.Now()
	// Берём уникальные company_id из активных wialon_connections — компании без подключений не нужны
	var companyIDs []uint
	if err := database.DB.Model(&models.WialonConnection{}).
		Where("is_active = ?", true).
		Distinct("company_id").
		Pluck("company_id", &companyIDs).Error; err != nil {
		log.Printf("⚠️ WialonAllAccountsScheduler: ошибка получения company_ids: %v", err)
		return
	}
	if len(companyIDs) == 0 {
		log.Printf("ℹ️ WialonAllAccountsScheduler: нет компаний с активными wialon connections")
		return
	}

	ok, fail := 0, 0
	for _, cid := range companyIDs {
		if err := s.refresh(cid); err != nil {
			log.Printf("⚠️ WialonAllAccountsScheduler: refresh company=%d: %v", cid, err)
			fail++
			continue
		}
		ok++
	}
	log.Printf("✅ WialonAllAccountsScheduler: %d/%d ok, fail=%d, за %s", ok, len(companyIDs), fail, time.Since(t0))
}
