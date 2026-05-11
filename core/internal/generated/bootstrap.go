// Package generated defines data-only contracts for future generated namespace
// bootstrap payloads. It deliberately contains no runtime mutation logic; root
// core remains responsible for installing sources/docs into namespaces.
package generated

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
