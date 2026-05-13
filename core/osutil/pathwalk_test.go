package osutil

import (
	"path/filepath"
	"testing"
)

func TestParentDir(t *testing.T) {
	path := filepath.Join("/tmp", "a", "b", "file.joke")
	got := ParentDir(path, 2)
	want := filepath.Join("/tmp", "a")
	if got != want {
		t.Fatalf("ParentDir() = %q, want %q", got, want)
	}
}
