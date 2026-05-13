package core

import (
	"sync"
	"sync/atomic"

	corert "github.com/rcarmo/go-joker/core/runtime"
)

// goroutine_rt.go — Per-goroutine runtime state, replacing the GIL.
//
// Each goroutine gets its own callstack and currentExpr for error reporting.
// The main goroutine uses a fast path (zero overhead when no spawned goroutines).
// Spawned goroutines use a sync.Map keyed by goroutine ID.
//
// With the GIL removed:
// - Immutable data structures (vectors, maps, lists, seqs) are already thread-safe.
// - Atoms use a per-atom mutex for swap!/reset!/compare-and-set! correctness.
// - Var.Value writes (def, alter-var-root) are rare and only safe from the main
//   goroutine or under user coordination (same semantics as Clojure's JVM runtime).
// - Namespace map mutations are protected by nsRWMu.

// goroutineRT holds per-goroutine interpreter state.
type goroutineRT struct {
	callstack   *Callstack
	currentExpr Expr
}

var (
	// mainRT is the default runtime for the main goroutine (hot path).
	mainRT = goroutineRT{
		callstack: &Callstack{frames: make([]Frame, 0, 50)},
	}

	// goroutineRTMap stores per-goroutine state for spawned goroutines.
	goroutineRTMap sync.Map // int64 -> *goroutineRT

	// numSpawnedGoroutines tracks how many go-spawned goroutines are active.
	// When 0, currentGRT() skips the map entirely (single atomic load).
	numSpawnedGoroutines atomic.Int64

	// mainGoroutineID is captured at init.
	mainGoroutineID int64

	// nsRWMu protects GLOBAL_ENV.Namespaces map mutations.
	nsRWMu sync.RWMutex
)

func init() {
	mainGoroutineID = corert.GoID()
}

// currentGRT returns the goroutineRT for the current goroutine.
// HOT PATH: When no spawned goroutines exist (the common case for
// single-threaded execution), returns &mainRT with a single atomic load.
func currentGRT() *goroutineRT {
	if numSpawnedGoroutines.Load() == 0 {
		return &mainRT
	}
	// Spawned goroutines are active — determine which goroutine we are.
	id := corert.GoID()
	if id == mainGoroutineID {
		return &mainRT
	}
	if v, ok := goroutineRTMap.Load(id); ok {
		return v.(*goroutineRT)
	}
	// Fallback (shouldn't happen with proper register/unregister).
	return &mainRT
}

// registerGoroutineRT sets up a new goroutineRT for the current goroutine.
// Called once at goroutine start.
func registerGoroutineRT() *goroutineRT {
	grt := &goroutineRT{
		callstack: &Callstack{frames: make([]Frame, 0, 20)},
	}
	goroutineRTMap.Store(corert.GoID(), grt)
	numSpawnedGoroutines.Add(1)
	return grt
}

// unregisterGoroutineRT removes the current goroutine's state.
// Called once at goroutine exit.
func unregisterGoroutineRT() {
	goroutineRTMap.Delete(corert.GoID())
	numSpawnedGoroutines.Add(-1)
}
