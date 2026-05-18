package string

import "testing"

func TestHashIsStableForSameInput(t *testing.T) {
	if a, b := Hash("abcdef"), Hash("abcdef"); a != b {
		t.Fatalf("Hash mismatch: %d != %d", a, b)
	}
}
