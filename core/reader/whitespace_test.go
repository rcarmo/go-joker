package reader

import "testing"

func TestSkipWhitespaceRun(t *testing.T) {
	r := newCommentReader(" \t\n{")
	if got := SkipWhitespaceRun(r, ' '); got != '{' {
		t.Fatalf("SkipWhitespaceRun = %q, want {", got)
	}
	r = newCommentReader("")
	if got := SkipWhitespaceRun(r, 'x'); got != 'x' {
		t.Fatalf("SkipWhitespaceRun non-space = %q, want x", got)
	}
}

func TestWhitespaceSkipHelpers(t *testing.T) {
	if !ShouldPreserveComma(true, ',') || ShouldPreserveComma(false, ',') || ShouldPreserveComma(true, 'x') {
		t.Fatal("unexpected comma preservation result")
	}
	if !ShouldSkipReaderComment(false, ';', 0) || !ShouldSkipReaderComment(false, '#', '!') {
		t.Fatal("expected comment/shebang skip")
	}
	if ShouldSkipReaderComment(true, ';', 0) || ShouldSkipReaderComment(false, '#', '_') {
		t.Fatal("unexpected comment skip")
	}
	if !ShouldDiscardNextForm(false, '#', '_') || ShouldDiscardNextForm(true, '#', '_') || ShouldDiscardNextForm(false, '#', '!') {
		t.Fatal("unexpected discard form decision")
	}
}
