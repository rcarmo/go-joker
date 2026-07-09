package deps

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalURLCacheBaseRejectsMalformedURL(t *testing.T) {
	if _, err := externalURLCacheBase("http:"); err == nil {
		t.Fatal("externalURLCacheBase accepted malformed URL")
	}
	if got, err := externalURLCacheBase("https://example.test/lib/"); err != nil || !strings.HasPrefix(got, "example.test-") || strings.ContainsAny(got, `/\\`) {
		t.Fatalf("externalURLCacheBase = %q/%v", got, err)
	}
}

func TestSafeCachePathRejectsTraversal(t *testing.T) {
	base := t.TempDir()
	for _, path := range []string{"../escape.joke", "/absolute.joke"} {
		if _, err := safeCachePath(base, path); err == nil {
			t.Fatalf("safeCachePath accepted %q", path)
		}
	}
	if got, err := safeCachePath(base, filepath.Join("safe", "lib.joke")); err != nil || !strings.HasPrefix(got, base+string(filepath.Separator)) {
		t.Fatalf("safeCachePath = %q/%v", got, err)
	}
}

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
