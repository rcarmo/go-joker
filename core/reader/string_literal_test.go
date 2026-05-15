package reader

import "testing"

func testUnicodeDecoder(r *commentReader) UnicodeEscapeDecoder {
	return func(initial rune, length, base int, exactLength bool) rune {
		code := ScanStringEscapeCode(r, initial, length)
		if base == 16 && exactLength && code == "03bb" {
			return 'λ'
		}
		if base == 8 && !exactLength && code == "101" {
			return 'A'
		}
		return '?'
	}
}

func TestScanStringLiteral(t *testing.T) {
	r := newCommentReader("a\\n\\u03bb\\101\"tail")
	got, err := ScanStringLiteral(r, false, testUnicodeDecoder(r))
	if err != nil {
		t.Fatalf("ScanStringLiteral error: %v", err)
	}
	if got != "a\nλA" {
		t.Fatalf("ScanStringLiteral = %q, want escaped string", got)
	}
	if got := r.Peek(); got != 't' {
		t.Fatalf("remaining peek = %q, want t", got)
	}
}

func TestScanStringLiteralFormatModePreservesEscapes(t *testing.T) {
	r := newCommentReader("a\\n\"tail")
	got, err := ScanStringLiteral(r, true, testUnicodeDecoder(r))
	if err != nil {
		t.Fatalf("ScanStringLiteral format error: %v", err)
	}
	if got != `a\n` {
		t.Fatalf("ScanStringLiteral format = %q, want preserved escape", got)
	}
}

func TestScanStringLiteralErrors(t *testing.T) {
	r := newCommentReader("a\\x\"tail")
	if got, err := ScanStringLiteral(r, false, testUnicodeDecoder(r)); err == nil || got != "" {
		t.Fatalf("unsupported escape = %q/%v, want error", got, err)
	}
	r = newCommentReader("unterminated")
	if got, err := ScanStringLiteral(r, false, testUnicodeDecoder(r)); err == nil || got != "" {
		t.Fatalf("unterminated = %q/%v, want error", got, err)
	}
}
