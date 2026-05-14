package generated

import (
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func TestGeneratedCoreNamespacesHelper(t *testing.T) {
	got := CoreNamespaces()
	if len(got) == 0 || got[0] != "joker.better-cond" {
		t.Fatalf("unexpected generated core namespaces helper result: %v", got)
	}
}

func TestGeneratedCoreSourceManifestRows(t *testing.T) {
	manifest := CoreSourceManifest()
	if len(manifest) == 0 {
		t.Fatal("generated core source manifest is empty")
	}
	for _, src := range manifest {
		if src.Name == "" || src.Path == "" {
			t.Fatalf("manifest row must include namespace and path: %#v", src)
		}
	}

	got := CoreNamespaces()
	want := []string{"joker.better-cond", "joker.core", "joker.hiccup", "joker.pprint", "joker.repl", "joker.set", "joker.template", "joker.test", "joker.tools.cli", "joker.walk"}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("manifest namespace count = %d %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("manifest namespaces = %v, want %v", got, want)
		}
	}
}

func TestCoreSourceManifest(t *testing.T) {
	manifest := CoreSourceManifest()
	if len(manifest) == 0 {
		t.Fatal("core source manifest should not be empty")
	}
	if manifest[0].Name != "joker.core" || manifest[0].Path != "core.joke" {
		t.Fatalf("unexpected first source manifest row: %#v", manifest[0])
	}
	for _, src := range manifest {
		if src.Name == "" || src.Path == "" {
			t.Fatalf("source manifest rows must include name/path: %#v", src)
		}
	}
}

func TestCoreSourceManifestReturnsFreshSlice(t *testing.T) {
	manifest := CoreSourceManifest()
	if len(manifest) == 0 {
		t.Fatal("core source manifest should not be empty")
	}
	original := manifest[0]
	manifest[0].Name = "mutated"
	again := CoreSourceManifest()
	if again[0] != original {
		t.Fatalf("CoreSourceManifest should return fresh immutable-by-convention payload slices: got %#v want %#v", again[0], original)
	}
}

func TestCoreSourceManifestPathsExist(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dataDir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "data"))
	seen := map[string]bool{}
	for _, src := range CoreSourceManifest() {
		if seen[src.Path] {
			t.Fatalf("duplicate source path in manifest: %s", src.Path)
		}
		seen[src.Path] = true
		if _, err := filepath.Abs(filepath.Join(dataDir, src.Path)); err != nil {
			t.Fatalf("bad source path %q: %v", src.Path, err)
		}
		if matches, err := filepath.Glob(filepath.Join(dataDir, src.Path)); err != nil || len(matches) != 1 {
			t.Fatalf("source path %q should match one data file, matches=%v err=%v", src.Path, matches, err)
		}
	}
}

func TestBootstrapPayloadsAreDataOnly(t *testing.T) {
	src := NamespaceSource{Name: "joker.core", Path: "core.joke", Source: "(ns joker.core)"}
	if src.Name == "" || src.Path == "" || src.Source == "" {
		t.Fatalf("namespace source should carry inert payload fields: %#v", src)
	}
	doc := VarDoc{Namespace: "joker.core", Name: "+", Doc: "Adds numbers", Arglists: []string{"[x y]"}}
	if doc.Namespace != src.Name || doc.Name == "" || len(doc.Arglists) != 1 || doc.Private {
		t.Fatalf("var doc should carry inert metadata fields: %#v", doc)
	}
	payload := BinaryPayload{Path: "linter_all.joke", Data: []byte{1, 2, 3}}
	if payload.Path == "" || len(payload.Data) != 3 {
		t.Fatalf("binary payload should carry inert path/data fields: %#v", payload)
	}
}
