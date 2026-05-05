package services

import (
	"log"
	"sync"
	"time"
)

// SnapshotInvalidator — асинхронный re-sync менеджер snapshot'ов Axenta.
//
// Зачем: при создании/обновлении/удалении user/account/object в Axenta Cloud
// локальные snapshot-таблицы (axenta_account_snapshots, axenta_user_snapshots,
// wialon_object_stats) не успевают обновиться до следующего scheduled cron
// (10 мин). Cross-section flow ломается: создал юзера — объекты не видят
// его как creator до next sync.
//
// Решение: API-handler после успешной mutation вызывает Invalidate(adminID).
// Воркер с debounce собирает несколько мутаций в один resync (если 5 юзеров
// создали подряд — один SyncAdmin, не пять).
type SnapshotInvalidator struct {
	syncSvc  *AxentaSyncService
	pending  map[uint]time.Time
	mu       sync.Mutex
	debounce time.Duration
	tickRate time.Duration
	quit     chan struct{}
}

var globalSnapshotInvalidator *SnapshotInvalidator

// InitSnapshotInvalidator создаёт глобальный экземпляр и запускает воркер.
// Вызывается один раз из main.go после инициализации AxentaSyncService.
func InitSnapshotInvalidator(syncSvc *AxentaSyncService) *SnapshotInvalidator {
	if globalSnapshotInvalidator != nil {
		return globalSnapshotInvalidator
	}
	inv := &SnapshotInvalidator{
		syncSvc:  syncSvc,
		pending:  make(map[uint]time.Time),
		debounce: 5 * time.Second,
		tickRate: 2 * time.Second,
		quit:     make(chan struct{}),
	}
	go inv.worker()
	globalSnapshotInvalidator = inv
	log.Printf("🔧 SnapshotInvalidator: запущен (debounce=%v)", inv.debounce)
	return inv
}

// GetSnapshotInvalidator возвращает глобальный экземпляр (может быть nil если не инициализирован).
func GetSnapshotInvalidator() *SnapshotInvalidator {
	return globalSnapshotInvalidator
}

// Invalidate помечает admin'а как требующего re-sync. Безопасно вызывать на nil-инстансе.
// reason — для логов: "user.create", "object.delete" и т.п.
func (s *SnapshotInvalidator) Invalidate(adminAccountID uint, reason string) {
	if s == nil || adminAccountID == 0 {
		return
	}
	s.mu.Lock()
	s.pending[adminAccountID] = time.Now()
	s.mu.Unlock()
	log.Printf("📥 SnapshotInvalidator: admin=%d reason=%s queued", adminAccountID, reason)
}

func (s *SnapshotInvalidator) worker() {
	ticker := time.NewTicker(s.tickRate)
	defer ticker.Stop()
	for {
		select {
		case <-s.quit:
			return
		case now := <-ticker.C:
			s.flush(now)
		}
	}
}

func (s *SnapshotInvalidator) flush(now time.Time) {
	s.mu.Lock()
	var ready []uint
	for id, ts := range s.pending {
		if now.Sub(ts) >= s.debounce {
			ready = append(ready, id)
			delete(s.pending, id)
		}
	}
	s.mu.Unlock()

	for _, id := range ready {
		if s.syncSvc == nil {
			log.Printf("⚠️ SnapshotInvalidator: admin=%d пропущен (syncSvc=nil)", id)
			continue
		}
		log.Printf("🔄 SnapshotInvalidator: resync admin=%d", id)
		start := time.Now()
		if err := s.syncSvc.SyncAdmin(id); err != nil {
			log.Printf("❌ SnapshotInvalidator: admin=%d failed (%v): %v", id, time.Since(start), err)
		} else {
			log.Printf("✅ SnapshotInvalidator: admin=%d done за %v", id, time.Since(start).Round(time.Second))
		}
	}
}

// Stop останавливает воркер. Вызывается через defer в main.go.
func (s *SnapshotInvalidator) Stop() {
	if s == nil {
		return
	}
	close(s.quit)
}
