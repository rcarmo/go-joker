package generated

import "testing"

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
