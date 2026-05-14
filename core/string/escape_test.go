package string

import "testing"

func TestEscapeRune(t *testing.T) {
	cases := map[rune]string{
		' ':  "\\space",
		'\n': "\\newline",
		'\t': "\\tab",
		'\r': "\\return",
		'\b': "\\backspace",
		'\f': "\\formfeed",
		'x':  "\\x",
	}
	for in, want := range cases {
		if got := EscapeRune(in); got != want {
			t.Fatalf("EscapeRune(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEscapeString(t *testing.T) {
	if got := EscapeString("a\"b\\c\n"); got != `"a\"b\\c\n"` {
		t.Fatalf("EscapeString = %q", got)
	}
}
