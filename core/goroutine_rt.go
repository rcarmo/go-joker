package core

import (
	"sync"

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
	goroutineState *corert.GoRTPool

	// nsRWMu protects GLOBAL_ENV.Namespaces map mutations.
	nsRWMu sync.RWMutex
)

func init() {
	goroutineState = corert.NewGoRTPool(corert.GoID, &mainRT)
}

// currentGRT returns the goroutineRT for the current goroutine.
// HOT PATH: When no spawned goroutines exist (the common case for
// single-threaded execution), returns &mainRT with a single atomic load.
func currentGRT() *goroutineRT {
	return goroutineState.Current().(*goroutineRT)
}

// registerGoroutineRT sets up a new goroutineRT for the current goroutine.
// Called once at goroutine start.
func registerGoroutineRT() *goroutineRT {
	grt := &goroutineRT{callstack: &Callstack{frames: make([]Frame, 0, 20)}}
	goroutineState.Register(grt)
	return grt
}

// unregisterGoroutineRT removes the current goroutine's state.
// Called once at goroutine exit.
func unregisterGoroutineRT() {
	goroutineState.Unregister()
}
