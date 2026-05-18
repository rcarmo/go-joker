package csv

import (
	"errors"
	coretypes "github.com/rcarmo/go-joker/core/types"
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
	opts.Add(coretypes.MakeKeyword(STRINGS.Intern, "comma"), coretypes.Char{Ch: '\n'})
	expectCSVPanic(t, func() { _ = writeString(NewVectorFrom(NewVectorFrom(coretypes.MakeString("a"))), opts) })

	readOpts := EmptyArrayMap()
	readOpts.Add(coretypes.MakeKeyword(STRINGS.Intern, "comma"), coretypes.Char{Ch: ';'})
	readOpts.Add(coretypes.MakeKeyword(STRINGS.Intern, "comment"), coretypes.Char{Ch: ';'})
	expectCSVPanic(t, func() { _ = csvSeqOpts(coretypes.MakeString("a;b\n"), readOpts) })
}

func TestWriteWriterSurfacesFlushErrors(t *testing.T) {
	data := NewVectorFrom(NewVectorFrom(coretypes.MakeString("a"), coretypes.MakeString("b")))
	expectCSVPanic(t, func() { writeWriter(failingWriter{}, data, EmptyArrayMap()) })
}

func TestWriteStringBasic(t *testing.T) {
	data := NewVectorFrom(NewVectorFrom(coretypes.MakeString("a"), coretypes.MakeString("b")))
	if got := writeString(data, EmptyArrayMap()); got != "a,b\n" {
		t.Fatalf("writeString = %q", got)
	}
}
