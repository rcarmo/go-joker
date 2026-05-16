package reader

import (
	"errors"
	"strings"
	"testing"
)

func TestRuneStreamGetUngetPeekPositions(t *testing.T) {
	s := NewRuneStream(strings.NewReader("a\nb"), nil)
	if s.Line() != 1 || s.Column() != 0 {
		t.Fatalf("initial position = %d:%d", s.Line(), s.Column())
	}
	if r := s.Get(); r != 'a' || s.Line() != 1 || s.Column() != 1 {
		t.Fatalf("after a = %q at %d:%d", r, s.Line(), s.Column())
	}
	if r := s.Get(); r != '\n' || s.Line() != 2 || s.Column() != 0 {
		t.Fatalf("after newline = %q at %d:%d", r, s.Line(), s.Column())
	}
	s.Unget()
	if s.Line() != 1 || s.Column() != 1 {
		t.Fatalf("after unget newline position = %d:%d", s.Line(), s.Column())
	}
	if r := s.Peek(); r != '\n' || s.Line() != 1 || s.Column() != 1 {
		t.Fatalf("peek newline = %q at %d:%d", r, s.Line(), s.Column())
	}
	if r := s.Get(); r != '\n' || s.Line() != 2 || s.Column() != 0 {
		t.Fatalf("re-read newline = %q at %d:%d", r, s.Line(), s.Column())
	}
	if r := s.Get(); r != 'b' || s.Line() != 2 || s.Column() != 1 {
		t.Fatalf("after b = %q at %d:%d", r, s.Line(), s.Column())
	}
	if r := s.Get(); r != EOF {
		t.Fatalf("EOF read = %q", r)
	}
}

type failingRuneReader struct{}

func (failingRuneReader) ReadRune() (rune, int, error) { return 0, 0, errors.New("boom") }

func TestRuneStreamErrorCallback(t *testing.T) {
	called := false
	s := NewRuneStream(failingRuneReader{}, func(err error) {
		called = true
		panic("wrapped: " + err.Error())
	})
	defer func() {
		if r := recover(); r != "wrapped: boom" || !called {
			t.Fatalf("recover = %#v called=%v", r, called)
		}
	}()
	_ = s.Get()
}
