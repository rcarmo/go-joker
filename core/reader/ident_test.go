package reader

import "testing"

func TestIsIdentRune(t *testing.T) {
	for _, r := range []rune{'a', 'Z', '0', '-', '_', ':', '/', '?', '!'} {
		if !IsIdentRune(r) {
			t.Fatalf("IsIdentRune(%q) = false, want true", r)
		}
	}
	for _, r := range []rune{'"', ';', '@', '^', '`', '~', '(', ')', '[', ']', '{', '}', '\\', ',', ' ', '\t', '\n', '\r', EOF} {
		if IsIdentRune(r) {
			t.Fatalf("IsIdentRune(%q) = true, want false", r)
		}
	}
	if IsIdentRune('\u2003') {
		t.Fatal("IsIdentRune(em space) = true, want false")
	}
}
