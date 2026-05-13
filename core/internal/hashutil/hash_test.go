package hashutil

import "testing"

func TestPtrHashIsStableForSameInput(t *testing.T) {
	const ptr = uintptr(0x1234)
	if a, b := Ptr(ptr), Ptr(ptr); a != b {
		t.Fatalf("Ptr hash mismatch: %d != %d", a, b)
	}
}
