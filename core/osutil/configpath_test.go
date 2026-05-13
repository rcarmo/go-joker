package osutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindConfigPathFindsParentConfig(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(root, ".joker")
	if err := os.WriteFile(cfg, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FindConfigPath(filepath.Join(nested, "file.joke"), "", ".joker", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != cfg {
		t.Fatalf("FindConfigPath() = %q, want %q", got, cfg)
	}
}

func TestFindConfigPathFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, ".joker")
	if err := os.WriteFile(cfg, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FindConfigPath("/", "", ".joker", home, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != cfg {
		t.Fatalf("FindConfigPath() = %q, want %q", got, cfg)
	}
}
