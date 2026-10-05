package runtime

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestGoIDConcurrentStable(t *testing.T) {
	const workers = 16
	ids := make(chan int64, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := GoID()
			for range 100 {
				if got := GoID(); got != id || got <= 0 {
					t.Errorf("goroutine identity changed: %d -> %d", id, got)
				}
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	seen := make(map[int64]bool)
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate goroutine id %d", id)
		}
		seen[id] = true
	}
}

func TestGoIDIsPositive(t *testing.T) {
	if id := GoID(); id <= 0 {
		t.Fatalf("GoID() = %d, want > 0", id)
	}
}

func TestGoRTPoolCurrentSkipsGoIDWithoutSpawnedGoroutines(t *testing.T) {
	var calls atomic.Int64
	pool := NewGoRTPool(func() int64 {
		calls.Add(1)
		return 1
	}, "main")
	calls.Store(0)

	if got := pool.Current(); got != "main" {
		t.Fatalf("Current() = %v, want main", got)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("goid calls = %d, want 0", got)
	}
}

func TestGoRTPoolCurrentUsesGoIDWhenSpawnedGoroutinesExist(t *testing.T) {
	var calls atomic.Int64
	var id atomic.Int64
	id.Store(1)
	pool := NewGoRTPool(func() int64 {
		calls.Add(1)
		return id.Load()
	}, "main")

	id.Store(2)
	pool.Register("worker")
	calls.Store(0)

	if got := pool.Current(); got != "worker" {
		t.Fatalf("Current() = %v, want worker", got)
	}
	if got := calls.Load(); got == 0 {
		t.Fatal("expected Current() to consult goid when spawned goroutines exist")
	}
}
