package services

import (
	"fmt"
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
	interval    int // Интервал синхронизации в минутах
}

// NewAxentaSyncScheduler создает новый планировщик синхронизации Axenta
func NewAxentaSyncScheduler(syncService *AxentaSyncService, intervalMinutes int) *AxentaSyncScheduler {
	if intervalMinutes <= 0 {
		intervalMinutes = 5 // Значение по умолчанию: 5 минут
	}
	return &AxentaSyncScheduler{
		syncService: syncService,
		cron:        cron.New(cron.WithSeconds()),
		interval:    intervalMinutes,
	}
}

// Start запускает планировщик с настраиваемым интервалом
func (s *AxentaSyncScheduler) Start() error {
	// Формируем cron выражение на основе интервала в минутах
	// Формат: "0 */N * * * *" означает "в секунду 0 каждой N-й минуты"
	cronExpr := fmt.Sprintf("0 */%d * * * *", s.interval)

	_, err := s.cron.AddFunc(cronExpr, func() {
		s.runSyncAll()
	})
	if err != nil {
		return fmt.Errorf("ошибка добавления cron задачи: %w", err)
	}

	s.cron.Start()
	log.Printf("AxentaSync: планировщик синхронизации запущен (каждые %d минут)", s.interval)
	return nil
}

// Stop останавливает планировщик
func (s *AxentaSyncScheduler) Stop() {
	s.cron.Stop()
	log.Println("AxentaSync: планировщик синхронизации остановлен")
}

// UpdateInterval обновляет интервал синхронизации
func (s *AxentaSyncScheduler) UpdateInterval(newIntervalMinutes int) error {
	if newIntervalMinutes <= 0 {
		return fmt.Errorf("интервал должен быть больше 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Останавливаем текущий планировщик
	s.cron.Stop()

	// Обновляем интервал
	s.interval = newIntervalMinutes

	// Создаем новый планировщик
	s.cron = cron.New(cron.WithSeconds())

	// Формируем новое cron выражение
	cronExpr := fmt.Sprintf("0 */%d * * * *", s.interval)
	
	_, err := s.cron.AddFunc(cronExpr, func() {
		s.runSyncAll()
	})
	if err != nil {
		return fmt.Errorf("ошибка добавления cron задачи: %w", err)
	}

	// Запускаем планировщик
	s.cron.Start()
	log.Printf("AxentaSync: интервал синхронизации обновлен на %d минут", s.interval)

	return nil
}

// GetInterval возвращает текущий интервал синхронизации
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
