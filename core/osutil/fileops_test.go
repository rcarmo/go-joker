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

func TestFileOperationsSurfaceMissingAndInvalidPaths(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing", "file.txt")
	if _, err := ReadFileString(missing); err == nil {
		t.Fatal("ReadFileString accepted a missing path")
	}
	if _, err := ReadFileBytes(missing); err == nil {
		t.Fatal("ReadFileBytes accepted a missing path")
	}
	if err := WriteFileString(missing, "data", false); err == nil {
		t.Fatal("WriteFileString silently created a missing parent directory")
	}
	if err := WriteFileString(root, "data", false); err == nil {
		t.Fatal("WriteFileString accepted a directory as a file")
	}
}
