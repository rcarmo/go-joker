// Package generated defines data-only contracts for future generated namespace
// bootstrap payloads. It deliberately contains no runtime mutation logic; root
// core remains responsible for installing sources/docs into namespaces.
package generated

import "sort"

// NamespaceSource is an inert generated source payload.
type NamespaceSource struct {
	Name   string
	Path   string
	Source string
}

// VarDoc is inert generated var documentation metadata.
type VarDoc struct {
	Namespace string
	Name      string
	Doc       string
	Arglists  []string
	Private   bool
}

// BinaryPayload is an inert generated binary payload such as serialized linter
// data. Root core decides when and how to process the bytes.
type BinaryPayload struct {
	Path string
	Data []byte
}

// CoreNamespaces returns the sorted namespace set represented by the generated
// bootstrap source manifest.
func CoreNamespaces() []string {
	seen := map[string]bool{}
	for _, src := range CoreSourceManifest() {
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
