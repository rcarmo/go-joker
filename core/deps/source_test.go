package deps

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type repeatingByteReader byte

func (r repeatingByteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r)
	}
	return len(p), nil
}

func withExternalHTTPClient(t *testing.T, fn roundTripFunc) {
	t.Helper()
	old := externalHTTPClient
	externalHTTPClient = &http.Client{Transport: fn}
	t.Cleanup(func() { externalHTTPClient = old })
}

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

func TestExternalHTTPSourceRejectsTraversalBeforeRequest(t *testing.T) {
	requested := false
	withExternalHTTPClient(t, func(*http.Request) (*http.Response, error) {
		requested = true
		return nil, nil
	})
	if _, err := ExternalHTTPSourceToPath(t.TempDir(), "../../escape", "https://example.test/lib/"); err == nil {
		t.Fatal("ExternalHTTPSourceToPath accepted a traversal-like library name")
	}
	if requested {
		t.Fatal("invalid dependency path reached the network boundary")
	}
}

func TestExternalHTTPSourceRejectsOversizeAndCleansTemporaryFile(t *testing.T) {
	withExternalHTTPClient(t, func(*http.Request) (*http.Response, error) {
		body := io.NopCloser(io.LimitReader(repeatingByteReader('x'), maxExternalSourceBytes+1))
		return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
	})

	home := t.TempDir()
	if _, err := ExternalHTTPSourceToPath(home, "safe.lib", "https://example.test/lib/"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized dependency error = %v", err)
	}
	var leftovers []string
	err := filepath.Walk(filepath.Join(home, ".jokerd"), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.Contains(info.Name(), ".dependency-") || strings.HasSuffix(info.Name(), ".joke") {
			leftovers = append(leftovers, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("failed download left cache artifacts: %v", leftovers)
	}
}

func TestExternalHTTPSourceDoesNotCacheErrorResponse(t *testing.T) {
	withExternalHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}, nil
	})
	home := t.TempDir()
	if _, err := ExternalHTTPSourceToPath(home, "safe.lib", "https://example.test/lib/"); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("HTTP error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(home, ".jokerd", "deps", "*", "safe", "lib.joke"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("error response was cached: %v", matches)
	}
}
