//go:build !gen_code
// +build !gen_code

package core

import (
	"sort"

	coregenerated "github.com/rcarmo/go-joker/core/internal/generated"
)

// generatedCoreNamespaces returns the namespace set represented by the
// data-only generated bootstrap manifest. The root runtime still owns mutation
// of *core-namespaces*; this helper is the first runtime consumer of the
// generated bootstrap contract and is guarded against a_data.go by tests.
func generatedCoreNamespaces() []string {
	seen := map[string]bool{}
	for _, src := range coregenerated.CoreSourceManifest() {
		if src.Name != "" {
			seen[src.Name] = true
		}
	}
	var names []string
	for ns := range seen {
		names = append(names, ns)
	}
	sort.Strings(names)
	return names
}
