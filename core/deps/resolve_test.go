package deps

import (
	"path/filepath"
	"testing"
)

func TestResolveRelativeLibPath(t *testing.T) {
	currentFile := filepath.Join("/repo", "src", "user", "core.joke")
	got := ResolveRelativeLibPath(currentFile, "user.core", "joker.string")
	want := filepath.Join("/repo", "src", "joker", "string") + ".joke"
	if got != want {
		t.Fatalf("ResolveRelativeLibPath() = %q, want %q", got, want)
	}
}
