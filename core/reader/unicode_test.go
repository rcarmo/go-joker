package reader

import "testing"

func TestParseExactUnicodeCode(t *testing.T) {
	if got, ok := ParseExactUnicodeCode("0041", 4, 16); !ok || got != 'A' {
		t.Fatalf("ParseExactUnicodeCode valid = %q/%v", got, ok)
	}
	if _, ok := ParseExactUnicodeCode("041", 4, 16); ok {
		t.Fatal("ParseExactUnicodeCode accepted short token")
	}
	if _, ok := ParseExactUnicodeCode("xxxx", 4, 16); ok {
		t.Fatal("ParseExactUnicodeCode accepted invalid token")
	}
}

func TestParseUnicodeCode(t *testing.T) {
	tests := []struct {
		str  string
		base int
		want rune
	}{
		{str: "41", base: 16, want: 'A'},
		{str: "101", base: 8, want: 'A'},
		{str: "03bb", base: 16, want: 'λ'},
	}
	for _, tt := range tests {
		got, err := ParseUnicodeCode(tt.str, tt.base)
		if err != nil {
			t.Fatalf("ParseUnicodeCode(%q, %d) error: %v", tt.str, tt.base, err)
		}
		if got != tt.want {
			t.Fatalf("ParseUnicodeCode(%q, %d) = %q, want %q", tt.str, tt.base, got, tt.want)
		}
	}
	if _, err := ParseUnicodeCode("not-hex", 16); err == nil {
		t.Fatal("ParseUnicodeCode invalid input succeeded")
	}
}

func TestHasExactLength(t *testing.T) {
	if !HasExactLength("03bb", 4) {
		t.Fatal("HasExactLength returned false for exact ASCII escape length")
	}
	if HasExactLength("3bb", 4) {
		t.Fatal("HasExactLength returned true for short escape")
	}
}

func TestScanStringEscapeCode(t *testing.T) {
	r := newCommentReader("3bb\"rest")
	if got := ScanStringEscapeCode(r, '0', 4); got != "03bb" {
		t.Fatalf("ScanStringEscapeCode = %q, want 03bb", got)
	}
	if got := r.Peek(); got != '"' {
		t.Fatalf("remaining peek = %q, want quote", got)
	}
}
