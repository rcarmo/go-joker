package generated

import (
	"path/filepath"
	"runtime"
	"testing"
)

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

func TestCoreSourceManifestPathsExist(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dataDir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "data"))
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
}
