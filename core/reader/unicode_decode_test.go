package reader

import "testing"

func TestDecodeStringEscapeCode(t *testing.T) {
	r, err := DecodeStringEscapeCode("0041", 4, 16, true)
	if err != nil {
		t.Fatalf("DecodeStringEscapeCode returned error: %v", err)
	}
	if r != 'A' {
		t.Fatalf("DecodeStringEscapeCode = %q, want A", r)
	}
}

func TestDecodeStringEscapeCodeErrors(t *testing.T) {
	if _, err := DecodeStringEscapeCode("41", 4, 16, true); err == nil {
		t.Fatal("short exact escape did not fail")
	}
	if _, err := DecodeStringEscapeCode("zzzz", 4, 16, true); err == nil {
		t.Fatal("invalid escape did not fail")
	}
}
