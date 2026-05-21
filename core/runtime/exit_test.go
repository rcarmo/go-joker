package runtime

import "testing"

func TestOnExitRegistersCallback(t *testing.T) {
	old := exitCallbacks
	defer func() { exitCallbacks = old }()
	exitCallbacks = nil
	called := false
	OnExit(func() { called = true })
	if len(exitCallbacks) != 1 {
		t.Fatalf("exitCallbacks len = %d, want 1", len(exitCallbacks))
	}
	exitCallbacks[0]()
	if !called {
		t.Fatal("registered exit callback was not called")
	}
}
