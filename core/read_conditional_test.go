package core

import (
	"strings"
	"testing"
)

func TestReadConditionalSpliceEmptyInList(t *testing.T) {
	reader := NewReader(strings.NewReader("(do #?@(:definitely-nope [1 2]) 3)"), "<test>")
	obj, err := TryRead(reader)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	lst, ok := obj.(*List)
	if !ok {
		t.Fatalf("expected list, got %T", obj)
	}
	if got := lst.Count(); got != 2 {
		t.Fatalf("expected 2 elements (do, 3), got %d", got)
	}
}

func TestReadConditionalNestedSpliceNoRuntimePanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected runtime panic: %v", r)
		}
	}()

	reader := NewReader(strings.NewReader("#?(:x #?@(:x [1 2]))"), "<test>")
	_, err := TryRead(reader)
	if err == nil {
		t.Fatal("expected read error for invalid nested splice")
	}
	if !strings.Contains(err.Error(), "Read error") {
		t.Fatalf("expected read error, got: %v", err)
	}
}
