package string

import "testing"

func TestNthRuneASCIIAndUnicode(t *testing.T) {
	if r, _, ok := NthRune("abcdef", 3); !ok || r != 'd' {
		t.Fatalf("NthRune ASCII = %q, %v; want d, true", r, ok)
	}
	if r, _, ok := NthRune("éclair", 1); !ok || r != 'c' {
		t.Fatalf("NthRune Unicode = %q, %v; want c, true", r, ok)
	}
}

func TestNthRuneOutOfRange(t *testing.T) {
	if _, _, ok := NthRune("abc", -1); ok {
		t.Fatal("NthRune accepted negative index")
	}
	if _, n, ok := NthRune("éx", 3); ok || n != 2 {
		t.Fatalf("NthRune out of range length = %d, %v; want 2, false", n, ok)
	}
}
