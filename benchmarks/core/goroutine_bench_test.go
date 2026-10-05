package core

import (
	corert "github.com/rcarmo/go-joker/v42/core/runtime"
	"testing"
)

func BenchmarkGoID(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if corert.GoID() <= 0 {
			b.Fatal("missing goroutine ID")
		}
	}
}

func BenchmarkRegisteredInterpreterCurrent(b *testing.B) {
	// Create the pool on a different goroutine: registering the pool's main
	// goroutine deliberately leaves Current() selecting the main state.
	ready := make(chan *corert.InterpreterStatePool, 1)
	go func() { ready <- corert.NewInterpreterStatePool(corert.NewGoroutineRT(1)) }()
	pool := <-ready
	want := pool.Register(1)
	defer pool.Unregister()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if pool.Current() != want {
			b.Fatal("wrong runtime state")
		}
	}
}
