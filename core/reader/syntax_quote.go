package reader

import "unicode/utf8"

// IsAutoGensymSymbolName reports whether a syntax-quoted unqualified symbol
// name ends with # and therefore needs reader auto-gensym expansion.
func IsAutoGensymSymbolName(name string, hasNamespace bool) bool {
	if hasNamespace {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(name)
	return r == '#'
}

// AutoGensymPrefix returns the generated-symbol prefix for an auto-gensym
// syntax-quote symbol name. Call only after IsAutoGensymSymbolName is true.
func AutoGensymPrefix(name string) string {
	return name[:len(name)-1] + "__"
}
