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
