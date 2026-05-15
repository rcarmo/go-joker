package reader

import "testing"

func TestScanUntilDelimiter(t *testing.T) {
	r := newCommentReader("123 abc")
	if got := ScanUntilDelimiter(r); got != "123" {
		t.Fatalf("ScanUntilDelimiter = %q, want 123", got)
	}
	if got := r.Peek(); got != ' ' {
		t.Fatalf("remaining peek = %q, want space", got)
	}
}

func TestScanUntilDelimiterEOF(t *testing.T) {
	r := newCommentReader("token")
	if got := ScanUntilDelimiter(r); got != "token" {
		t.Fatalf("ScanUntilDelimiter EOF = %q, want token", got)
	}
	if got := r.Peek(); got != EOF {
		t.Fatalf("remaining peek = %q, want EOF", got)
	}
}
