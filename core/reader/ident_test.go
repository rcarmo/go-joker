package reader

import "testing"

func TestClassifyIdentLiteral(t *testing.T) {
	cases := map[string]IdentLiteralKind{
		"nil":   IdentLiteralNil,
		"true":  IdentLiteralTrue,
		"false": IdentLiteralFalse,
		"foo":   IdentLiteralSymbol,
		"Nil":   IdentLiteralSymbol,
	}
	for token, want := range cases {
		if got := ClassifyIdentLiteral(token); got != want {
			t.Fatalf("ClassifyIdentLiteral(%q) = %v, want %v", token, got, want)
		}
	}
}

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
