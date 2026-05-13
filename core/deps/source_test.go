package deps

import (
	"path/filepath"
	"testing"
)

func TestExternalSourceToPathForLocalSource(t *testing.T) {
	got, err := ExternalSourceToPath("/tmp/home", "joker.core", "/repo/lib")
	if err != nil {
		t.Fatalf("ExternalSourceToPath returned error: %v", err)
	}
	want := filepath.Join("/repo/lib", "joker", "core") + ".joke"
	if got != want {
		t.Fatalf("ExternalSourceToPath = %q, want %q", got, want)
	}
}
