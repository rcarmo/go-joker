package reader

import "testing"

func TestScanRegexLiteral(t *testing.T) {
	r := newCommentReader(`a\"b"tail`)
	got, ok := ScanRegexLiteral(r)
	if !ok || got != `a\"b` {
		t.Fatalf("ScanRegexLiteral = %q/%v, want escaped body/true", got, ok)
	}
	if got := r.Peek(); got != 't' {
		t.Fatalf("remaining peek = %q, want t", got)
	}
}

func TestScanRegexLiteralEOF(t *testing.T) {
	for _, in := range []string{"abc", `abc\`} {
		r := newCommentReader(in)
		if got, ok := ScanRegexLiteral(r); ok || got != "" {
			t.Fatalf("ScanRegexLiteral(%q) = %q/%v, want empty/false", in, got, ok)
		}
	}
}
