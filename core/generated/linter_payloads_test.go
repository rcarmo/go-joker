//go:build !gen_code
// +build !gen_code

package generated

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLinterDataPayloadsMatchSourceManifest(t *testing.T) {
	manifestLinterPaths := map[string]bool{}
	for _, src := range CoreSourceManifest() {
		if filepath.Dir(src.Path) == "." && strings.HasPrefix(src.Path, "linter_") {
			manifestLinterPaths[src.Path] = true
		}
	}
	payloads := LinterDataPayloads()
	if len(payloads) != len(manifestLinterPaths) {
		t.Fatalf("linter payload count = %d, manifest linter path count = %d", len(payloads), len(manifestLinterPaths))
	}
	seen := map[string]bool{}
	for _, payload := range payloads {
		if payload.Path == "" || len(payload.Data) == 0 {
			t.Fatalf("linter payload must include path and non-empty data: %#v", payload)
		}
		if seen[payload.Path] {
			t.Fatalf("duplicate linter payload path: %s", payload.Path)
		}
		seen[payload.Path] = true
		if !manifestLinterPaths[payload.Path] {
			t.Fatalf("linter payload path %q not present in source manifest", payload.Path)
		}
		got, ok := LinterDataByPath(payload.Path)
		if !ok || len(got) != len(payload.Data) {
			t.Fatalf("LinterDataByPath(%q) length = %d ok=%v, want %d true", payload.Path, len(got), ok, len(payload.Data))
		}
	}
	for path := range manifestLinterPaths {
		if !seen[path] {
			t.Fatalf("source manifest linter path %q has no generated payload", path)
		}
	}
}
