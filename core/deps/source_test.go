package deps

import (
	"path/filepath"
	"testing"
)

func TestExternalURLCacheBaseRejectsMalformedURL(t *testing.T) {
	if _, err := externalURLCacheBase("http:"); err == nil {
		t.Fatal("externalURLCacheBase accepted malformed URL")
	}
	if got, err := externalURLCacheBase("https://example.test/lib/"); err != nil || got != "example.test/lib/" {
		t.Fatalf("externalURLCacheBase = %q/%v", got, err)
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
