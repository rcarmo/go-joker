package osutil

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadLineStripsCRLF(t *testing.T) {
	got, err := ReadLine(bufio.NewReader(strings.NewReader("hello\r\nworld")))
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("ReadLine() = %q", got)
	}
}

func TestAsRuneReader(t *testing.T) {
	r := AsRuneReader(strings.NewReader("abc"))
	if _, _, err := r.ReadRune(); err != nil {
		t.Fatal(err)
	}
}
