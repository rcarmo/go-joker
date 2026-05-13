package deps

import (
	"path/filepath"
	"testing"
)

func TestLibNamePath(t *testing.T) {
	want := filepath.Join("joker", "core") + ".joke"
	if got := LibNamePath("joker.core"); got != want {
		t.Fatalf("LibNamePath() = %q, want %q", got, want)
	}
}

func TestResolveLibPath(t *testing.T) {
	want := filepath.Join("/tmp/base", "joker", "core") + ".joke"
	if got := ResolveLibPath("/tmp/base", "joker.core"); got != want {
		t.Fatalf("ResolveLibPath() = %q, want %q", got, want)
	}
}
