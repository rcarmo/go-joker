package runtime

import "testing"

func TestFutureCompleteAwaitAndRealized(t *testing.T) {
	f := NewFuture[int, string]()
	if f.IsRealized() {
		t.Fatal("new future realized")
	}
	f.Complete(7, "")
	if !f.IsRealized() {
		t.Fatal("completed future not realized")
	}
	v, err := f.Await()
	if v != 7 || err != "" {
		t.Fatalf("Await = (%d, %q)", v, err)
	}
}

func TestPromiseDeliverOnce(t *testing.T) {
	p := NewPromise[int]()
	if p.IsRealized() {
		t.Fatal("new promise realized")
	}
	if !p.Deliver(1) {
		t.Fatal("first deliver failed")
	}
	if p.Deliver(2) {
		t.Fatal("second deliver succeeded")
	}
	if !p.IsRealized() || p.Await() != 1 {
		t.Fatalf("promise value = %d realized=%v", p.Await(), p.IsRealized())
	}
}
