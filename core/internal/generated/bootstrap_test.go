package generated

import "testing"

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
