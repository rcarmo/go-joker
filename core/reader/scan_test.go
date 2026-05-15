package reader

import "testing"

func TestConsumeExpectedAndPeekDelimiter(t *testing.T) {
	r := newCommentReader("abc def")
	if got, ok := ConsumeExpected(r, "abc"); !ok || got != 0 {
		t.Fatalf("ConsumeExpected ok = %q/%v, want 0/true", got, ok)
	}
	if !PeekDelimiter(r) {
		t.Fatal("PeekDelimiter after expected token = false, want true")
	}

	r = newCommentReader("ax")
	if got, ok := ConsumeExpected(r, "ab"); ok || got != 'x' {
		t.Fatalf("ConsumeExpected mismatch = %q/%v, want x/false", got, ok)
	}
}

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
