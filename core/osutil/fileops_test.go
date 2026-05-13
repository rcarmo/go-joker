package osutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteFileString(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.txt")
	if err := WriteFileString(path, "hello", false); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFileString(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("ReadFileString() = %q", got)
	}
	if err := WriteFileString(path, " world", true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello world" {
		t.Fatalf("append result = %q", string(b))
	}
}
