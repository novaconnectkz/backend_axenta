package services

import (
	"log"
	"sync"

	"github.com/robfig/cron/v3"
)

// AxentaSyncScheduler планирует периодическую синхронизацию данных Axenta
type AxentaSyncScheduler struct {
	syncService *AxentaSyncService
	cron        *cron.Cron
	mu          sync.Mutex
	running     bool
}

// NewAxentaSyncScheduler создает новый планировщик синхронизации Axenta
func NewAxentaSyncScheduler(syncService *AxentaSyncService) *AxentaSyncScheduler {
	return &AxentaSyncScheduler{
		syncService: syncService,
		cron:        cron.New(cron.WithSeconds()),
	}
}

// Start запускает планировщик (синхронизация каждую минуту)
func (s *AxentaSyncScheduler) Start() error {
	_, err := s.cron.AddFunc("0 * * * * *", func() {
		s.runSyncAll()
	})
	if err != nil {
		return err
	}

	s.cron.Start()
	log.Println("AxentaSync: планировщик синхронизации запущен (каждую минуту)")
	return nil
}

// Stop останавливает планировщик
func (s *AxentaSyncScheduler) Stop() {
	s.cron.Stop()
	log.Println("AxentaSync: планировщик синхронизации остановлен")
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
