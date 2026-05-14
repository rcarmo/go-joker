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

// BinaryPayload is an inert generated binary payload such as serialized linter
// data. Root core decides when and how to process the bytes.
type BinaryPayload struct {
	Path string
	Data []byte
}

// LinterDataPayloads returns the generated linter data payload family in the
// same path vocabulary used by CoreSourceManifest.
func LinterDataPayloads() []BinaryPayload {
	return []BinaryPayload{
		{Path: "linter_all.joke", Data: LinterAllData},
		{Path: "linter_joker.joke", Data: LinterJokerData},
		{Path: "linter_cljx.joke", Data: LinterCljxData},
		{Path: "linter_clj.joke", Data: LinterCljData},
		{Path: "linter_cljs.joke", Data: LinterCljsData},
	}
}
