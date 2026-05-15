package reader

import "testing"

func TestClassifyStringEscape(t *testing.T) {
	if got := ClassifyStringEscape('n'); got != StringEscapeSimple {
		t.Fatalf("ClassifyStringEscape('n') = %v, want simple", got)
	}
	if got := ClassifyStringEscape('u'); got != StringEscapeUnicode {
		t.Fatalf("ClassifyStringEscape('u') = %v, want unicode", got)
	}
	if got := ClassifyStringEscape('7'); got != StringEscapeOctal {
		t.Fatalf("ClassifyStringEscape('7') = %v, want octal", got)
	}
	if got := ClassifyStringEscape('x'); got != StringEscapeUnsupported {
		t.Fatalf("ClassifyStringEscape('x') = %v, want unsupported", got)
	}
}

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
