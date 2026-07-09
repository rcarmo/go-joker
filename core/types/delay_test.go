package types

import (
	"sync"
	"sync/atomic"
	"testing"
)

type delayTestCallable func([]Object) Object

func (f delayTestCallable) Call(args []Object) Object { return f(args) }

func TestDelayForceEvaluatesOnceConcurrently(t *testing.T) {
	oldDelayCall := DelayCall
	DelayCall = func(fn Callable) Object { return fn.Call(nil) }
	defer func() { DelayCall = oldDelayCall }()

	var calls atomic.Int32
	d := NewDelay(delayTestCallable(func([]Object) Object {
		calls.Add(1)
		return Int{I: 42}
	}))
	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if got := d.Force().(Int).I; got != 42 {
				t.Errorf("Force = %d, want 42", got)
			}
		}()
	}
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("delay calls = %d, want 1", got)
	}
	if !d.IsRealized() {
		t.Fatal("delay was not marked realized")
	}
}

func TestDelayRetriesAfterPanic(t *testing.T) {
	oldDelayCall := DelayCall
	DelayCall = func(fn Callable) Object { return fn.Call(nil) }
	defer func() { DelayCall = oldDelayCall }()

	var calls int
	d := NewDelay(delayTestCallable(func([]Object) Object {
		calls++
		if calls == 1 {
			panic("boom")
		}
		return Int{I: 7}
	}))
	func() {
		defer func() {
			if recovered := recover(); recovered != "boom" {
				t.Fatalf("panic = %v, want boom", recovered)
			}
		}()
		d.Force()
	}()
	if d.IsRealized() {
		t.Fatal("panicking delay was marked realized")
	}
	if got := d.Force().(Int).I; got != 7 {
		t.Fatalf("retry Force = %d, want 7", got)
	}
}
