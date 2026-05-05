package services

import (
	"sync"
	"testing"
	"time"
)

// TestSnapshotInvalidator_Debounce проверяет, что несколько последовательных
// Invalidate для одного admin'а схлопываются в один resync.
func TestSnapshotInvalidator_Debounce(t *testing.T) {
	inv := &SnapshotInvalidator{
		syncSvc:  nil,
		pending:  make(map[uint]time.Time),
		debounce: 50 * time.Millisecond,
		tickRate: 10 * time.Millisecond,
		quit:     make(chan struct{}),
	}

	// Симулируем 5 быстрых invalidations
	for i := 0; i < 5; i++ {
		inv.mu.Lock()
		inv.pending[42] = time.Now()
		inv.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}

	// Проверка: pending содержит только одну запись
	inv.mu.Lock()
	count := len(inv.pending)
	inv.mu.Unlock()
	if count != 1 {
		t.Fatalf("ожидали 1 запись в pending после дебаунса, получили %d", count)
	}

	// Проверка: после debounce-периода flush очищает pending
	time.Sleep(60 * time.Millisecond)
	inv.flush(time.Now())

	inv.mu.Lock()
	count = len(inv.pending)
	inv.mu.Unlock()
	if count != 0 {
		t.Fatalf("ожидали 0 в pending после flush, получили %d", count)
	}
}

// TestSnapshotInvalidator_NilSafe проверяет, что Invalidate на nil-инстансе не паникует.
func TestSnapshotInvalidator_NilSafe(t *testing.T) {
	var inv *SnapshotInvalidator
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Invalidate на nil вызвал панику: %v", r)
		}
	}()
	inv.Invalidate(1, "test")
	inv.Stop()
}

// TestSnapshotInvalidator_ZeroAdminID проверяет, что adminID=0 игнорируется.
func TestSnapshotInvalidator_ZeroAdminID(t *testing.T) {
	inv := &SnapshotInvalidator{
		pending: make(map[uint]time.Time),
		mu:      sync.Mutex{},
	}
	inv.Invalidate(0, "noop")
	if len(inv.pending) != 0 {
		t.Fatalf("adminID=0 не должен попадать в pending")
	}
}
