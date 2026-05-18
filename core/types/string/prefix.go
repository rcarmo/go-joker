package string

import stringsdk "strings"

// TrimVarQuotePrefix removes Joker's #' var-quote prefix when present.
func TrimVarQuotePrefix(name string) string {
	return stringsdk.TrimPrefix(name, "#'")
}

// HasJokerNamespacePrefix reports whether a namespace name is under joker.*.
func HasJokerNamespacePrefix(name string) bool {
	return stringsdk.HasPrefix(name, "joker.")
}
