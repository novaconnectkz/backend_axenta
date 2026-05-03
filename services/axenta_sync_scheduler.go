package services

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// AxentaSyncScheduler планирует периодическую синхронизацию данных Axenta
// Автоматическая синхронизация запускается каждую минуту
// Ручной запуск доступен через API endpoint /api/auth/axenta-sync/trigger
type AxentaSyncScheduler struct {
	syncService *AxentaSyncService
	cron        *cron.Cron
	mu          sync.Mutex
	running     bool
	interval    int // Интервал синхронизации в минутах (используется только для совместимости API)
}

// NewAxentaSyncScheduler создает новый планировщик синхронизации Axenta
// intervalMinutes - параметр для обратной совместимости, не используется для автоматического запуска
// Автоматическая синхронизация всегда запускается каждую минуту
func NewAxentaSyncScheduler(syncService *AxentaSyncService, intervalMinutes int) *AxentaSyncScheduler {
	// Сохраняем interval для совместимости с API, но не используем для автоматического запуска
	if intervalMinutes <= 0 {
		intervalMinutes = 1440 // 24 часа в минутах (для совместимости API)
	}
	return &AxentaSyncScheduler{
		syncService: syncService,
		cron:        cron.New(cron.WithSeconds()),
		interval:    intervalMinutes,
	}
}

// Start запускает планировщик для автоматической синхронизации
// Интервал берётся из s.interval (минуты), управляется через ENV AXENTA_SYNC_INTERVAL_MIN
// Для ручного запуска используйте API endpoint /api/auth/axenta-sync/trigger
func (s *AxentaSyncScheduler) Start() error {
	intervalMin := s.interval
	if intervalMin <= 0 {
		intervalMin = 10
	}
	cronExpr := fmt.Sprintf("@every %dm", intervalMin)

	_, err := s.cron.AddFunc(cronExpr, func() {
		s.runSyncAll()
	})
	if err != nil {
		return fmt.Errorf("ошибка добавления cron задачи: %w", err)
	}

	// Первый sync через 5 секунд после старта приложения, чтобы snapshot не был пустым на холодном старте
	go func() {
		time.Sleep(5 * time.Second)
		log.Println("⏰ AxentaSync: запуск первичной синхронизации после старта приложения")
		s.runSyncAll()
	}()

	s.cron.Start()
	log.Printf("⏰ AxentaSync: планировщик автоматической синхронизации запущен")
	log.Printf("   📅 Расписание: каждые %d мин", intervalMin)
	log.Printf("   🔧 Cron выражение: %s", cronExpr)
	log.Printf("   🔄 Ручной запуск: POST /api/auth/axenta-sync/trigger")
	return nil
}

// Stop останавливает планировщик
func (s *AxentaSyncScheduler) Stop() {
	s.cron.Stop()
	log.Println("AxentaSync: планировщик синхронизации остановлен")
}

// UpdateInterval обновляет интервал синхронизации (для совместимости API)
// ВНИМАНИЕ: Автоматическая синхронизация всегда запускается каждую минуту
// Этот метод только обновляет значение interval для совместимости с API, но не меняет расписание
func (s *AxentaSyncScheduler) UpdateInterval(newIntervalMinutes int) error {
	if newIntervalMinutes <= 0 {
		return fmt.Errorf("интервал должен быть больше 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Обновляем только значение interval для совместимости с API
	s.interval = newIntervalMinutes

	log.Printf("⏰ AxentaSync: значение интервала обновлено для совместимости API")
	log.Printf("   📊 Интервал (только для API): %d минут", s.interval)
	log.Printf("   ℹ️  Автоматическая синхронизация продолжает работать по расписанию: каждую минуту")
	log.Printf("   🔄 Для ручного запуска используйте: POST /api/auth/axenta-sync/trigger")

	return nil
}

// GetInterval возвращает текущий интервал синхронизации (для совместимости API)
// ВНИМАНИЕ: Автоматическая синхронизация всегда запускается каждую минуту
// Это значение используется только для совместимости с API
func (s *AxentaSyncScheduler) GetInterval() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interval
}

// SyncAdminAsync выполняет синхронизацию для администратора в отдельной горутине
func (s *AxentaSyncScheduler) SyncAdminAsync(adminAccountID uint) {
	go func() {
		if err := s.syncService.SyncAdmin(adminAccountID); err != nil {
			log.Printf("AxentaSync: ошибка синхронизации admin_account_id=%d: %v", adminAccountID, err)
		}
	}()
}

func (s *AxentaSyncScheduler) runSyncAll() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	log.Println("AxentaSync: начало плановой синхронизации")
	s.syncService.SyncAllAdmins()
	log.Println("AxentaSync: завершение плановой синхронизации")
}
