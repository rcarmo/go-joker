package wasm

const HostModuleName = "joker"

type HostImport struct {
	Name      string
	NumParams int // all current host imports use i64 params and i64 result
}

var StandardHostImports = []HostImport{
	{Name: "get", NumParams: 2},   // (coll, key) -> result
	{Name: "get3", NumParams: 3},  // (coll, key, default) -> result
	{Name: "assoc", NumParams: 3}, // (coll, key, val) -> new_coll
	{Name: "nth", NumParams: 2},   // (coll, idx) -> result
	{Name: "conj", NumParams: 2},  // (coll, val) -> new_coll
	{Name: "count", NumParams: 1}, // (coll) -> i64
	{Name: "first", NumParams: 1}, // (coll) -> result
}

func HostImportNames(imports []HostImport) []string {
	names := make([]string, len(imports))
	for i, imp := range imports {
		names[i] = imp.Name
	}
	return names
}
