package services

import (
	"sync"
	"testing"
	"time"
)

// TestSnapshotInvalidator_Debounce проверяет, что несколько последовательных
// invalidate для одного admin'а схлопываются в один resync.
func TestSnapshotInvalidator_Debounce(t *testing.T) {
	inv := &SnapshotInvalidator{
		pending:  make(map[targetKey]time.Time),
		debounce: 50 * time.Millisecond,
		tickRate: 10 * time.Millisecond,
		quit:     make(chan struct{}),
	}

	// Симулируем 5 быстрых invalidations для одного axenta-admin
	for i := 0; i < 5; i++ {
		inv.InvalidateAxenta(42, "test")
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

// TestSnapshotInvalidator_DifferentSystems проверяет, что Axenta и Wialon
// с одинаковым ID — это разные ключи, не схлопываются.
func TestSnapshotInvalidator_DifferentSystems(t *testing.T) {
	inv := &SnapshotInvalidator{
		pending:  make(map[targetKey]time.Time),
		debounce: 50 * time.Millisecond,
		tickRate: 10 * time.Millisecond,
		quit:     make(chan struct{}),
	}

	inv.InvalidateAxenta(7, "a")
	inv.InvalidateWialon(7, "w")

	inv.mu.Lock()
	count := len(inv.pending)
	inv.mu.Unlock()
	if count != 2 {
		t.Fatalf("ожидали 2 записи (axenta+wialon), получили %d", count)
	}
}

// TestSnapshotInvalidator_NilSafe проверяет, что invalidate на nil-инстансе не паникует.
func TestSnapshotInvalidator_NilSafe(t *testing.T) {
	var inv *SnapshotInvalidator
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("invalidate на nil вызвал панику: %v", r)
		}
	}()
	inv.InvalidateAxenta(1, "test")
	inv.InvalidateWialon(1, "test")
	inv.Stop()
}

// TestSnapshotInvalidator_ZeroID проверяет, что ID=0 игнорируется.
func TestSnapshotInvalidator_ZeroID(t *testing.T) {
	inv := &SnapshotInvalidator{
		pending: make(map[targetKey]time.Time),
		mu:      sync.Mutex{},
	}
	inv.InvalidateAxenta(0, "noop")
	inv.InvalidateWialon(0, "noop")
	if len(inv.pending) != 0 {
		t.Fatalf("ID=0 не должен попадать в pending")
	}
}
