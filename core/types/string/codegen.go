package string

import stringsdk "strings"

// SymbolGoName converts a Joker symbol string into a Go-name-safe fragment.
func SymbolGoName(sym string) string { return GoName(stringsdk.ReplaceAll(sym, "/", "_FW_")) }

// KeywordGoName converts a Joker keyword string into a Go-name-safe fragment.
func KeywordGoName(kw string) string {
	return GoName(stringsdk.ReplaceAll(stringsdk.ReplaceAll(kw, "/", "_FW_"), ":", ""))
}

// VarRefExprName rewrites a generated var_ prefix to varRefExpr_.
func VarRefExprName(name string) string { return stringsdk.Replace(name, "var_", "varRefExpr_", 1) }

// TypeNameInCore strips the root package prefix from a reflected type string.
func TypeNameInCore(typeName string) string { return stringsdk.Replace(typeName, "core.", "", 1) }

// TypeNameAsGo strips pointer markers and lower-cases the initial character.
func TypeNameAsGo(typeName string) string {
	typeName = stringsdk.Replace(typeName, "*", "", 1)
	if typeName == "" {
		return ""
	}
	return stringsdk.ToLower(typeName[0:1]) + typeName[1:]
}

// IsInteropName reports whether a symbol name uses Joker's Java interop naming
// conventions such as a leading dot, trailing dot, or embedded dollar sign.
func IsInteropName(name string) bool {
	return stringsdk.HasPrefix(name, ".") || stringsdk.HasSuffix(name, ".") || stringsdk.Contains(name, "$")
}

// IsRecordConstructorName reports whether a symbol name denotes a record constructor helper.
func IsRecordConstructorName(name string) bool {
	return stringsdk.HasPrefix(name, "->") || stringsdk.HasPrefix(name, "map->")
}
