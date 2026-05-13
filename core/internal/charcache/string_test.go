package charcache

import "testing"

func TestString(t *testing.T) {
	if got := String('A'); got != "A" {
		t.Fatalf("String('A') = %q", got)
	}
	if got := String('é'); got != "é" {
		t.Fatalf("String('é') = %q", got)
	}
}
