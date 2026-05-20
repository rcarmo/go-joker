package string

import "testing"

func TestObjectCache(t *testing.T) {
	cache := NewObjectCache(func(ch rune) string { return String(ch) })
	if got, ok := cache.Lookup('A'); !ok || got != "A" {
		t.Fatalf("Lookup ASCII = %q, %v; want A, true", got, ok)
	}
	if _, ok := cache.Lookup('é'); ok {
		t.Fatal("Lookup accepted non-ASCII rune")
	}
}

func TestString(t *testing.T) {
	if got := String('A'); got != "A" {
		t.Fatalf("String('A') = %q", got)
	}
	if got := String('é'); got != "é" {
		t.Fatalf("String('é') = %q", got)
	}
}

func TestHashIsStableForSameInput(t *testing.T) {
	if a, b := Hash("abcdef"), Hash("abcdef"); a != b {
		t.Fatalf("Hash mismatch: %d != %d", a, b)
	}
}

func TestIntUsesCacheForSmallValues(t *testing.T) {
	if got := Int(42); got != "42" {
		t.Fatalf("Int(42) = %q", got)
	}
}

func TestIntHandlesLargeValues(t *testing.T) {
	if got := Int(50000); got != "50000" {
		t.Fatalf("Int(50000) = %q", got)
	}
}

func TestJoinDotted(t *testing.T) {
	if got := JoinDotted([]string{"Math", "PI"}); got != "Math.PI" {
		t.Fatalf("JoinDotted() = %q", got)
	}
}
