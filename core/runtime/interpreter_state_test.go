package runtime

import "testing"

func TestGoroutineRTCloneCopiesStackAndCurrentExpr(t *testing.T) {
	state := NewGoroutineRT(1)
	state.CurrentExpr = "expr"
	state.Callstack.Push(testTraceable{name: "f"})
	clone := state.Clone()
	state.CurrentExpr = "other"
	state.Callstack.Pop()
	if clone.CurrentExpr != "expr" {
		t.Fatalf("clone CurrentExpr = %v, want expr", clone.CurrentExpr)
	}
	if clone.Callstack.Len() != 1 {
		t.Fatalf("clone Callstack Len = %d, want 1", clone.Callstack.Len())
	}
}

func TestInterpreterStatePoolCurrentRegisterUnregister(t *testing.T) {
	pool := NewInterpreterStatePool(NewGoroutineRT(2))
	if pool.Current() == nil {
		t.Fatal("Current returned nil")
	}
	registered := pool.Register(3)
	if registered == nil || registered.Callstack == nil {
		t.Fatalf("Register returned invalid state: %#v", registered)
	}
	pool.Unregister()
}
