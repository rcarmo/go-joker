package reader

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type closeReader struct {
	io.Reader
	closed bool
}

func (r *closeReader) Close() error {
	r.closed = true
	return nil
}

type closeWriter struct {
	io.Writer
	closed bool
}

func (w *closeWriter) Close() error {
	w.closed = true
	return nil
}

func TestBufferWrapsBytesBufferAndHashes(t *testing.T) {
	buf := NewBuffer(bytes.NewBufferString("abc"))
	if buf.Hash() == 0 {
		t.Fatal("buffer hash should be initialized")
	}
	if got := buf.String(); got != "abc" {
		t.Fatalf("String() = %q, want abc", got)
	}
}

func TestBufferedWrapsReaderAndHashes(t *testing.T) {
	br := NewBuffered(bytes.NewBufferString("abc"))
	if br.Hash() == 0 {
		t.Fatal("buffered reader hash should be initialized")
	}
	b, err := br.ReadByte()
	if err != nil || b != 'a' {
		t.Fatalf("ReadByte = %q, %v; want 'a', nil", b, err)
	}
}

func TestIOReaderClose(t *testing.T) {
	cr := &closeReader{Reader: bytes.NewBufferString("abc")}
	ior := NewIOReader(cr)
	if ior.Hash() == 0 {
		t.Fatal("IO reader hash should be initialized")
	}
	if err := ior.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !cr.closed {
		t.Fatal("Close did not reach wrapped reader")
	}
}

func TestIOReaderCloseNonCloser(t *testing.T) {
	ior := NewIOReader(bytes.NewBufferString("abc"))
	if err := ior.Close(); !errors.Is(err, ErrNotClosable) {
		t.Fatalf("Close error = %v, want ErrNotClosable", err)
	}
}

func TestIOWriterClose(t *testing.T) {
	cw := &closeWriter{Writer: &bytes.Buffer{}}
	iow := NewIOWriter(cw)
	if iow.Hash() == 0 {
		t.Fatal("IO writer hash should be initialized")
	}
	if err := iow.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !cw.closed {
		t.Fatal("Close did not reach wrapped writer")
	}
}

func TestIOWriterCloseNonCloser(t *testing.T) {
	iow := NewIOWriter(&bytes.Buffer{})
	if err := iow.Close(); !errors.Is(err, ErrNotClosable) {
		t.Fatalf("Close error = %v, want ErrNotClosable", err)
	}
}
