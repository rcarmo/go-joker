package runtime

import (
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

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

func TestObjectFutureDerefAndRealized(t *testing.T) {
	f := NewObjectFuture()
	if f.IsRealized() {
		t.Fatal("new future is realized")
	}
	value := coretypes.Int{I: 7}
	f.Complete(value, nil)
	if !f.IsRealized() {
		t.Fatal("completed future is not realized")
	}
	if got := f.Deref(); !got.Equals(value) {
		t.Fatalf("future deref = %s, want 7", got.ToString(false))
	}
}

func TestObjectFutureDerefPanicsOnError(t *testing.T) {
	f := NewObjectFuture()
	sentinel := channelObjectTestError("future boom")
	f.Complete(nil, sentinel)
	defer func() {
		if r := recover(); r != sentinel {
			t.Fatalf("future panic = %v, want sentinel", r)
		}
	}()
	_ = f.Deref()
}

func TestObjectPromiseDeliverOnce(t *testing.T) {
	p := NewObjectPromise()
	if p.IsRealized() {
		t.Fatal("new promise is realized")
	}
	first := coretypes.Int{I: 1}
	second := coretypes.Int{I: 2}
	if !p.Deliver(first) {
		t.Fatal("first deliver failed")
	}
	if p.Deliver(second) {
		t.Fatal("second deliver succeeded")
	}
	if !p.IsRealized() {
		t.Fatal("delivered promise is not realized")
	}
	if got := p.Deref(); !got.Equals(first) {
		t.Fatalf("promise deref = %s, want first value", got.ToString(false))
	}
}
