package filepath

import (
	"os"
	"path/filepath"
	"testing"
)

func expectPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func TestFileSeqMissingRootPanics(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	expectPanic(t, func() {
		_ = fileSeq(missing)
	})
}

func TestFileSeqReturnsEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "d"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	v := fileSeq(root)
	if v.Count() < 3 { // root + file + dir
		t.Fatalf("unexpected file count: %d", v.Count())
	}
}
