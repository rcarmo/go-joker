package reader

import "testing"

type commentReader struct {
	runes []rune
	pos   int
	eof   bool
}

func newCommentReader(s string) *commentReader { return &commentReader{runes: []rune(s)} }

func (r *commentReader) Peek() rune {
	if r.eof || r.pos >= len(r.runes) {
		return EOF
	}
	return r.runes[r.pos]
}

func (r *commentReader) Get() rune {
	ch := r.Peek()
	if ch == EOF {
		r.eof = true
		return EOF
	}
	r.pos++
	return ch
}

func (r *commentReader) Unget() {
	if r.eof {
		return
	}
	if r.pos > 0 {
		r.pos--
	}
}

func TestReadCommentText(t *testing.T) {
	r := newCommentReader("; hello\nnext")
	if got := ReadCommentText(r); got != "; hello" {
		t.Fatalf("ReadCommentText = %q, want %q", got, "; hello")
	}
	if got := r.Peek(); got != '\n' {
		t.Fatalf("remaining peek = %q, want newline", got)
	}
}

func TestReadCommentTextEOF(t *testing.T) {
	r := newCommentReader("#! /bin/joker")
	if got := ReadCommentText(r); got != "#! /bin/joker" {
		t.Fatalf("ReadCommentText EOF = %q", got)
	}
	if got := r.Peek(); got != EOF {
		t.Fatalf("remaining peek = %q, want EOF", got)
	}
}

func TestSkipLine(t *testing.T) {
	r := newCommentReader("ignored\nnext")
	if got := SkipLine(r, 'i'); got != 'n' {
		t.Fatalf("SkipLine = %q, want n", got)
	}
	r = newCommentReader("ignored")
	if got := SkipLine(r, 'i'); got != EOF {
		t.Fatalf("SkipLine EOF = %q, want EOF", got)
	}
}
