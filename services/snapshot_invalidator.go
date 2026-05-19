package services

import (
	"log"
	"sync"
	"time"
)

// targetSystem — тип системы, snapshot которой нужно резинкнуть.
// Каждая система имеет собственный sync-механизм:
//   - axenta:  AxentaSyncService.SyncAdmin(adminAccountID)
//   - wialon:  WialonStatsService.CollectForConnectionID(connectionID)
//
// Wialon Hosting и Wialon Local оба попадают в "wialon" — разница только в host'е
// у конкретного WialonConnection, sync-логика одна и та же.
type targetSystem string

const (
	systemAxenta targetSystem = "axenta"
	systemWialon targetSystem = "wialon"
)

type targetKey struct {
	system targetSystem
	id     uint
}

// SnapshotInvalidator — асинхронный re-sync менеджер snapshot'ов всех 3 систем
// (Axenta, Wialon Hosting, Wialon Local).
//
// Зачем: при создании/обновлении/удалении user/account/object snapshot-таблицы
// (axenta_account_snapshots, axenta_user_snapshots, wialon_object_stats,
// wialon_users, wialon_units) не успевают обновиться до следующего cron.
// Cross-section flow ломается: создал юзера в Wialon — на /unified/users
// он появится только через 10 минут.
//
// Решение: API-handler после успешной mutation вызывает InvalidateAxenta
// или InvalidateWialon. Воркер с debounce собирает несколько мутаций в один
// resync (если 5 юзеров создали подряд — один CollectForConnection, не пять).
type SnapshotInvalidator struct {
	axentaSvc *AxentaSyncService
	wialonSvc *WialonStatsService

	pending  map[targetKey]time.Time
	mu       sync.Mutex
	debounce time.Duration
	tickRate time.Duration
	quit     chan struct{}
}

var globalSnapshotInvalidator *SnapshotInvalidator

// InitSnapshotInvalidator создаёт глобальный экземпляр и запускает воркер.
// Вызывается один раз из main.go после инициализации Axenta + Wialon sync сервисов.
func InitSnapshotInvalidator(axentaSvc *AxentaSyncService, wialonSvc *WialonStatsService) *SnapshotInvalidator {
	if globalSnapshotInvalidator != nil {
		return globalSnapshotInvalidator
	}
	inv := &SnapshotInvalidator{
		axentaSvc: axentaSvc,
		wialonSvc: wialonSvc,
		pending:   make(map[targetKey]time.Time),
		debounce:  5 * time.Second,
		tickRate:  2 * time.Second,
		quit:      make(chan struct{}),
	}
	go inv.worker()
	globalSnapshotInvalidator = inv
	log.Printf("🔧 SnapshotInvalidator: запущен для всех систем (axenta+wialon, debounce=%v)", inv.debounce)
	return inv
}

// GetSnapshotInvalidator возвращает глобальный экземпляр (может быть nil если не инициализирован).
func GetSnapshotInvalidator() *SnapshotInvalidator {
	return globalSnapshotInvalidator
}

// InvalidateAxenta помечает Axenta-admin'а как требующего re-sync.
// Триггер: CreateUserInAxentaCloud, CreateAccount, MoveAccount, etc.
//
// Ф3-D долг #2: adminAccountID = доверенный company.ID. Воркер зовёт
// SyncAdmin(id) → getActiveTokenForAdmin(id) по user_tokens.account_id;
// после Ф1 user_tokens пуст → no-op (cron-backed, не регрессия — основной
// путь мутаций Ф3-C = синхронный RefreshAccount на server-токене).
// См. middleware.GetTrustedAdminAccountID.
func (s *SnapshotInvalidator) InvalidateAxenta(adminAccountID uint, reason string) {
	if s == nil || adminAccountID == 0 {
		return
	}
	s.queue(targetKey{system: systemAxenta, id: adminAccountID}, reason)
}

// InvalidateWialon помечает Wialon-connection как требующее re-collect stats.
// Триггер: CreateWialonAccount, CreateWialonUser, UpdateWialonAccount, etc.
// Один connectionID покрывает оба варианта (WH/WL) — разница в host'е.
func (s *SnapshotInvalidator) InvalidateWialon(connectionID uint, reason string) {
	if s == nil || connectionID == 0 {
		return
	}
	s.queue(targetKey{system: systemWialon, id: connectionID}, reason)
}

// Invalidate (legacy) — синоним InvalidateAxenta для обратной совместимости.
func (s *SnapshotInvalidator) Invalidate(adminAccountID uint, reason string) {
	s.InvalidateAxenta(adminAccountID, reason)
}

func (s *SnapshotInvalidator) queue(key targetKey, reason string) {
	s.mu.Lock()
	s.pending[key] = time.Now()
	s.mu.Unlock()
	log.Printf("📥 SnapshotInvalidator: %s=%d reason=%s queued", key.system, key.id, reason)
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
	var ready []targetKey
	for k, ts := range s.pending {
		if now.Sub(ts) >= s.debounce {
			ready = append(ready, k)
			delete(s.pending, k)
		}
	}
	s.mu.Unlock()

	for _, k := range ready {
		s.runResync(k)
	}
}

func (s *SnapshotInvalidator) runResync(k targetKey) {
	start := time.Now()
	switch k.system {
	case systemAxenta:
		if s.axentaSvc == nil {
			log.Printf("⚠️ SnapshotInvalidator: axenta admin=%d пропущен (syncSvc=nil)", k.id)
			return
		}
		log.Printf("🔄 SnapshotInvalidator: axenta resync admin=%d", k.id)
		if err := s.axentaSvc.SyncAdmin(k.id); err != nil {
			log.Printf("❌ SnapshotInvalidator: axenta admin=%d failed (%v): %v", k.id, time.Since(start), err)
		} else {
			log.Printf("✅ SnapshotInvalidator: axenta admin=%d done за %v", k.id, time.Since(start).Round(time.Second))
		}
	case systemWialon:
		if s.wialonSvc == nil {
			log.Printf("⚠️ SnapshotInvalidator: wialon conn=%d пропущен (wialonSvc=nil)", k.id)
			return
		}
		log.Printf("🔄 SnapshotInvalidator: wialon collect conn=%d", k.id)
		count, err := s.wialonSvc.CollectForConnectionID(k.id)
		if err != nil {
			log.Printf("❌ SnapshotInvalidator: wialon conn=%d failed (%v): %v", k.id, time.Since(start), err)
		} else {
			log.Printf("✅ SnapshotInvalidator: wialon conn=%d done за %v (%d ресурсов)", k.id, time.Since(start).Round(time.Second), count)
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
