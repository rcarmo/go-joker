package osutil

import (
	"bytes"
	"errors"
	"testing"
)

type errorReader struct {
	data []byte
	err  error
}

func (r *errorReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

type shortErrorWriter struct{}

func (shortErrorWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) / 2, errors.New("short write")
}

func TestReadAllStringAndWriteString(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteString(&buf, "hello"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAllString(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("ReadAllString() = %q", got)
	}
}

func TestByteRuneReader(t *testing.T) {
	r := ByteRuneReader([]byte("abc"))
	if _, _, err := r.ReadRune(); err != nil {
		t.Fatal(err)
	}
}

func TestReadAllStringPropagatesInterruptedRead(t *testing.T) {
	wantErr := errors.New("interrupted")
	got, err := ReadAllString(&errorReader{data: []byte("partial"), err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ReadAllString error = %v, want %v", err, wantErr)
	}
	if got != "" {
		t.Fatalf("ReadAllString returned partial data %q with error", got)
	}
}

func TestWriteStringPropagatesShortWrite(t *testing.T) {
	if err := WriteString(shortErrorWriter{}, "payload"); err == nil || err.Error() != "short write" {
		t.Fatalf("WriteString error = %v, want short write", err)
	}
}
