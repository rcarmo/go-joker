package reader

import "testing"

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
