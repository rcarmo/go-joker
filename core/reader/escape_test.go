package reader

import "testing"

func TestDecodeSimpleStringEscape(t *testing.T) {
	tests := map[rune]rune{
		'\\': '\\',
		'"':  '"',
		'n':  '\n',
		't':  '\t',
		'r':  '\r',
		'b':  '\b',
		'f':  '\f',
	}
	for in, want := range tests {
		got, ok := DecodeSimpleStringEscape(in)
		if !ok || got != want {
			t.Fatalf("DecodeSimpleStringEscape(%q) = %q/%v, want %q/true", in, got, ok, want)
		}
	}
	if got, ok := DecodeSimpleStringEscape('x'); ok || got != 0 {
		t.Fatalf("DecodeSimpleStringEscape unsupported = %q/%v, want 0/false", got, ok)
	}
}
