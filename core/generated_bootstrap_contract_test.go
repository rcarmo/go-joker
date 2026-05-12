package core

import (
	"sort"
	"testing"

	coregenerated "github.com/rcarmo/go-joker/core/internal/generated"
)

func TestGeneratedCoreSourceManifestMatchesCoreNamespaces(t *testing.T) {
	manifest := coregenerated.CoreSourceManifest()
	if len(manifest) == 0 {
		t.Fatal("generated core source manifest is empty")
	}
	manifestNamespaces := map[string]bool{}
	for _, src := range manifest {
		if src.Name == "" || src.Path == "" {
			t.Fatalf("manifest row must include namespace and path: %#v", src)
		}
		manifestNamespaces[src.Name] = true
	}

	var want []string
	for _, ns := range coreNamespaces {
		if ns != "user" {
			want = append(want, ns)
		}
	}
	sort.Strings(want)
	var got []string
	for ns := range manifestNamespaces {
		got = append(got, ns)
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("manifest namespace count = %d %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("manifest namespaces = %v, want %v", got, want)
		}
	}
}
