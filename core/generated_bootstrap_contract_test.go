package core

import (
	"sort"
	"testing"

	coregenerated "github.com/rcarmo/go-joker/core/internal/generated"
)

func TestGeneratedCoreNamespacesHelper(t *testing.T) {
	got := generatedCoreNamespaces()
	if len(got) == 0 || got[0] != "joker.better-cond" {
		t.Fatalf("unexpected generated core namespaces helper result: %v", got)
	}
}

func TestGeneratedCoreSourceManifestRows(t *testing.T) {
	manifest := coregenerated.CoreSourceManifest()
	if len(manifest) == 0 {
		t.Fatal("generated core source manifest is empty")
	}
	for _, src := range manifest {
		if src.Name == "" || src.Path == "" {
			t.Fatalf("manifest row must include namespace and path: %#v", src)
		}
	}

	got := generatedCoreNamespaces()
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
