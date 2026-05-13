package osutil

import (
	"bytes"
	"testing"
)

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
