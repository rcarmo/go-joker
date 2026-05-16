package csv

import (
	"errors"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func expectCSVPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func TestCSVOptionsRejectInvalidDelimiters(t *testing.T) {
	opts := EmptyArrayMap()
	opts.Add(MakeKeyword("comma"), Char{Ch: '\n'})
	expectCSVPanic(t, func() { _ = writeString(NewVectorFrom(NewVectorFrom(MakeString("a"))), opts) })

	readOpts := EmptyArrayMap()
	readOpts.Add(MakeKeyword("comma"), Char{Ch: ';'})
	readOpts.Add(MakeKeyword("comment"), Char{Ch: ';'})
	expectCSVPanic(t, func() { _ = csvSeqOpts(MakeString("a;b\n"), readOpts) })
}

func TestWriteWriterSurfacesFlushErrors(t *testing.T) {
	data := NewVectorFrom(NewVectorFrom(MakeString("a"), MakeString("b")))
	expectCSVPanic(t, func() { writeWriter(failingWriter{}, data, EmptyArrayMap()) })
}

func TestWriteStringBasic(t *testing.T) {
	data := NewVectorFrom(NewVectorFrom(MakeString("a"), MakeString("b")))
	if got := writeString(data, EmptyArrayMap()); got != "a,b\n" {
		t.Fatalf("writeString = %q", got)
	}
}
