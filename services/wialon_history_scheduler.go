package services

import (
	"log"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"
)

// WialonHistoryScheduler — ночной cron, обновляет wialon_daily_snapshots за вчерашний
// день для всех companies с wialon_history_settings.enabled=true.
//
// Запускается каждый день в 02:00 UTC. Idempotent: если backfill уже шёл за этот
// день для company, перезаписывает (см. backfillConnection — DELETE перед записью).
//
// Включается через ENABLE_WIALON_HISTORY_SCHEDULER=true (не запускается по умолчанию).
type WialonHistoryScheduler struct {
	historyService *WialonHistoryService
	stopCh         chan struct{}
}

// NewWialonHistoryScheduler создаёт scheduler.
func NewWialonHistoryScheduler(hs *WialonHistoryService) *WialonHistoryScheduler {
	return &WialonHistoryScheduler{
		historyService: hs,
		stopCh:         make(chan struct{}),
	}
}

// Start запускает goroutine с ежедневным циклом.
func (s *WialonHistoryScheduler) Start() {
	go s.loop()
	log.Printf("✅ WialonHistoryScheduler: запущен (ежедневно в 02:00 UTC)")
}

// Stop останавливает scheduler.
func (s *WialonHistoryScheduler) Stop() {
	close(s.stopCh)
}

func (s *WialonHistoryScheduler) loop() {
	for {
		nextRun := nextDailyRunUTC(time.Now().UTC(), 2, 0) // 02:00 UTC
		wait := time.Until(nextRun)
		log.Printf("⏰ WialonHistoryScheduler: next run в %s (через %s)",
			nextRun.Format("2006-01-02 15:04:05 UTC"), wait.Round(time.Second))

		select {
		case <-time.After(wait):
			s.runForAllCompanies()
		case <-s.stopCh:
			return
		}
	}
}

// nextDailyRunUTC — следующее время hour:minute UTC после now (или сегодня, или завтра).
func nextDailyRunUTC(now time.Time, hour, minute int) time.Time {
	today := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if today.After(now) {
		return today
	}
	return today.AddDate(0, 0, 1)
}

// runForAllCompanies обновляет вчерашний день для всех companies с enabled=true.
func (s *WialonHistoryScheduler) runForAllCompanies() {
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	day := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)

	var settings []models.WialonHistorySettings
	if err := database.DB.Where("enabled = true").Find(&settings).Error; err != nil {
		log.Printf("❌ WialonHistoryScheduler: get settings: %v", err)
		return
	}

	if len(settings) == 0 {
		log.Printf("⏭️ WialonHistoryScheduler: нет companies с enabled=true, пропуск")
		return
	}

	log.Printf("🔄 WialonHistoryScheduler: backfill за %s для %d companies",
		day.Format("2006-01-02"), len(settings))

	for _, st := range settings {
		// StartBackfill async — но мы хотим подождать. Используем синхронный путь
		// через прямой вызов backfill для всех connections.
		if s.historyService.IsRunning(st.CompanyID) {
			log.Printf("⚠️ WialonHistoryScheduler: company=%d backfill уже идёт, пропуск", st.CompanyID)
			continue
		}

		if err := s.historyService.StartBackfill(st.CompanyID, day, day); err != nil {
			log.Printf("❌ WialonHistoryScheduler: company=%d StartBackfill: %v", st.CompanyID, err)
			continue
		}

		// Ждём окончания (max 5 мин per company)
		s.waitForCompletion(st.CompanyID, 5*time.Minute)
	}

	log.Printf("✅ WialonHistoryScheduler: цикл завершён")
}

func (s *WialonHistoryScheduler) waitForCompletion(companyID uint, maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if !s.historyService.IsRunning(companyID) {
			return
		}
		time.Sleep(2 * time.Second)
	}
	log.Printf("⏰ WialonHistoryScheduler: company=%d backfill не завершился за %s", companyID, maxWait)
}
