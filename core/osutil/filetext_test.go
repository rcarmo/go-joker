package osutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExistingChild(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.edn")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ExistingChild(dir, "config.edn"); got != path {
		t.Fatalf("ExistingChild() = %q, want %q", got, path)
	}
}

func TestOpenRuneFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, r, err := OpenRuneFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatalf("close file: %v", err)
		}
	}()
	if _, _, err := r.ReadRune(); err != nil {
		t.Fatal(err)
	}
}
