package runtime

import (
	"sync/atomic"
	"testing"
)

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
